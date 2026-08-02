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
| `KILO_GATEWAY_API_KEY` | Optional | Fallback LLM key |
| `KILO_GATEWAY_BASE_URL` | Optional | Fallback base URL (defaults to https://api.kilo.ai/api/gateway) |

### 4. Verify Setup

Run the smoke test workflow:

1. Go to **Actions → Smoke Test → Run workflow**
2. You should receive a Telegram message: "Engine Test OK"

### 5. Generate Profiles (Optional)

1. Go to **Actions → Cron Monitor → Run workflow**
2. Set `mode` to `setup`
3. Profiles are generated and committed to `profiles/`

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