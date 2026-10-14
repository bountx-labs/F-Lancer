package bounty

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bountx-labs/F-Lancer/internal/llm"
)

// Reporter is the side channel for run summaries. The Telegram notifier
// implements it; tests use a no-op.
type Reporter interface {
	SendAlert(text string) error
}

// Config holds the runtime knobs for one bounty-agent cycle.
type Config struct {
	AgentID    string
	Verticals  []string
	MaxBids    int
	MaxWorkers int
	Timeout    time.Duration
}

// Runner discovers, qualifies, bids on, and completes MoltJobs work in one
// autonomous pass. It is safe to run repeatedly: already-bid jobs surface as
// API errors and are skipped rather than failing the whole run.
type Runner struct {
	client    *Client
	evaluator *Evaluator
	worker    *Worker
	reporter  Reporter
	scanner   *GitHubScanner
	cfg       Config
}

// NewRunner wires a runner from a pool, client, and reporter.
func NewRunner(pool *llm.Pool, client *Client, reporter Reporter, cfg Config) *Runner {
	if cfg.MaxBids <= 0 {
		cfg.MaxBids = 5
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Minute
	}
	return &Runner{
		client:    client,
		evaluator: NewEvaluator(pool),
		worker:    NewWorker(pool),
		reporter:  reporter,
		cfg:       cfg,
	}
}

// SetScanner attaches the GitHub bounty scanner so a run also reports live
// crypto bounties beyond the MoltJobs marketplace.
func (r *Runner) SetScanner(s *GitHubScanner) {
	r.scanner = s
}

// Run executes one full cycle and reports what happened. It never returns an
// error for per-job failures: a single bad job must not stop the pass.
func (r *Runner) Run(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	completed, failed := r.finishAssigned(ctx)

	bidsPlaced, evaluated := 0, 0
	verticals := r.cfg.Verticals
	if len(verticals) == 0 {
		verticals = DefaultVerticals
	}
	for _, vertical := range verticals {
		if bidsPlaced >= r.cfg.MaxBids {
			break
		}

		jobs, err := r.client.DiscoverJobs(ctx, vertical, 20)
		if err != nil {
			log.Printf("bounty: discover %q: %v", vertical, err)
			continue
		}

		for _, job := range jobs {
			if bidsPlaced >= r.cfg.MaxBids {
				break
			}
			if job.ID == "" {
				continue
			}

			evaluated++
			ok, err := r.evaluator.CanComplete(ctx, job)
			if err != nil {
				log.Printf("bounty: evaluate %s: %v", job.ID, err)
				continue
			}
			if !ok {
				continue
			}

			bid := SuggestedBid(job)
			if _, err := r.client.PlaceBid(ctx, job.ID, bid, "Autonomous agent. I can deliver this to the stated acceptance criteria."); err != nil {
				log.Printf("bounty: bid %s: %v", job.ID, err)
				continue
			}
			bidsPlaced++
			log.Printf("bounty: bid %s USDC placed on job %s (%s)", bid, job.ID, job.Title)
		}
	}

	summary := fmt.Sprintf("Bounty run complete. Evaluated: %d, bids placed: %d, completed: %d, failed: %d.",
		evaluated, bidsPlaced, completed, failed)
	if scanned := r.reportGitHubBounties(ctx); scanned > 0 {
		summary += fmt.Sprintf(" GitHub bounties seen: %d.", scanned)
	}
	log.Println(summary)
	if r.reporter != nil {
		if err := r.reporter.SendAlert(summary); err != nil {
			log.Printf("bounty: telegram alert failed: %v", err)
		}
	}
}

// reportGitHubBounties scans the public GitHub search API for open, unassigned
// issues carrying a bounty label and returns how many were found. This is the
// supply signal the operator needs: it works with no account and no KYC, so it
// runs even when the MoltJobs marketplace is empty.
func (r *Runner) reportGitHubBounties(ctx context.Context) int {
	if r.scanner == nil {
		return 0
	}

	issues, err := r.scanner.ScanBounties(ctx, nil, r.cfg.MaxBids*4)
	if err != nil {
		log.Printf("bounty: github scan: %v", err)
		return 0
	}

	best := 0.0
	for _, issue := range issues {
		if amt := BountyHint(issue); amt > best {
			best = amt
		}
		log.Printf("bounty: github issue %s (%s) reward~%.0f", issue.HTMLURL, issue.RepoFullName(), BountyHint(issue))
	}
	log.Printf("bounty: github scan found %d open bounty issues (best reward hint ~%.0f)", len(issues), best)
	return len(issues)
}

// finishAssigned completes every job currently assigned to the agent.
func (r *Runner) finishAssigned(ctx context.Context) (completed, failed int) {
	if r.cfg.AgentID == "" {
		return 0, 0
	}

	jobs, err := r.client.AssignedJobs(ctx, r.cfg.AgentID)
	if err != nil {
		log.Printf("bounty: list assigned: %v", err)
		return 0, 0
	}

	for _, job := range jobs {
		if err := r.worker.Complete(ctx, r.client, r.cfg.AgentID, job); err != nil {
			log.Printf("bounty: complete %s: %v", job.ID, err)
			failed++
			continue
		}
		completed++
		log.Printf("bounty: submitted work for job %s (%s)", job.ID, job.Title)
	}
	return completed, failed
}

// ReadReceiveAddress extracts the receive address from a WALLET.md body.
// An empty or placeholder value means sweeping is disabled.
func ReadReceiveAddress(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "RECEIVE_ADDRESS=") {
			continue
		}
		addr := strings.TrimSpace(strings.TrimPrefix(line, "RECEIVE_ADDRESS="))
		addr = strings.Trim(addr, "`\"' ")
		if addr == "" || strings.Contains(strings.ToUpper(addr), "YOUR_") {
			return ""
		}
		return addr
	}
	return ""
}
