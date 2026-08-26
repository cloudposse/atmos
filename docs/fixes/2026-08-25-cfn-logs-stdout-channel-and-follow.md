# Fix: `aws cloudformation logs` wrote to stderr instead of stdout, and had no live-tail mode

**Date:** 2026-08-25

## Summary

`atmos aws cloudformation logs <component> -s <stack>` (non-`--chart` mode) sent 100% of its
per-event output through the UI channel (`ui.Writeln`/`ui.Error`, stderr) instead of the data
channel (stdout), so `logs >out.txt` produced an empty file — a direct violation of this repo's
data/UI channel convention. Separately, `logs` had no way to continuously tail new events (`watch`
does this, but only for one stack; `logs` uniquely merges nested-stack history but only as a
one-shot snapshot). Both are fixed together since they touch the same code.

## Context

Found live during the same field-test pass as the bulk-selection recursion fix (see
`2026-08-25-bulk-tags-labels-infinite-recursion.md`): redirecting `logs`'s stdout and stderr
separately showed the stdout file completely empty, with every event line landing in stderr instead
— confirmed the shared `printStackEvent` helper (used correctly by `watch`, a live human-facing
status command) was being reused by `logs`'s non-chart branch, which is a pipeable data command per
`docs/io-and-ui-output.md`. `logs`'s own `--chart` branch already correctly used `data.Writeln`,
making the inconsistency clear by contrast. The user separately asked about adding a `--follow` mode
after seeing this; researched prior art (`mindriot101/cftail`, a Rust CLI solving the identical
nested-stack live-tail problem) which confirmed polling is the only viable strategy —
CloudFormation's `DescribeStackEvents` has no push/subscribe API (CloudWatch Logs' `StartLiveTail` is
a different, unrelated AWS service).

## Changes

- `pkg/component/aws/cloudformation/events.go`: extracted the pure line-formatting logic out of
  `printStackEvent` into a new `formatStackEventLine(event) (line string, failed bool)` helper.
  `printStackEvent` (used by `streamStackEvents`/`watch`) is otherwise unchanged — its stderr
  behavior for `watch` is correct and untouched.
- `pkg/component/aws/cloudformation/observability.go`:
  - Added `writeLogLine`, which formats via `formatStackEventLine` and writes via `data.Writeln`
    (stdout) — used by `logs`'s non-chart branch in place of `printStackEvent`.
  - Added a `logsOptions{Chart, Follow bool}` struct (`runLogs` needed a 6th parameter, over this
    repo's 5-argument lint limit; matches the existing `deleteOptions` pattern in `delete.go`).
  - Added `followLogs`: polls every stack in the once-flattened nested-stack tree (matches
    `cftail`'s own design — a child stack created after the initial tree walk is not picked up, a
    documented limitation, not a bug) on `eventPollInterval` (3s, reused from `events.go`), with a
    per-stack dedup set, writing via `writeLogLine`. Unlike `streamStackEvents`/`watch`, it does not
    stop at a terminal stack status and does not apply `watch`'s operation deadline — `--follow` is
    tail -f style, ended by the caller (ctx cancellation), matching this repo's existing
    `--follow`/`-f` convention (`cmd/container/verbs.go`, `cmd/devcontainer/logs.go`,
    `cmd/composition/composition.go`). On `ctx.Done()` it returns a nil error (not `ctx.Err()`) since
    cancellation is the expected successful exit path for a tail, not an abnormal interruption.
- `cmd/aws/cloudformation/cloudformation.go`: registered `--follow`/`-f` on `logs` (`phase3FlagOptions`),
  threaded it through `getOperationFlags`, and added `validateLogsFollowChart` (called from
  `validateOperationArgs`, extracted to a helper to stay under this repo's cyclomatic-complexity
  lint limit) rejecting `--follow` combined with `--chart` with a new sentinel
  (`errors.ErrAwsCloudFormationLogsFollowChartExclusive`) rather than silently ignoring one of them.
- `pkg/component/aws/cloudformation/executor.go`: `OperationLogs` now builds and passes `logsOptions`.
- Updated existing `runLogs` call sites and one test's stderr→stdout capture helper
  (`observability_test.go`) to match.

## Validation

- New tests: `TestFollowLogs_PollError`, `TestFollowLogs_ContextCancelledReturnsNilAfterOnePoll`,
  `TestFollowLogs_PollsEveryStackIndependently`, `TestRunLogs_Follow_DispatchesToFollowLogs`,
  `TestOperationSpecificFlagOptions_Logs_RegistersFollowFlag`, `TestGetOperationFlags_IncludesFollow`,
  `TestValidateOperationArgs_RejectsFollowWithChart`, `TestValidateOperationArgs_AcceptsFollowAlone`,
  `TestValidateOperationArgs_FollowChartCheckIsNoOpOnOtherCommands`.
- `TestRunLogs_MergesAndSortsAcrossStacks` updated from `captureStderr` to `captureStdout` — this is
  itself the regression guard for the channel bug (fails against pre-fix code).
- `go test ./pkg/component/aws/cloudformation/... ./cmd/aws/cloudformation/...` — all pass.
- Live, against a real local Floci AWS emulator: `logs demo -s local >out.txt` now populates
  `out.txt` with the event lines (previously empty); `logs demo -s local --follow` run in a real
  pseudo-TTY (`script -q /dev/null`) streamed the historical event backlog immediately and exited
  cleanly (code 0) on `SIGINT`. Confirming a genuinely new event appears mid-stream (vs. only the
  initial backlog) was inconclusive against Floci specifically — it drops a deleted stack's event
  history immediately once the stack is gone (a pre-existing, documented emulator quirk that equally
  affects `watch`'s identical polling approach), not something this change introduced; the
  multi-stack polling/dedup mechanics are otherwise unit-tested using the same machinery `watch`
  already relies on in production.
- `atmos lint --changed` — clean (fixed two lint findings introduced by this change during
  development: cyclomatic complexity in `validateOperationArgs`, argument-limit on `runLogs`; one
  pre-existing, unrelated finding in `cmd/terraform/utils.go` remains).
- `atmos build && go build ./...` — clean.

## Follow-ups

None.
