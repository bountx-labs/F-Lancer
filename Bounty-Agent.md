# Bounty-Agent — Autonomous Crypto Earning Leg

This is Leg B of the F-Lancer 2026 charter (see `PLAN-2026.md`). It earns
without any owner account on the payout path: money arrives in a self-custody
crypto wallet that the agent generates and controls.

## Why crypto

Traditional marketplaces (Freelancer, Fiverr, Upwork) require a human identity
and a bank account at the payout boundary. Crypto wallets do not. That makes
them the only payout rail where the agent remains 100% autonomous from
discovery through collection.

## What it does

| Stage | Action |
|-------|--------|
| Discover | Poll agent-marketplace and bounty APIs on a cron |
| Qualify | LLM judges whether the work is completable to acceptance criteria |
| Bid | Submit a competitive bid within skill scope |
| Execute | Produce the artifact and self-verify before submitting |
| Deliver | Submit through the platform API with heartbeat presence |
| Collect | Monitor wallet balance; earnings settle on-chain |
| Report | Telegram digest of wins, losses, earnings, blockers |
| Sweep | Move earnings to the configured receive address in `WALLET.md` |

## Configuration

- `WALLET.md` at the repo root holds only a public receive address.
  Never a private key. If absent, sweeping is skipped and earnings accumulate.
- Secrets live in GitHub Actions secrets only.

## Rules

- $0 budget. Free tiers and free bid allowances only.
- Never bid on work that cannot genuinely be delivered.
- Never log or commit a private key.
- CI is the source of truth for every claim.
