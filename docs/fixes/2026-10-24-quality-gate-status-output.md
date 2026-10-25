# Fix: Send pending quality gate status to stderr

**Date:** 2026-10-24

## Summary

The quality gate's pending reconciliation message now uses the UI channel,
keeping this status message out of captured stdout.

## Context

[Review of PR #3476](https://github.com/cloudposse/atmos/pull/3476#discussion_r4365254221)
identified a `fmt.Printf` call that wrote human-readable status to the data
channel while CI was incomplete.

## Changes

Use `ui.New(iolayer.UI).Writef` for the pending message. Passing the UI writer
explicitly preserves output when running through Mage without the CLI's global
formatter initialization. Pending report publication and update suppression are
unchanged.

## Validation

The existing pending-to-final report test now captures the UI channel and checks
the status message for both successful and failed final CI outcomes. Both cases
failed before the fix because the UI channel was empty.

- `go test -tags=mage -race ./magefiles/qualitygate/...` passed after the fix.
- `go build ./...` passed.
- Patch-scoped `custom-gcl` lint for the quality gate package passed with zero
  issues. `atmos lint --changed` reported four unrelated Terraform lint issues
  when comparing against the advanced `origin/main`; those files have no changes
  in this PR's merge-base diff.
- The fix-log validator and `git diff --check` passed.

## Follow-ups

Verify hosted CI and the updated review on PR #3476 before merging.
