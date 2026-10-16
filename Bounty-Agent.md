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
| Discover | Search GitHub for funded work, by bounty label and by paid wording |
| Qualify | Run hard gates (age, repo health, payout rail, scam and self-funding markers) |
| Check | Confirm the work is deliverable before claiming it |
| Claim | Post a claim comment carrying the payout address from `WALLET.md` |
| Execute | Ask the model for targeted find/replace edits and apply them |
| Deliver | Open a pull request against the upstream repository |
| Respond | Answer reviewer feedback on the agent's own open pull requests |
| Collect | Reward is sent to the address in the claim / `WALLET.md` |
| Report | Telegram digest of wins, losses, earnings, blockers |

## Why discovery searches wording, not only labels

A live scan showed the labelled market almost entirely stale: the newest open
issue with a `bounty` label was months old. Current, genuinely open work sat in
funded programs that publish findings as plain issues with an explicit severity
and reward and no label at all. Discovery therefore runs both label queries and
free-text paid-marker queries, and the paid gate accepts a marker found in the
title or body.

## Why a live bounty document is not enough

Discovery reached a funded program that looked ideal on paper: a public
$3,000 USDC pool, severity tiers, an in-repo bounty policy that named paid
external reporters, and fresh unlabelled findings posted daily. Pull requests
were opened into it. The program was dead on arrival.

The evidence was in plain sight and the old gates could not see it. Its default
branch had not moved since a date weeks earlier. Its merge rate over the three
preceding months had fallen from dozens to three, then to zero. The maintainer
had not acknowledged a single one of the roughly thirty findings posted that
month. What looked like a market was a backlog: a program whose policy was
still published while nobody was triaging.

Two gates now cover this:

- **Main-branch activity.** A repository's `pushed_at` is refreshed by any
  branch push, so a repo drowning in unmerged finding branches still reports
  itself as active. The gate reads the newest commit on the default branch
  instead, because that is what decides whether work lands and gets paid.
- **Selection-time dedupe.** An issue is recorded as handled the moment its
  public claim comment is posted, not after delivery succeeds. Before this, a
  failed solve left the issue unrecorded and the next cycle claimed the same
  issue again, posting the identical public comment twice.

## Why edits are targeted

A fix touches a few lines. Asking a model for the whole file back produced
paraphrases that dropped documentation and helpers the maintainer wrote, which
reads as a rewrite and gets rejected. The model now returns a small list of
find/replace edits applied to the original content, so unrelated bytes are
preserved by construction and a missing or ambiguous find is rejected.

## Configuration

- `WALLET.md` at the repo root holds only a public receive address.
  It is used both for sweeping and for the payout line in a claim. Never a
  private key. If absent, sweeping is skipped and the claim omits the address.
- Secrets live in GitHub Actions secrets only.

## Rules

- $0 budget. Free tiers and free bid allowances only.
- Never bid on work that cannot genuinely be delivered.
- Never log or commit a private key.
- CI is the source of truth for every claim.
