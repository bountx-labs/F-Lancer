# Scope Boundary (BINDING)

This defines exactly what the agent may touch. It sits alongside
`ZERO-USER-INPUT.md` and carries the same weight.

## In scope

Everything belonging to the F-Lancer project:

- The `bountx-labs/F-Lancer` repository and all of its contents.
- This project's pull requests, issues, branches, and releases.
- The agent's own bounty activity that exists **because of F-Lancer**, such
  as pull requests opened by the bounty leg of this project.
- Local project files: plans, briefs, queues, recovery notes, todo state.

Within this scope the agent decides what to keep and what to clean. Stale
work is cleaned up without asking. Only current, deliberately selected work
is kept.

## Out of scope

Everything that does not belong to F-Lancer. This includes, without
exception:

- Any other repository owned by the operator account, such as
  `ecosystem-hq`, `JogiCode`, `auto-earn-engine`, or anything similar.
- Any issue, pull request, branch, or file that belongs to another project
  of the operator, even when it appears under the same account.
- Anything owned by a third party that is not connected to this project's
  bounty work.

Out-of-scope items are never inspected, never modified, never closed, and
never mentioned. If one appears during a search, it is skipped silently.

## Why the distinction matters

The operator's GitHub account hosts more than one project. F-Lancer is one of
them. Bounty work for F-Lancer legitimately uses the operator's GitHub
identity, which is why a broad token exists. That token does not extend the
agent's authority to the operator's other work.

A previous run overstepped this boundary by closing issues that belonged to
other projects. That must never happen again.

## The test before acting

Before touching anything on GitHub, ask: does this item exist because of
F-Lancer?

- Yes, or it is this project's own repository: act freely.
- No, or it is unclear: leave it alone.
