# F-Lancer — Project Plan

> Operating charter: see `AGENT-PRINCIPLES.md` (user mandate, immutable).
> This plan records where the project stands and how it runs.

## 1. Objective

100% cloud-based freelance job discovery and proposal drafting engine.

- GitHub Actions cron discovers matched Freelancer.com jobs.
- Matched jobs land as markdown briefs in `proposals/inbox/<date>/`.
- A local agent drafts client-ready proposals and executive guides.
- The user performs the single manual step by design: final review and
  submission on Freelancer.com.

## 2. Verified Current State (2026-09)

- **Repo:** `bountx-labs/F-Lancer`, branch `main`, working tree clean.
- **Architecture:**
  `RSS Feed → Scraper → Dedupe → Skill Matcher → LLM (Kilo/Gemini) → Proposal + Guide → Telegram`
- **LLM pool:** Kilo Gateway primary, Gemini flash-lite fallback.
  OpenCode Zen provider removed (commit `fbefd9c`). Cleanup 5/5 complete.
- **Workflows:** `cron-monitor` (every 5 min), `smoke-test`, `secrets-check`.
- **Data:** dedupe state in `state/seen_jobs.json` (prune 30d, cap 500);
  briefs in `proposals/inbox/` for 2026-08-25..28.
- **Local recovery:** `private-recovery/` holds the backed-up session
  transcript and notes. Local-only — never committed, never uploaded.

## 3. Operating Model

### 3.1 Division of Labor

| Layer | Owner | Responsibility |
|-------|-------|----------------|
| Discovery | CI (GitHub Actions) | RSS scrape, dedupe, skill match, write briefs |
| Intelligence | Agent | Read briefs, analyze jobs, draft proposals + guides |
| Submission | User | Review and submit on Freelancer.com (by design) |
| Monitoring | Agent | CI health, LLM failures, inbox backlog, follow-ups |

### 3.2 Agent Routine (on request or scheduled check)

1. Check CI health: `gh run list` — did `cron-monitor` succeed?
2. Read unread briefs in `proposals/inbox/<date>/`.
3. Deep-analyze each job (skills, budget, category, client intent).
4. Draft client-ready proposal + executive guide (with bid suggestion)
   following `prompts/proposal.tmpl` and `prompts/executive-guide.tmpl`.
5. Deliver drafts to the user in one batch for review + submission.
6. Track submitted proposals and follow up on outcomes.

## 4. Roadmap

### Done (verified in repo history)

- [x] Core engine: scrape → dedupe → match → brief → Telegram alert
- [x] Dedupe state with prune/cap (`state/seen_jobs.json`)
- [x] Skills registry + priority matching + skills.sh packages
- [x] Profile generation (setup mode → `profiles/gig-profiles.md`)
- [x] Secrets check workflow (guard against leaked credentials)
- [x] Smoke test workflow (Telegram "Engine Test OK")
- [x] LLM pool cleanup: OpenCode removed, Kilo primary + Gemini fallback

### Known Issues / Next Steps (evidence-based)

- [ ] **Provider error diagnosability** — the LLM pool discards underlying
      provider errors, so failures report only "all providers failed".
      Fix: log per-provider errors in `pool.Complete` so the next CI run
      reveals the actual cause.
- [ ] **Portfolio sample slides rebuild** — 3 slides were created in a past
      session but never committed and were lost in a Windows refresh.
      Client brief context is preserved in the recovery transcript.
- [ ] **Inbox backlog** — process unread briefs from 2026-08-25..28.
- [ ] **Proposal outcome tracking** — record which proposals were submitted
      and their results, to tune matching and bids.

## 5. Credentials & Secrets (F-Lancer)

- Stored ONLY as GitHub Actions secrets
  (Settings → Secrets and variables → Actions).
- Required: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`,
  `KILO_GATEWAY_API_KEY` (also accepts `KILO_API_KEY`).
- Optional: `KILO_GATEWAY_BASE_URL`, `GEMINI_API_KEY`.
- Never in source code, never in chat, never in git history.
- Local `.env` (if ever needed) is gitignored and never committed.

## 6. $0 Budget Rule

- Free tiers only: Kilo free tier, free Gemini key, GitHub Actions free,
  Telegram Bot API free.
- No paid services, no subscriptions, no SaaS upgrades.
- If a free tool limits us, we reduce scope — we do not pay.

## 7. Verification

- CI is the source of truth (`cron-monitor`, `smoke-test`, `secrets-check`).
- Never claim success without inspecting CI output and live results
  (e.g., Telegram alert received).
- The local machine stays lightweight: edit / inspect / orchestrate only.
- All building, testing, and runtime verification happens on GitHub Actions.

## 8. Change Scope

- Smallest correct change. Reuse existing infrastructure.
- One file, one coherent responsibility.
- No speculative features, no unnecessary dependencies.
- Record reusable lessons after each correction.
