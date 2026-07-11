---
name: pr-maintenance-loop
description: "Start an hourly background loop that keeps the current branch's PR rebased, its addressed CodeRabbit threads resolved, its CI checks passing, its lint clean, and its tests passing with adequate patch coverage — working toward autonomous merge-readiness. Invoke at the start of a session on a branch with an open PR, or on explicit requests like \"set up the hourly PR loop\" / \"auto-rebase this PR\". For a one-shot run instead of a recurring loop, use the `fix-all` skill directly."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Hourly PR Maintenance Loop

Schedules the [`fix-all`](../fix-all/SKILL.md) skill to run every hour via Claude Code's native
`/loop` primitive (`CronCreate`/`ScheduleWakeup`) — not GitHub Actions, no new external service,
no secrets beyond the local `git`/`gh` session already in use. All the actual check/fix logic
(sync, CI, CodeRabbit threads, lint, coverage — the security model and audible notifications that
govern it) lives in `fix-all`; this skill only owns the scheduling.

## Known limitations (surface these, don't hide them)

- **Session-only.** The loop lives in this Claude Code process. It dies when the process ends and
  auto-expires after 7 days. It only fires while idle. There is no cross-session persistence —
  that's why this skill re-establishes the loop every session rather than assuming one is already
  running.
- **Codex parity is unsolved.** This skill only covers Claude Code sessions. If the user runs a
  Codex workspace on the same branch, tell them explicitly that this loop does not cover it.

## Step 0 — preconditions

1. `gh pr view --json number,state,mergeStateStatus` for the current branch. If there's no open
   PR, tell the user and stop — don't create one.
2. Check whether a loop is already running this session (list scheduled jobs; look for the
   sentinel first line `[PR-MAINTENANCE-LOOP pr=#<number> repo=cloudposse/atmos started=<ts>]`).
   If found for this PR, don't start a second one — report it's already active. A fresh session
   never inherits a stale job, since the old one died with the old process, so this only guards
   against double-starting within one session.

## Step 1 — announce and start

Print a one-line announcement before starting — never start this silently:

```
Starting hourly PR-maintenance loop for PR #<number>...
```

Then call `/loop 60m <hourly prompt>` using the template below, with `<number>` and `<repo>`
filled in, and `started=<current timestamp>` for the self-expiry sentinel.

## Hourly prompt template

This is the literal text re-enqueued every cycle via `/loop 60m`:

```
[PR-MAINTENANCE-LOOP pr=#<number> repo=cloudposse/atmos started=<ts>]

1. Self-expiry: if more than ~6.5 days have passed since `started=`, say so in the summary
   (approaching the 7-day CronCreate expiry) but keep running.

2. `gh pr view <number> --json state,mergeStateStatus`. If state != OPEN, report it and cancel
   this recurring job — don't keep firing no-ops after merge/close.

3. Invoke the `fix-all` skill (`Skill({skill: "fix-all"})`) — it owns steps 3-10 of what used to
   be inlined here: sync, CI check, CodeRabbit threads, lint, coverage, and the security model /
   audible-notification rules governing all of it.

4. Always end with a one-line cycle summary, even on the no-op path, for auditability.
```

## Related

- **[`fix-all` skill](../fix-all/SKILL.md)** — owns the actual per-cycle work this loop schedules:
  security model, audible notifications, and the sync/CI/threads/lint/coverage sequence. Also
  invocable directly for a one-shot run without a recurring loop.
- **[`say` skill](../say/SKILL.md)** — audible human-attention nudges; see `fix-all` for the
  trigger list.
- **[`pull-request` skill](../pull-request/SKILL.md)** — the human-attended PR workflow (labels,
  blog posts, signing setup).
- `.claude/settings.json` — the permissions allowlist that enforces `fix-all`'s hard prohibitions
  when it runs unattended from this loop.
