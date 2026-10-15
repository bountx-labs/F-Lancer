package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bountx-labs/F-Lancer/internal/config"
	"github.com/bountx-labs/F-Lancer/internal/llm"
	"github.com/bountx-labs/F-Lancer/internal/matcher"
	"github.com/bountx-labs/F-Lancer/internal/notify"
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
	skillsReg, err := matcher.LoadRegistry(filepath.Join(baseDir, "skills-registry.json"))
	if err != nil {
		log.Fatalf("load skills registry: %v", err)
	}
	if err := skillsReg.Validate(); err != nil {
		log.Fatalf("validate skills registry: %v", err)
	}

	// Inbox mode needs no LLM and no Telegram; it only writes job briefs.
	if cfg.Mode == "inbox" {
		runInbox(cfg, skillsReg, baseDir)
		return
	}

	// Submit mode needs no LLM: it places queued bids via the Freelancer API.
	if cfg.Mode == "submit" {
		tg := notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)
		runSubmit(cfg, baseDir, tg)
		return
	}

	modelsCfg, err := llm.LoadModelsConfig(filepath.Join(baseDir, "llm-models.json"))
	if err != nil {
		log.Fatalf("load models config: %v", err)
	}
	if err := modelsCfg.Validate(); err != nil {
		log.Fatalf("validate models config: %v", err)
	}

	pool := llm.NewPool(modelsCfg, cfg.GeminiAPIKey,
		cfg.KiloGatewayKey, cfg.KiloGatewayURL, time.Duration(cfg.LLMTimeoutSeconds)*time.Second)

	tg := notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)

	if !pool.IsHealthy() {
		tg.SendAlert("No LLM available. Check API keys.")
		os.Exit(0)
	}
	if cfg.Mode == "setup" {
		runSetup(pool, skillsReg, baseDir, tg)
		return
	}

	if cfg.Mode == "strategy" {
		runStrategy(cfg, pool, skillsReg, baseDir, tg)
		return
	}

	if cfg.Mode == "build-queue" {
		runBuildQueue(cfg, pool, baseDir, tg)
		return
	}

	if cfg.Mode == "bounty" {
		runBounty(cfg, pool, baseDir, tg)
		return
	}

	if cfg.Mode == "bounty-probe" {
		runBountyProbe(cfg, baseDir, tg)
		return
	}

	runMonitor(cfg, pool, skillsReg, baseDir, tg)
}

func dryRun(cfg *config.Config) {
	tg := notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)
	if err := tg.SendAlert("Engine Test OK - Telegram connectivity verified."); err != nil {
		fmt.Printf("DRY_RUN telegram test failed: %v\n", err)
		return
	}
	fmt.Println("DRY_RUN completed successfully")
}
