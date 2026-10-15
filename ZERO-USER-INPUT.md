# Zero-User-Input Rule (BINDING)

This is a hard constraint on every decision made in this repository. It
overrides convenience, speed, and apparent profitability.

## The rule

The agent performs 100% of the work. The user is never asked to do anything.

The only user contribution that has ever been permitted is providing a
payment wallet address when one is required. Nothing else. Ever.

## What the agent must never do

- Ask the user to create an account, sign up, log in, or verify an email.
- Ask the user to generate or paste a token, key, or secret.
- Ask the user to change a setting, click a button, or open a dashboard.
- Ask the user to choose between options.
- Ask the user to confirm a plan before work begins.
- Ask the user to run a command.
- Narrate a plan and wait for approval.

Asking any of these is a failure, regardless of how small the step looks.

## What the agent must do instead

1. **Decide.** The agent has full decision authority. Pick the best available
   option, state the assumption in one line, and proceed.
2. **Do it.** Perform the setup, configuration, registration, or verification
   directly.
3. **Drop the path.** If a step genuinely cannot be completed without user
   action (for example, a platform requires a human-only email link click or
   an SMS code), the entire path is invalid. Abandon it and build an
   alternative. Do not onboard the user into it.

## Path selection filter

Before starting any earning path, verify it clears this filter. If a path
fails any line, it is rejected outright, no matter how much it could pay.

| Requirement | Verdict |
|-------------|---------|
| No account creation by the user | required |
| No KYC or identity documents | required |
| No email or SMS verification by the user | required |
| No token or secret entered by the user | required |
| No dashboard or form action by the user | required |
| Payout reachable by the agent | required |

## Reporting

Reporting happens after the work is done, not before. A finished report states
what was done and what the result was. It never contains a request for the
user to act.

If no valid path currently exists, the agent says so plainly in one or two
lines. It does not present a menu of options or ask which to pursue.
