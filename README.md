# Autonomous Freelance Engine

100% cloud-based, zero-budget, autonomous freelance monitoring and proposal generation engine. Runs entirely inside GitHub Actions on a cron schedule. Telegram delivers ready-to-copy proposals and executive guides.

## How It Works

1. Cron job runs every 5 minutes (or manual dispatch)
2. Scrapes Freelancer.com RSS feeds for new jobs
3. Matches jobs against your skills registry
4. LLM generates a client-ready proposal and executive guide
5. Delivers 3-block Telegram message: Link → Proposal → Guide

## Quick Setup

### 1. Create Telegram Bot

- Open [@BotFather](https://t.me/BotFather) on Telegram
- Send `/newbot` and follow instructions
- Copy the bot token
- Open [@userinfobot](https://t.me/userinfobot) to get your Chat ID

### 2. Get API Keys

- **Gemini (Primary):** Get a free key from [Google AI Studio](https://aistudio.google.com/apikey)
- **OpenCode Zen (Optional Fallback):** Get from your provider
- **Kilo Gateway (Optional Fallback):** Get from your provider

### 3. Configure Repository Secrets

Go to **Settings → Secrets and variables → Actions** and add:

| Secret | Required | Description |
|--------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | Token from @BotFather |
| `TELEGRAM_CHAT_ID` | Yes | Your Telegram chat ID |
| `GEMINI_API_KEY` | Yes | Google AI Studio API key |
| `OPENCODE_ZEN_API_KEY` | Optional | Fallback LLM key |
| `OPENCODE_ZEN_BASE_URL` | Optional | Fallback base URL (defaults to https://opencode.ai/zen/v1) |
| `KILO_GATEWAY_API_KEY` | Optional | Fallback LLM key (also accepts `KILO_API_KEY`) |
| `KILO_GATEWAY_BASE_URL` | Optional | Fallback base URL (defaults to https://api.kilo.ai/api/gateway) |

### 4. Verify Setup

Run the smoke test workflow:

1. Go to **Actions → Smoke Test → Run workflow**
2. You should receive a Telegram message: "Engine Test OK"

### 5. Generate Profiles (Optional)

1. Go to **Actions → Cron Monitor → Run workflow**
2. Set `mode` to `setup`
3. The engine renders `prompts/setup-gigs.tmpl` through the LLM using your skill registry
4. Generated copy is written to `profiles/gig-profiles.md` and committed to the repo

Setup mode runs the profile-generation pipeline only (no RSS scraping, matching, or dedupe). It requires at least one healthy LLM provider, same as monitor mode.

### Tuning (Optional Env Vars)

All values have built-in defaults; set them as repository variables or secrets if you want to override.

| Variable | Default | Purpose |
|----------|---------|---------|
| `MAX_JOBS_PER_RUN` | `5` | Max jobs processed per monitor run |
| `STATE_PRUNE_DAYS` | `30` | Drop dedupe hashes older than this many days |
| `STATE_MAX_ENTRIES` | `500` | Cap on dedupe hashes kept in `state/seen_jobs.json` |
| `RSS_TIMEOUT_SECONDS` | `10` | HTTP timeout per RSS fetch |
| `LLM_TIMEOUT_SECONDS` | `30` | HTTP timeout per LLM call |

## Skills Configuration

Edit `skills-registry.json` to define your skills. Each skill has keywords used for job matching and optional `skills_packages` from [skills.sh](https://skills.sh) for enhanced proposals.

```json
{
  "id": "web-scraping",
  "name": "Web Scraping & Data Extraction",
  "keywords": ["scraping", "selenium", "puppeteer"],
  "skills_packages": ["scrapegraphai/just-scrape"],
  "priority": 8
}
```

## Architecture

```
RSS Feed → Scraper → Deduplication → Skill Matcher → LLM (Gemini/OpenCode/Kilo)
                                                          ↓
                                              Proposal + Executive Guide
                                                          ↓
                                                    Telegram Alert
```

## Local Development

```bash
cp .env.example .env
# Fill in your API keys
go run ./cmd/freelance-engine
```

Set `DRY_RUN=true` to test Telegram connectivity without real LLM calls.

## License

Copyright (c) 2026. All rights reserved.
This repository and its source code are public solely for cloud-execution (GitHub Actions) and educational purposes.
Strictly Prohibited: Commercial use, modification, redistribution, or publishing as your own work.