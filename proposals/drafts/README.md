# Drafts & Bid Queue

This folder holds the queue the CI uses to place bids automatically
(`MODE=submit`). Only `submit-queue.json` is machine-processed.

## Queue format

`submit-queue.json` is an array of bid items:

```json
[
  {
    "project_id": 40693857,
    "title": "React / Next.js Business Website",
    "bid_amount": 500,
    "period_days": 14,
    "milestone_percentage": 50,
    "description": "The full proposal text sent to the client."
  }
]
```

Field rules (enforced by the engine):

- `project_id` — Freelancer.com project ID (positive integer), required.
- `bid_amount` — numeric, in the project's currency, required.
- `description` — the proposal body, required.
- `period_days` — delivery days; defaults to `7` when missing/invalid.
- `milestone_percentage` — 0-100; defaults to `50` when out of range.

## How it runs

1. The agent appends drafted bids to `submit-queue.json` and pushes.
2. CI runs `MODE=submit`, places each bid via the official Freelancer.com
   API, and sends a Telegram confirmation per placed bid.
3. Placed items are removed from the queue; failed/invalid items stay for
   the next run. An empty queue file is deleted.
4. Because processed items are stripped immediately, a rerun can never
   place a duplicate bid.

## Requirements

- Repository secret `FREELANCER_OAUTH_TOKEN` (one-time user setup).
- The API token identifies the Freelancer.com account; bids are placed as
  that account. Keep the queue reviewed before pushing.
