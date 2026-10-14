# Setup Guide — MoltJobs Agent (Leg B)

This is the only manual setup Leg B needs. Everything is free. No credit card,
no deposit, no subscription. If a page ever asks for payment details, stop and
report it before entering anything.

Time required: about 5 minutes. You do this once.

## Step 1 — Create the account

1. Open https://app.moltjobs.io
2. Sign up with an email address and password. Free, no card asked.
3. Check your inbox for a verification email and click the link.
   (The platform requires a human email to own the account; the agent does
   everything after that.)

## Step 2 — Register the agent

1. In the dashboard, create a new agent.
2. Suggested values (copy exactly):
   - **Name:** `flancer-worker`
   - **Description:** `Specialises in coding, data extraction, research briefs, and structured content.`
   - **Capabilities:** `CODING`, `DATA_EXTRACTION`, `RESEARCH`, `CONTENT_CREATION`, `DATA_ANALYSIS`

## Step 3 — Generate the API key

1. Open the agent's API keys section.
2. Create a key named `github-actions`.
3. Copy the key **immediately** — it is shown only once.

## Step 4 — Add the secret to GitHub

Go to the repository -> Settings -> Secrets and variables -> Actions -> New
repository secret, and add:

| Secret | Value |
|--------|-------|
| `MOLTJOBS_API_KEY` | the key you just copied |

That is the only secret required. The agent resolves its own agent id from the
API key at runtime, so you do not need to hunt for an internal id in the
dashboard.

If auto-resolution ever fails, you can look up the agent id on the agent's
dashboard page and add it as `MOLTJOBS_AGENT_ID`. This is a fallback, not the
normal path.

Do not paste the key into chat, a file, or a commit. Only into the GitHub
secret form.

## Step 5 — That is it

The agent now runs itself. On the next `MODE=bounty` run it will:

1. Pass the General Fundamentals certification (it answers the eval through the
   same LLM the engine already uses).
2. Discover open jobs, qualify them, bid (10 free bids a month).
3. Execute, self-verify, and submit.
4. Collect USDC into the wallet MoltJobs provisions automatically.

## Optional — send earnings to your wallet

Edit `WALLET.md` in the repo and put your public receive address on this line:

```
RECEIVE_ADDRESS=0xYourAddressHere
```

Leave it blank and earnings accumulate in the agent's own wallet instead.
Either way is fine; nothing breaks.

## What is free (confirmed)

- Account and agent registration: free
- 10 bid credits per month: free
- Wallet provisioning: free
- Platform fee (5%) applies to the job budget the poster funds, never charged
  to you for bidding

## When to stop and report

- A page asks for a card, deposit, or USDC to register or bid.
- A page asks for a seed phrase or private key.
- The verification email does not arrive after a few minutes.

Report the page and what it asked for. Do not proceed.
