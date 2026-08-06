---
name: pr-maintenance-loop
description: "Start an hourly background loop that keeps the current branch's PR rebased, its addressed CodeRabbit threads resolved, its CI checks passing, its lint clean, its tests passing with adequate patch coverage, and its code free of architectural-smell (code-hygiene) findings — working toward autonomous merge-readiness. Invoke at the start of a session on a branch with an open PR, or on explicit requests like \"set up the hourly PR loop\" / \"auto-rebase this PR\". For a one-shot run instead of a recurring loop, use the `fix-all` skill directly."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Hourly PR Maintenance Loop

Schedules the [`fix-all`](../fix-all/SKILL.md) skill to run every hour via Claude Code's native
`/loop` primitive (`CronCreate`/`ScheduleWakeup`) — not GitHub Actions, no new external service,
no secrets beyond the local `git`/`gh` session already in use. All the actual check/fix logic
(sync, CI, CodeRabbit threads, lint, coverage, code-hygiene — the security model and audible
notifications that govern it) lives in `fix-all`; this skill only owns the scheduling.

## Known limitations (surface these, don't hide them)

- **Session-only, and there's no cloud-compatible alternative.** The loop lives in this Claude
  Code process, using this exact worktree and this local `gh`/`git` session (signing key, gh auth,
  checkout state) — that's the whole point, per the intro above ("no new external service, no
  secrets beyond the local git/gh session already in use"). It dies when the process ends and
  auto-expires after 7 days. It only fires while idle. There is no cross-session persistence —
  that's why this skill re-establishes the loop every session rather than assuming one is already
  running. A cloud-scheduled agent runs in a fresh, isolated environment with none of that local
  state, so it **cannot** run this particular loop — there's no cloud path to fall back to. That's
  why Step 1 below calls `CronCreate` directly instead of going through the generic `/loop` skill:
  `/loop` would ask whether to use a cloud schedule for anything ≥60 minutes, and that question has
  no useful answer here.
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

Call `CronCreate` **directly** — `cron: "0 * * * *"` (every hour), `recurring: true`, `prompt` set
to the hourly prompt template below with `<number>`/`<repo>` filled in and
`started=<current timestamp>` for the self-expiry sentinel. **Do not** route this through the
generic `/loop` skill: `/loop` asks whether to use a cloud schedule for any interval ≥60 minutes,
but per "Known limitations" above there is no cloud-compatible way to run this loop, so that
question has no useful answer — don't ask it, go straight to `CronCreate`. If a user explicitly
insists on trying `/loop` anyway and it offers the cloud option, tell them plainly it won't work
for this loop and point back to this skill instead of proceeding down that path.

After scheduling, immediately run the hourly prompt's steps once now (don't wait for the first
cron fire) — same as `/loop`'s own fixed-interval mode would have done automatically, but since
this skill bypasses `/loop` it must do that step itself.

## Hourly prompt template

This is the literal text re-enqueued every cycle via `/loop 60m`:

```
[PR-MAINTENANCE-LOOP pr=#<number> repo=cloudposse/atmos started=<ts>]

1. Self-expiry: if more than ~6.5 days have passed since `started=`, say so in the summary
  (approaching the 7-day CronCreate expiry) but keep running.

2. `gh pr view <number> --json state,mergeStateStatus`. If state != OPEN, report it and cancel
  this recurring job — don't keep firing no-ops after merge/close.

3. Invoke the `fix-all` skill (`Skill({skill: "fix-all"})`) — it owns steps 3-10 of what used to
  be inlined here: sync, CI check, CodeRabbit threads, lint, coverage, code-hygiene, and the
  security model / audible-notification rules governing all of it.

4. Always end with a one-line cycle summary, even on the no-op path, for auditability.
```

## Related

- **[`fix-all` skill](../fix-all/SKILL.md)** — owns the actual per-cycle work this loop schedules:
  security model, audible notifications, and the sync/CI/threads/lint/coverage sequence. Also
  invocable directly for a one-shot run without a recurring loop.
- **[`say` skill](../say/SKILL.md)** — audible human-attention nudges; see `fix-all` for the
  trigger list.
- **[`pull-request` skill](../pull-request/SKILL.md)** — owns the label decision tree `fix-all`
  applies autonomously each cycle, plus the still human-attended parts of the PR workflow (blog
  posts, signing setup).
- `.claude/settings.json` — the permissions allowlist that enforces `fix-all`'s hard prohibitions
  when it runs unattended from this loop.
