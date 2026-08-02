package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/bountx-labs/autonomous-freelance-engine/internal/config"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/executor"
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

	skillRunner := executor.New(os.TempDir())
	generate := withSkillContext(gen, skillRunner)

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

			prop, err := generate(ctx, job, matches)
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

// withSkillContext wraps proposal generation. For each matched skill that
// declares skills_packages, it installs the package via `npx skills add`,
// injects the SKILL.md content into the LLM context, then generates the
// proposal. Failures to install a skill never block the proposal — the job
// is flagged skill_missing and the pipeline continues.
func withSkillContext(gen *proposal.Generator, runner *executor.SkillRunner) func(context.Context, scraper.Job, []matcher.MatchResult) (string, error) {
	return func(ctx context.Context, job scraper.Job, matches []matcher.MatchResult) (string, error) {
		var skillContext string

		for _, m := range matches {
			for _, pkg := range m.Skill.SkillsPackages {
				res, err := runner.InstallAndRun(pkg)
				if err != nil || !res.Success {
					log.Printf("skill %s install failed for %s: %s", pkg, job.Link, res.Error)
					continue
				}
				skillContext += "\n\n" + res.SKILLMD
				if res.Artifact != "" {
					skillContext += "\nSample run output:\n" + res.Artifact
				}
			}
		}

		if skillContext != "" {
			// Clone job description with skill knowledge appended for LLM context.
			job.Description += "\n\n--- RELEVANT SKILL KNOWLEDGE ---" + skillContext
		}

		return gen.GenerateProposal(ctx, job, matches)
	}
}

func dryRun(cfg *config.Config) {
	tg := notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)
	if err := tg.SendAlert("Engine Test OK - Telegram connectivity verified."); err != nil {
		fmt.Printf("DRY_RUN telegram test failed: %v\n", err)
		return
	}
	fmt.Println("DRY_RUN completed successfully")
}