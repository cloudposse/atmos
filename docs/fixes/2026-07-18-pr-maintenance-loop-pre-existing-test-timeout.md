# Fix: PR-maintenance loop no longer treats "pre-existing" as "don't touch" for failing tests

**Date:** 2026-07-18

## Summary

`atmos fix coverage`'s patch-scoped `go test` run had no `-timeout` override, so Go's 10-minute
per-binary default tripped on the full `./tests` acceptance package (hundreds of tests, one
binary — Go can't compile/run a subset of a package) whenever a patch touched any file under
`tests/`. Three consecutive hourly `pr-maintenance-loop` cycles on PR #2763 each reported
`STATUS: TESTS_FAILING` with a different, patch-unrelated subtest as the one "running" when the
panic fired — a false positive in the check, not a real failure. The `fix-all`/`test-coverage`
skills and the `test-coverage-fix` agent also treated any failure classified "pre-existing" (not
in a file this patch touched) as untouchable-by-policy, so even a fixable pre-existing failure
would only ever be reported, never resolved.

## Context

`pr-maintenance-loop` runs `fix-all` hourly against PR #2763 (`osterman/auto-install-cast-renderers`).
Every cycle that touched `tests/cli_test.go` / `tests/test-cases/toolchain.yaml` hit
`STATUS: TESTS_FAILING` from `.claude/skills/test-coverage/scripts/patch-test-coverage.sh`, each
time on a different subtest (`atmos_toolchain_info_tar.gz_format_tool`, then
`TestRemoteStackImports_GitDirectoryAndSkipIfMissing`/`_NestedImportsRemote`, then
`TestDescribeAffectedWithInclude`, then `TestToolchainAquaTools_KubectlBinaryNaming`) — none in
files this patch touched, none reproducing as a real failure in isolation with more time. The
common signature was `panic: test timed out after 10m0s` from the Go test binary itself, not an
individual test's `--- FAIL`.

The user's explicit direction: stop letting "it's pre-existing, not this patch's fault" excuse the
loop from either fixing the root cause of a false positive, or attempting a fix on a genuinely
pre-existing failure it's confident it can safely resolve. The loop should care whether the suite
actually passes, not whose commit originally broke it.

## Changes

- `.claude/skills/test-coverage/scripts/patch-test-coverage.sh`: added `-timeout 40m` to the
  scoped `go test` invocation, matching `.atmos.d/test.yaml`'s existing full-suite convention.
  This is the root-cause fix for the false positive — the acceptance package's genuine runtime
  regularly exceeds Go's 10-minute default under concurrent-session load or unauthenticated
  GitHub API calls, even though every individual test passes.
- `.claude/agents/test-coverage-fix.md` (Section A): removed the blanket "pre-existing → do not
  touch, report only" rule. The agent now classifies a failure as in-scope/pre-existing only to
  gauge how much diagnosis is needed, and attempts a fix either way — including fixing a genuine
  bug in untouched code, or fixing test-infrastructure defects (timeout, missing env var, unpinned
  flaky dependency). It reports without attempting a fix only when it can't do so safely/
  confidently this cycle (needs a human decision/credential, or would touch a hard-prohibited
  file).
- `.claude/skills/test-coverage/SKILL.md`: updated the "Fix failing tests" section and frontmatter
  description to match — one fix attempt per failing test per cycle, in-scope or pre-existing
  alike.
- `.claude/skills/fix-all/SKILL.md`: updated `say` trigger 4's meaning (from "pre-existing test
  failure surfaced, reported only" to "a failing test couldn't be safely attempted this cycle"),
  step 2's CI-sourced `Acceptance Tests` handling, step 7's `test-coverage` delegation summary, and
  step 8's readiness-gate wording to match the new policy.

## Validation

- `bash -n .claude/skills/test-coverage/scripts/patch-test-coverage.sh` — shell syntax check
  passes.
- Re-ran `atmos fix coverage` against PR #2763's actual diff after the timeout fix; the run was
  interrupted by the harness before completion, so full pass/fail confirmation under the new
  40-minute timeout is still outstanding — the underlying reasoning (individual tests reproduce as
  passing in isolation; only the overall 10-minute binary cap was tripping) was independently
  confirmed across all three prior cycles' `test-coverage-fix` diagnoses.
- No `.go` files were changed by this fix, so `atmos lint --changed` / `go build ./...` are not
  applicable.

## Follow-ups

Re-run `atmos fix coverage` on a patch that touches `tests/` to confirm the 40-minute timeout
lets the full `./tests` package run to completion without a false-positive panic. No GitHub issue
opened for this — low-risk, self-verifying the next time the loop runs against a `tests/`-touching
patch.
