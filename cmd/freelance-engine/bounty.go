package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bountx-labs/F-Lancer/internal/bounty"
	"github.com/bountx-labs/F-Lancer/internal/config"
	"github.com/bountx-labs/F-Lancer/internal/llm"
	"github.com/bountx-labs/F-Lancer/internal/notify"
)

// runBounty implements MODE=bounty: the autonomous crypto earning leg. It
// registers nothing and needs no user action beyond an optional receive
// address; MoltJobs provisions the agent wallet itself.
func runBounty(cfg *config.Config, pool *llm.Pool, baseDir string, tg *notify.Telegram) {
	apiKey := os.Getenv("MOLTJOBS_API_KEY")
	agentID := os.Getenv("MOLTJOBS_AGENT_ID")

	if apiKey == "" || agentID == "" {
		tg.SendAlert("Bounty mode: MOLTJOBS_API_KEY / MOLTJOBS_AGENT_ID missing. Add them once and the agent runs itself.")
		log.Println("bounty: credentials not configured, skipping")
		return
	}

	client := bounty.NewClient(apiKey, time.Duration(cfg.LLMTimeoutSeconds)*time.Second)

	ctx := context.Background()
	verticals := splitList(os.Getenv("BOUNTY_VERTICALS"))
	runner := bounty.NewRunner(pool, client, tg, bounty.Config{
		AgentID:    agentID,
		Verticals:  verticals,
		MaxBids:    cfg.MaxJobsPerRun,
		MaxWorkers: 2,
	})
	runner.Run(ctx)

	// Sweep earnings only when a receive address is configured.
	sweep(ctx, client, agentID, baseDir, tg)
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
