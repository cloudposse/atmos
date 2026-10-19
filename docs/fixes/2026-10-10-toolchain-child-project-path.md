# Fix: Remove inherited project tool selections in nested Atmos commands

**Date:** 2026-10-10

## Summary

Nested Atmos processes replace the parent project's installed tool baseline with the current
project's selections, preserving unrelated PATH entries and tools supplied by the user.

## Context

[Review of PR #3346](https://github.com/cloudposse/atmos/pull/3346#discussion_r4237934289)
identified that a child command switching projects could retain the parent's Terraform version.
Startup only prepended the new selections, and returned without changing PATH when none existed.
A regression test reproduced the incorrect executable lookup in a real child process before the fix.

## Changes

- Track directories inserted by shared startup in an inherited, internal environment marker.
- Remove those directories before resolving the current project's installed selections, including
  when the manifest is absent, empty, unreadable, or selects only uninstalled tools.
- Preserve pre-existing user PATH entries, including selected tool directories already present
  before Atmos startup. Repeated startup remains idempotent.
- Isolate the marker alongside PATH in the command test kit.
- Add child-process regression coverage for missing, empty, unreadable, and uninstalled selections,
  different tools, different versions, unchanged versions, and user-provided tool directories.
  All tools are temporary executable fixtures; startup does not download anything.

## Validation

- Before the fix, the child-process regression failed for absent, empty, unreadable, and uninstalled
  selections and for selections of different tools or versions.
- `go test ./cmd -run '^Test(RootCommand.*|InstalledProjectTools.*)$' -count=1`: passed.
- `TEST='./cmd ./pkg/dependencies' atmos test`: passed.
- `go build ./...`: passed.
- `git diff --check` and the fix-log validator: passed.
- `atmos lint --changed` reported a pre-existing `gci` import-formatting issue in
  `internal/exec/template_funcs_store.go`, which is unchanged by this PR.
- `./custom-gcl run --config=.golangci.yml --allow-serial-runners --new-from-rev=HEAD`: passed.
- Website `npm run build`: passed, with existing warnings about unrelated anchors, dynamic
  imports, and Markdown normalization.

## Follow-ups

None.
