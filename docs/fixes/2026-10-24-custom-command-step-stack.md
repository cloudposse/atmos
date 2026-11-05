# Fix: Honor the stack on custom Atmos command steps

**Date:** 2026-10-24

## Summary

A custom `type: atmos` step passes its rendered `stack:` to the child command.

## Context

On main at `cb23b3f701`, a step with `command: describe component noop` and `stack: dev` failed because the child command received no stack flag. Workflow steps already supported this behavior.

## Changes

Append `-s` when the command has no explicit stack flag and insert it before any passthrough `--`. Keep explicit command-line stack precedence, including pflag's attached short form `-sprod`. An attached flag after `--` remains passthrough data and does not suppress the step stack. Copy the workflow test executable into its temporary PATH directory instead of hard-linking the still-running parent executable: Windows holds an image lock on the shared file and otherwise rejects test cleanup. Cover custom command and workflow execution using Go subprocess helpers, and clarify the shared step documentation and agent guidance.

The race shard planner now spreads the four measured expensive integration suites (`internal/exec`, `cmd`, `pkg/toolchain`, and `pkg/describe`) across runners before assigning the remaining shuffled packages. A stable partition retains the seeded order within both groups, and round-robin assignment keeps package counts balanced. All packages still run with the same timeout and parallelism. This prevents the packing that accompanied two consecutive package-deadline failures, where random assignment placed all four suites on one runner; the same planner remains on main at `8d23803cec`.

## Validation

Regressions failed before the fix. Ran `go test ./cmd ./pkg/workflow -run "TestCustomCommand.*Stack|TestWorkflow.*Stack|TestAppendAtmosStepFlags" -count=1` and package lint on the isolated PR. Changed-code lint, `go build ./...`, and the combined website build passed. Tests require no external CLI or services.

Windows CI exposed the hard-link cleanup failure after the workflow argument assertions succeeded. Local focused command/workflow tests and changed-code lint validate the copied fixture; native Windows execution remains a CI check.

The attached-short regression failed before its fix: `-sprod` incorrectly became `-sprod -s dev`. A temporary parser contract check and inspection of the local pflag dependency verified attached values and passthrough handling. The custom-command argument table covers both attached stack selection and attached flags after `--`.

Race-shard balancing regressions failed before the scheduling fix, using the failed CI seed `40128500401`. They verify exact package preservation, no input mutation, deterministic and differing seeded assignments, balanced package counts, and expensive-suite distribution with one, fewer, equal, or more shards than expensive suites.

`go test -race -tags mage ./magefiles/test -count=1` passed, as did the two `internal/exec` tests active at the CI timeouts under `-race`. The tooling tests used command-scoped Git configuration to disable signing only for their disposable fixture commits, avoiding the unavailable local signer; repository signing settings were unchanged. Changed-code mage lint and the fix-log validator passed. CI will verify the revised distribution on Linux runners.

## Follow-ups

None.
