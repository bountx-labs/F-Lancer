package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bountx-labs/F-Lancer/internal/bounty"
	"github.com/bountx-labs/F-Lancer/internal/config"
	"github.com/bountx-labs/F-Lancer/internal/llm"
	"github.com/bountx-labs/F-Lancer/internal/notify"
)

// runCertify completes the MoltJobs certification packs. The general pack is
// required before any bid is accepted, so it runs first. A pack that fails is
// reported honestly rather than retried in a loop.
func runCertify(cfg *config.Config, pool *llm.Pool, baseDir string, tg *notify.Telegram) {
	apiKey := os.Getenv("MOLTJOBS_API_KEY")
	if apiKey == "" {
		log.Println("certify: MOLTJOBS_API_KEY missing")
		tg.SendAlert("Certify mode: MOLTJOBS_API_KEY missing.")
		return
	}

	client := bounty.NewClient(apiKey, time.Duration(cfg.LLMTimeoutSeconds)*time.Second)
	certifier := bounty.NewCertifier(client, pool)
	ctx := context.Background()

	var lines []string
	for _, pack := range []string{bounty.EvalGeneral, bounty.EvalEngineering, bounty.EvalProduct} {
		result, err := certifier.Certify(ctx, pack)
		if err != nil {
			log.Printf("certify: %s failed: %v", pack, err)
			lines = append(lines, fmt.Sprintf("%s: error %v", pack, err))
			continue
		}
		status := "FAILED"
		if result.Passed {
			status = "PASSED"
		}
		lines = append(lines, fmt.Sprintf("%s: %.0f%% %s", pack, result.Score, status))
	}

	summary := "Certification results -- " + strings.Join(lines, " | ")
	log.Println(summary)
	tg.SendAlert(summary)
}

// runBounty implements MODE=bounty: the autonomous crypto earning leg. It
// registers nothing and needs no user action beyond an optional receive
// address; MoltJobs provisions the agent wallet itself.
func runBounty(cfg *config.Config, pool *llm.Pool, baseDir string, tg *notify.Telegram) {
	apiKey := os.Getenv("MOLTJOBS_API_KEY")
	if apiKey == "" {
		tg.SendAlert("Bounty mode: MOLTJOBS_API_KEY missing. Add it once and the agent runs itself.")
		log.Println("bounty: MOLTJOBS_API_KEY not configured, skipping")
		return
	}

	client := bounty.NewClient(apiKey, time.Duration(cfg.LLMTimeoutSeconds)*time.Second)
	ctx := context.Background()

	// Certification is the hard gate on the marketplace. Without a passing
	// general pack every bid is refused, so it runs before any bidding and
	// only while certification is still missing.
	if strings.EqualFold(os.Getenv("BOUNTY_CERTIFY"), "true") {
		certifier := bounty.NewCertifier(client, pool)
		for _, pack := range []string{bounty.EvalGeneral, bounty.EvalEngineering, bounty.EvalProduct} {
			result, err := certifier.Certify(ctx, pack)
			if err != nil {
				log.Printf("bounty: certification %s failed: %v", pack, err)
				continue
			}
			log.Printf("bounty: certification %s scored %.0f%%, passed=%v", pack, result.Score, result.Passed)
		}
	}

	// Resolve the agent id from the API key so setup only needs one secret.
	// An explicit MOLTJOBS_AGENT_ID still wins when provided.
	agentID := strings.TrimSpace(os.Getenv("MOLTJOBS_AGENT_ID"))
	if agentID == "" {
		resolved, err := client.ResolveAgentID(ctx, os.Getenv("MOLTJOBS_AGENT_HANDLE"))
		if err != nil {
			log.Printf("bounty: resolve agent id: %v", err)
			tg.SendAlert("Bounty mode: could not resolve the agent id from the API key. Set MOLTJOBS_AGENT_ID if the list call is unavailable.")
			return
		}
		agentID = resolved
		log.Printf("bounty: resolved agent id %s from API key", agentID)
	}

	verticals := splitList(os.Getenv("BOUNTY_VERTICALS"))
	if len(verticals) == 0 {
		verticals = bounty.DefaultVerticals
	}

	// The bounty loop runs often and dedupe skips what it already saw, so a
	// wider per-run budget costs little and raises the odds of catching a
	// freshly posted bounty before it is taken.
	maxBids := cfg.MaxJobsPerRun
	if env := strings.TrimSpace(os.Getenv("BOUNTY_MAX_PER_RUN")); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			maxBids = n
		}
	}

	runner := bounty.NewRunner(pool, client, tg, bounty.Config{
		AgentID:    agentID,
		Verticals:  verticals,
		MaxBids:    maxBids,
		MaxWorkers: 2,
	})
	scanner := bounty.NewGitHubScanner(githubToken(), 15*time.Second)
	if store, err := bounty.NewSeenStore(filepath.Join(baseDir, "state", "bounty_seen.json")); err != nil {
		log.Printf("bounty: seen store unavailable: %v", err)
	} else {
		store.Prune(7 * 24 * time.Hour)
		scanner.SetSeenStore(store)
		defer func() {
			if err := store.Save(); err != nil {
				log.Printf("bounty: save seen store: %v", err)
			}
		}()
	}

	runner.SetScanner(scanner)
	runner.SetContributor(bounty.NewContributor(githubPRToken(), 15*time.Second))
	runner.Run(ctx)
	// Sweep earnings only when a receive address is configured.
	sweep(ctx, client, agentID, baseDir, tg)
}

// runBountyProbe verifies the delivery machinery end to end without opening a
// pull request. It resolves the token identity, checks a real issue can be
// parsed into file targets, and reads one of those files. Use it to confirm
// the pipeline works before letting a run act on a live bounty.
func runBountyProbe(cfg *config.Config, baseDir string, tg *notify.Telegram) {
	token := githubPRToken()
	if token == "" {
		log.Println("bounty-probe: no PR token configured")
		tg.SendAlert("Bounty probe: no PR token configured.")
		return
	}

	ctx := context.Background()
	contributor := bounty.NewContributor(token, 15*time.Second)

	login, err := contributor.CurrentLogin(ctx)
	if err != nil {
		log.Printf("bounty-probe: identity check failed: %v", err)
		tg.SendAlert("Bounty probe failed at identity: " + err.Error())
		return
	}
	log.Printf("bounty-probe: authenticated as %s", login)

	// Probe against a real issue that lists its target files.
	const probeRepo = "mxsm/rocketmq-rust"
	const probeNumber = 11546

	fetched, err := contributor.FetchIssue(ctx, probeRepo, probeNumber)
	if err != nil {
		log.Printf("bounty-probe: fetch issue failed: %v", err)
		tg.SendAlert("Bounty probe failed fetching the issue: " + err.Error())
		return
	}

	targets := bounty.TargetFilesForProbe(*fetched)
	log.Printf("bounty-probe: issue %s#%d lists %d target files", probeRepo, probeNumber, len(targets))
	if len(targets) == 0 {
		log.Println("bounty-probe: no target files parsed")
		tg.SendAlert("Bounty probe: file parsing returned nothing.")
		return
	}

	ref, err := contributor.ReadFileAtFork(ctx, probeRepo, "main", targets[0])
	if err != nil {
		// Reading from the upstream repo is fine too; the branch may not exist
		// on this account yet.
		ref, err = contributor.ReadFile(ctx, probeRepo, targets[0], "")
	}
	if err != nil {
		log.Printf("bounty-probe: read %s failed: %v", targets[0], err)
		tg.SendAlert("Bounty probe failed reading " + targets[0] + ": " + err.Error())
		return
	}
	log.Printf("bounty-probe: read %s (%d bytes)", ref.Path, len(ref.Content))

	// Report the agent's own open pull requests and whether any carry
	// reviewer feedback that still needs an answer.
	handler := bounty.NewReviewHandler(nil, contributor)
	pulls, err := handler.PendingReviews(ctx, login)
	if err != nil {
		log.Printf("bounty-probe: list open pull requests failed: %v", err)
	} else {
		pending := 0
		for _, p := range pulls {
			comments, err := contributor.ReviewComments(ctx, p.Repo, p.Number)
			if err != nil {
				log.Printf("bounty-probe: %s#%d comments failed: %v", p.Repo, p.Number, err)
				continue
			}
			if len(comments) > 0 {
				pending++
			}
			log.Printf("bounty-probe: open pull %s#%d (%s) with %d review comments", p.Repo, p.Number, p.Title, len(comments))
		}
		log.Printf("bounty-probe: %d open pull requests, %d awaiting a reply", len(pulls), pending)

		if orphans, err := handler.FindOrphans(ctx, login); err != nil {
			log.Printf("bounty-probe: orphan scan failed: %v", err)
		} else {
			for _, o := range orphans {
				log.Printf("bounty-probe: orphan %s#%d idle %d days (comments %d)", o.Repo, o.Number, o.IdleDays, o.Comments)
			}
			log.Printf("bounty-probe: %d orphaned pull requests", len(orphans))
		}
	}

	summary := fmt.Sprintf("Bounty probe OK. Identity: %s. Parsed %d target files. Read %s (%d bytes).",
		login, len(targets), ref.Path, len(ref.Content))
	tg.SendAlert(summary)
	log.Println(summary)
}

// sweep moves accrued USDC to the user's address when WALLET.md defines one.
// With no address the agent keeps earnings in its own wallet, which is the
// fully autonomous default.
func sweep(ctx context.Context, client *bounty.Client, agentID, baseDir string, tg *notify.Telegram) {
	address := bounty.ReadReceiveAddress(bounty.ReadWalletFile(baseDir))
	if address == "" {
		log.Println("bounty: no receive address configured, earnings stay in agent wallet")
		return
	}

	wallet, err := client.Wallet(ctx, agentID)
	if err != nil {
		log.Printf("bounty: read wallet: %v", err)
		return
	}
	balance := strings.TrimSpace(wallet.UsdcBalance)
	if balance == "" || balance == "0" || balance == "0.00" {
		log.Println("bounty: wallet balance is zero, nothing to sweep")
		return
	}

	if err := client.Withdraw(ctx, agentID, address, balance); err != nil {
		log.Printf("bounty: sweep failed: %v", err)
		tg.SendAlert("Bounty sweep failed: " + err.Error())
		return
	}
	log.Printf("bounty: swept %s USDC to configured address", balance)
	tg.SendAlert("Bounty sweep complete: " + balance + " USDC sent to the configured wallet.")
}

// splitList parses a comma-separated env value into trimmed entries.
func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// githubToken returns the best available token for the public search API.
// A dedicated token raises the rate limit enough to scan every bounty label;
// the default workflow token is used only as a last resort.
func githubToken() string {
	for _, key := range []string{"BOUNTY_GITHUB_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// githubPRToken returns the token used to open pull requests and post claims.
// It needs write access to forks and public repositories, so it is separate
// from the read-only search token.
func githubPRToken() string {
	for _, key := range []string{"PR_GITHUB_TOKEN", "BOUNTY_GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}
