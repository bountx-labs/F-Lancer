package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/bountx-labs/autonomous-freelance-engine/internal/config"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/llm"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/matcher"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/notify"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/proposal"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/scraper"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/state"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if cfg.DryRun {
		log.Println("DRY_RUN mode enabled - no real LLM calls or Telegram sends")
		dryRun(cfg)
		return
	}

	baseDir := os.Getenv("GITHUB_WORKSPACE")
	if baseDir == "" {
		baseDir = "."
	}

	stateDir := filepath.Join(baseDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		log.Fatalf("create state dir: %v", err)
	}

	modelsCfg, err := llm.LoadModelsConfig(filepath.Join(baseDir, "llm-models.json"))
	if err != nil {
		log.Fatalf("load models config: %v", err)
	}

	pool := llm.NewPool(modelsCfg, cfg.GeminiAPIKey, cfg.OpenCodeZenKey, cfg.OpenCodeZenURL, cfg.KiloGatewayKey, cfg.KiloGatewayURL)

	if !pool.IsHealthy() {
		tg := notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)
		tg.SendAlert("No LLM available. Check API keys.")
		os.Exit(0)
	}

	ctx := context.Background()

	skillsReg, err := matcher.LoadRegistry(filepath.Join(baseDir, "skills-registry.json"))
	if err != nil {
		log.Fatalf("load skills registry: %v", err)
	}

	seenPath := filepath.Join(stateDir, "seen_jobs.json")
	seen, err := state.Load(seenPath)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	feeds := skillsReg.GetFeeds(cfg.TargetRSSFeeds)

	gen, err := proposal.New(pool, filepath.Join(baseDir, "prompts"))
	if err != nil {
		log.Fatalf("init generator: %v", err)
	}

	tg := notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)

	jobCount := 0
	maxJobs := 5

	for _, feedURL := range feeds {
		if jobCount >= maxJobs {
			break
		}

		jobs, err := scraper.FetchFeed(feedURL)
		if err != nil {
			log.Printf("fetch feed %s: %v", feedURL, err)
			continue
		}

		for _, job := range jobs {
			if jobCount >= maxJobs {
				break
			}

			if seen.IsSeen(job.Link) {
				continue
			}

			matches := skillsReg.Match(job.Title, job.Description)
			if len(matches) == 0 {
				continue
			}

			prop, err := gen.GenerateProposal(ctx, job, matches)
			if err != nil {
				log.Printf("proposal for %s: %v", job.Link, err)
				continue
			}

			guide, err := gen.GenerateGuide(ctx, job)
			if err != nil {
				log.Printf("guide for %s: %v", job.Link, err)
				continue
			}

			if err := tg.SendJobAlert(job.Link, prop, guide); err != nil {
				log.Printf("telegram alert for %s: %v", job.Link, err)
				continue
			}

			seen.MarkSeen(job.Link)
			jobCount++
			log.Printf("processed job: %s", job.Link)
		}
	}

	if err := seen.Save(); err != nil {
		log.Printf("save state: %v", err)
	}

	log.Printf("run complete. processed %d jobs", jobCount)
}

func dryRun(cfg *config.Config) {
	tg := notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)
	if err := tg.SendAlert("Engine Test OK - Telegram connectivity verified."); err != nil {
		fmt.Printf("DRY_RUN telegram test failed: %v\n", err)
		return
	}
	fmt.Println("DRY_RUN completed successfully")
}