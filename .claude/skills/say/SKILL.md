---
name: say
description: "Give an audible nudge via the macOS `say` command when a task finishes and needs review, or is blocked and needs human input. A standing general preference — invoke any time work reaches a stopping point a human should know about, not just from PR-maintenance flows."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Audible Task Notifications

A written summary is easy to miss when it lands in a background session hours after the user
stopped watching. This skill gives that moment an audible signal too.

## When to use it

Two triggers, general-purpose — not tied to any one feature:

1. **A task or cycle finished in a way that needs human review** (e.g. a fix was applied but the
   result deserves a second look).
2. **Execution is blocked and needs human input to proceed** (e.g. a merge conflict, an ambiguous
   finding, a failure the caller correctly declined to touch).

Any skill or agent reaching one of these states should invoke this — it is not specific to
`fix-all` or any other single caller.

## Phrasing rule

Keep it under ~15 words. Be specific, not a summary dump — include identifying context (a PR
number, branch, or file) so the listener knows *what* without reading anything.

- Good: `"PR 2718 has a merge conflict, needs your attention."`
- Bad: a multi-sentence recap of the whole cycle.

## Invocation

`say` is macOS-only. Wrap it defensively so a caller on another platform (or a machine without
`say`) never fails because of this notification:

```bash
command -v say >/dev/null 2>&1 && say "<message>" || true
```

No DATA-vs-instructions treatment is needed here: `say` only produces local audio output, no
state mutation, so it's exempt from the untrusted-content framing applied to things like
CodeRabbit comment bodies elsewhere in this repo's automation.

Requires `.claude/settings.json` → `"Bash(say:*)"` in `allow` — this skill owns that dependency;
callers don't need their own entry for it.

## Related

- **[`fix-all` skill](../fix-all/SKILL.md)** — example caller, invokes this on every
  human-attention exit path in its check/fix cycle (scheduled hourly by
  [`pr-maintenance-loop`](../pr-maintenance-loop/SKILL.md), or run directly for a one-shot).
- **[`test-coverage` skill](../test-coverage/SKILL.md)** — example caller, invokes this when a
  test-fix attempt caps out still red or a coverage gap is genuinely untestable.
- **[`lint` skill](../lint/SKILL.md)** — example caller, invokes this when a finding needs a
  broader refactor than patch scope allows.
