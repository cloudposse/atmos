# Fix: Refresh policy CLI help snapshots

**Date:** 2026-10-15

## Summary

Refresh the acceptance snapshots for the policy command and inherited explain flag.

## Context

Linux, macOS, and Windows acceptance shards 2, 5, 6, 7, and 9 failed because their expected
output predates the `policy` command and global `--explain` flag. The attached CI
logs identify the same 15 failing cases across all three platforms. The Windows
attachments contain only runner cleanup output; the full job logs fetched from
GitHub confirm the same snapshot mismatches.

## Changes

Regenerate the affected snapshots through `go test ./tests -regenerate-snapshots`:

- Root help includes `policy` and its description.
- Unknown-command diagnostics list `policy` among valid subcommands.
- Subcommand and alias help includes the inherited `--explain` flag.

The CLI behavior and snapshot comparison rules are unchanged.

## Validation

- Regenerated only the 15 cases identified in the attached logs, using an exact
  subtest-name filter and `-regenerate-snapshots`.
- Reran those 15 cases with `-count=1 -v`, without regeneration: all passed on
  macOS. The harness builds and executes the current source binary.
- Reviewed all 22 added lines: only the expected command and flag entries changed.
- Compared all 22 added lines from the full Windows failure logs against the
  regenerated snapshot diff: every line matches the existing fix.
- Fix-document validation and `git diff --check` passed.
- The hosted Linux/macOS/Windows CI jobs have not been rerun from this workspace.

## Follow-ups

None.
