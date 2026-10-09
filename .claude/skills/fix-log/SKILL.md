---
name: fix-log
description: "Use when implementing, finishing, documenting, or reviewing a fix, repair, remediation, bug fix, debug-and-fix task, workflow fix, infrastructure fix, or any change that should leave a durable fix record under docs/fixes."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Fix Documentation

Every implemented fix must leave a short Markdown record in `docs/fixes/`.

`docs/fixes/` already contains 90+ pre-existing records in an older, inconsistent format (some with a
`# Fix: <title>` heading, some without; some with a `**Date:**` line, some without; and 25+ different ad hoc
`## ` section names across the set — `## Problem`, `## Root Cause`, `## Tests`, `## Related`, etc.). **Do not
migrate those** — they stay as-is. All new records use the merged format below, which keeps the old
convention's most useful, scannable elements (the `# Fix:` heading, the explicit `**Date:**` line) and adopts
a single, consistent five-section structure instead of ad hoc headers.

## Workflow

1. Before final ready status, create or update `docs/fixes/YYYY-MM-DD-<slug>.md`.
2. Use the current local date for `YYYY-MM-DD`.
3. Use a lowercase slug with letters, numbers, and hyphens.
4. Record what changed, why it changed, validation that actually ran, and follow-ups.
5. Do not invent validation. If a check was skipped or blocked, say why.
6. Triage every candidate follow-up by origin before writing the Follow-ups section:
    - **Introduced by this PR or its stack** (a regression, gap, inconsistency or stale artifact such as a cast or
      snapshot that the change itself caused): fix it in the same PR. It is not a follow-up, and it never becomes
      an issue.
    - **Pre-existing** (reproduces on the base branch or a released version): ask the user, per item, whether to
      fix it now or open a GitHub issue. Never open an issue without that per-item answer.
    - **Intended behavior** (the code does what it was designed to do, but it surprised someone): document it
      where users will look; it is neither a fix nor a follow-up.
7. Anything left under **Follow-ups** (other than `None.`) must satisfy `CLAUDE.md`'s **Follow-up Tracking
    (MANDATORY)** section: link the user-approved GitHub issue by number (e.g. `#1234`). A Follow-ups entry like
    "a follow-up will..." with no issue number is incomplete — the work will never be tracked.

## Required Document Shape

```markdown
# Fix: <Title>

**Date:** YYYY-MM-DD

## Summary

## Context

## Changes

## Validation

## Follow-ups
```

- `<Title>` is a short human-readable description of the fix (inline code spans are fine, e.g.
  `` # Fix: `describe affected` now checks `source` and `provision` sections ``).
- `**Date:**` restates the filename's date for at-a-glance scanning; keep it in sync with the filename.
- Use `None.` in `Follow-ups` only when no follow-up is known.

## Validation

After creating or updating fix docs, run:

```bash
bash .claude/skills/fix-log/scripts/validate-fix-doc.sh docs/fixes/*.md
```

The validator only checks the five required `## ` sections and the filename shape — it doesn't (yet) enforce
the `# Fix:` heading or `**Date:**` line. Run it against the files you're adding, not the whole directory
indiscriminately (it will report errors against the old-format records, which is expected and not something
to fix).
