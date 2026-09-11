package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bountx-labs/F-Lancer/internal/config"
	"github.com/bountx-labs/F-Lancer/internal/freelancer"
	"github.com/bountx-labs/F-Lancer/internal/notify"
)

// submitQueueItem is one queued bid. The agent writes these into
// proposals/drafts/submit-queue.json; CI places the bids and strips
// processed items so a rerun can never double-bid.
type submitQueueItem struct {
	ProjectID           int64   `json:"project_id"`
	Title               string  `json:"title"`
	BidAmount           float64 `json:"bid_amount"`
	PeriodDays          int     `json:"period_days"`
	MilestonePercentage int     `json:"milestone_percentage"`
	Description         string  `json:"description"`
}

// runSubmit implements MODE=submit: it reads the bid queue written by the
// agent, places each bid through the official Freelancer.com API, confirms
// every placed bid via Telegram, and rewrites the queue with only the items
// that still need attention. It requires FREELANCER_OAUTH_TOKEN.
func runSubmit(cfg *config.Config, baseDir string, tg *notify.Telegram) {
	token := os.Getenv("FREELANCER_OAUTH_TOKEN")
	if token == "" {
		log.Println("submit: FREELANCER_OAUTH_TOKEN not set, nothing to do")
		tg.SendAlert("Submit mode: FREELANCER_OAUTH_TOKEN secret is missing. Add it once and bids will be placed automatically.")
		return
	}

	queuePath := filepath.Join(baseDir, "proposals", "drafts", "submit-queue.json")
	data, err := os.ReadFile(queuePath)
	if err != nil {
		log.Printf("submit: no queue file (%v), nothing to do", err)
		return
	}

	var items []submitQueueItem
	if err := json.Unmarshal(data, &items); err != nil {
		log.Fatalf("submit: parse queue: %v", err)
	}
	if len(items) == 0 {
		log.Println("submit: queue empty")
		return
	}

	client := freelancer.NewClient(token, time.Duration(cfg.LLMTimeoutSeconds)*time.Second)
	ctx := context.Background()

	var remaining []submitQueueItem
	placed, pending := 0, 0
	for _, item := range items {
		if item.ProjectID <= 0 || item.BidAmount <= 0 || item.Description == "" {
			log.Printf("submit: invalid queue item (project %d, %s), keeping for fix", item.ProjectID, item.Title)
			pending++
			remaining = append(remaining, item)
			continue
		}

		period := item.PeriodDays
		if period <= 0 {
			period = 7
		}
		milestone := item.MilestonePercentage
		if milestone < 0 || milestone > 100 {
			milestone = 50
		}

		bidID, err := client.PlaceBid(ctx, freelancer.BidRequest{
			ProjectID:           item.ProjectID,
			Amount:              item.BidAmount,
			Period:              period,
			MilestonePercentage: milestone,
			Description:         item.Description,
		})
		if err != nil {
			log.Printf("submit: bid for project %d (%s) failed: %v", item.ProjectID, item.Title, err)
			pending++
			remaining = append(remaining, item)
			continue
		}

		placed++
		log.Printf("submit: bid %d placed on project %d (%s)", bidID, item.ProjectID, item.Title)
		msg := fmt.Sprintf("Bid placed: %s\nProject %d | bid %v | delivery %d days | bid id %d",
			item.Title, item.ProjectID, item.BidAmount, period, bidID)
		if err := tg.SendAlert(msg); err != nil {
			log.Printf("submit: telegram alert failed: %v", err)
		}
	}

	// Persist only unprocessed items so a rerun never double-bids.
	if len(remaining) > 0 {
		out, mErr := json.MarshalIndent(remaining, "", "  ")
		if mErr == nil {
			if wErr := os.WriteFile(queuePath, out, 0644); wErr != nil {
				log.Printf("submit: rewrite queue failed: %v", wErr)
			}
		} else {
			log.Printf("submit: encode queue failed: %v", mErr)
		}
	} else {
		if rErr := os.Remove(queuePath); rErr != nil {
			log.Printf("submit: remove empty queue failed: %v", rErr)
		}
	}

	log.Printf("submit complete: %d placed, %d pending", placed, pending)
	if err := tg.SendAlert(fmt.Sprintf("Submit mode complete. Placed: %d, Pending: %d.", placed, pending)); err != nil {
		log.Printf("submit: telegram summary failed: %v", err)
	}
}
