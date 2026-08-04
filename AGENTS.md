# AGENTS.md

Autonomous freelance monitoring/proposal engine. Go 1.22, pure stdlib (no external deps). Runs 100% in GitHub Actions on a cron; local execution is only for editing/prompting, never building/testing.

## Context & entrypoint

- Module root: `github.com/bountx-labs/autonomous-freelance-engine`. Entrypoint: `cmd/freelance-engine/main.go`.
- Pipeline: RSS feed → `internal/scraper` → dedupe (`internal/state`) → match (`internal/matcher`) → LLM (`internal/llm`) → `internal/proposal` + `internal/executor` (skill dry-runs) → Telegram (`internal/notify`).
- Real code lives here; the parent `Ai_Bots/` folder holds only plan/temp files (`plan/`, `.plans/`, `.tmp_*`). `AGENTS.md` belongs at this repo root.
- No test framework, no lint config, no Makefile. Verification is `go build ./...` and `go vet ./...` (stdlib only, so no dependency install needed).

## How the app runs

- Static binary invoked as `go run ./cmd/freelance-engine`. All config comes from env vars only (no config file, no auto-load of `.env`). See `.env.example` and `internal/config/config.go`.
- Hard requirement: `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` must be set or `config.Load()` exits. `DRY_RUN`, `MODE`, and the LLM keys/URLs are optional.
- Ran on GH Actions, the working dir is `$GITHUB_WORKSPACE`; locally it defaults to `"."`. All relative paths (`llm-models.json`, `skills-registry.json`, `prompts/`, `state/seen_jobs.json`) resolve against it.

## Gotchas an agent would otherwise miss

- `DRY_RUN=true` does NOT run the pipeline — `main.go` short-circuits and only sends a "Telegram connectivity verified" alert. Use it to test creds, not logic.
- `MODE=setup` is referenced in `skills-registry.json`/cron workflow/`prompts/setup-gigs.tmpl`, but `main.go` never actually branches on `cfg.Mode` — the setup path is not implemented. Don't assume it works; verify before relying on it.
- LLM selection is driven by `llm-models.json` `fallback_order` (gemini → opencode → kilo) and per-task model keys (`default`, `proposal`). Adding a task `id` like `setup` means also adding a model entry for it (`pool.pickModel` looks up exact task → else `default`).
- `Kilo.Healthy()` performs a real generation call ("ping"); the others hit model-list endpoints. So health checks DO consume quota for the kilo provider.
- Dedupe state: `state/seen_jobs.json` stores SHA1 of job URLs, pruned after 30 days / capped at 500. The cron workflow commits any state/profiles change back with `[skip ci]`; if a state commit fails to push, duplicates get sent next run.
- Skills (`executor/skillrunner.go`) shell out to `npx skills add <pkg>` then dry-run the first ```python block of SKILL.md. Requires node/npx and python3, only present on the runner. Install failures are non-fatal (job flagged `skill_missing`).
- Telegram uses Telegram MarkdownV2 and escapes content on `Send` but not on `post`; headers are `*bold*` blocks manually. Editing Telegram formatting requires touching `internal/notify/telegram.go` (`escapeMarkdownV2`).
- Job matching is a naive case-insensitive substring scan — keyword long lists matching almost any description; revisit `matcher/` only if match precision matters.

## Cloud/git conventions (from global instructions)

- Never build, test, package, or run long processes on the local machine; prefer GH Actions (missing workflows are worth adding). Never claim a build/test succeeded locally unless it actually ran.
- The `npx skills add` mechanism was never verified locally (blocked by policy) — it only gets validated when the workflow first runs on Actions. Keep that in mind when deciding what to assume about it.
- Secrets go in GH Actions secrets (org-level for `bountx-labs`). The kilo provider reads `KILO_GATEWAY_API_KEY` first, then falls back to `KILO_API_KEY` (both code and workflows). Never hardcode/commit API keys; `.env` is gitignored, `state/seen_jobs.json` and `profiles/` are intentionally committed by workflow.
- All temp/cache/logs must live in-project (`.tmp/` / `.cache/`, gitignored) and be purged before finishing a session; never use global system temp dirs. `.gitignore` already lists `.tmp/` and `.cache/`.
- Code, comments, commits, docs in English; explanations/planning to the user in Roman Urdu.