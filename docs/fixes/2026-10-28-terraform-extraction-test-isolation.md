# Fix: Preserve test isolation when integrating the Terraform extraction

**Date:** 2026-10-28

## Summary

The extracted Terraform component tests retain upstream credential isolation, and
command test fixtures preserve tools supplied by the repository test runner.

## Context

The Terraform extraction and incoming main both changed the host test runner.
Main added credential isolation while the extraction moved CI-dependent tests to
the component package. Keeping only one side would lose either isolation or the
new package's test dependencies.

The maintained `atmos test` wrapper also supplies installed tools through PATH.
Its project-selection marker must not make ordinary test fixtures remove those
test prerequisites when they switch to disposable projects.

## Changes

- Resolve the host import conflict while retaining main's credential isolation.
- Apply the same isolation to the moved Terraform component test runner without
  changing its portable subprocess modes or cache cleanup.
- Treat the wrapper's initial tools as test prerequisites in `NewTestKit`.
- Explicitly restore the inherited project marker in the child-process test that
  verifies real project-selection replacement. Its exact PATH assertions remain.
- Update plan-diff caller tests to verify the returned difference status and
  unique-copy ownership rather than the removed exit interception behavior.

## Validation

The final CLI and `go build ./...` pass. Full `cmd` and `internal/exec` package
tests pass through the maintained wrapper. Component tests execute successfully,
including portable subprocess, dry-run, stream and plan ownership checks.
`atmos lint --changed` and the final scoped lint pass with zero issues.

Broad tests are tracked in the repair report. Earlier runs with a global memory
keyring override were invalid for tests intentionally sharing credentials across
store instances; removing that override lets the existing mocked AWS identity
suite pass. No production authentication behavior was changed for these tests.

## Follow-ups

Finish the repository-wide validation gates recorded in the Terraform repair
report before declaring the integration ready. No commits or publication are
part of this local repair.
