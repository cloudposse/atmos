# Fix: bulk `--tags`/`--labels`-only selection recursed infinitely instead of running

**Date:** 2026-08-25

## Summary

`atmos aws cloudformation deploy --tags=<x> -s <stack>` (and any other verb, or `--labels`, and the
same trigger on native `kubernetes`/`helm` components) hung indefinitely at high CPU with zero
output whenever the bulk selection was triggered by `--tags`/`--labels` alone, without `--all` or
`--affected`. `pkg/component/graph.go`'s per-node dispatch cleared `All`/`Affected` before handing a
node off to its component provider, but never cleared `Tags`/`Labels` — so each provider's
top-level `Execute()` (which re-enters the bulk path whenever `len(info.Tags) > 0 ||
len(info.Labels) > 0`) saw the selection still active and recursed into `executeBulk` again, for
every single node, forever, before any node did real work.

## Context

Found live during a field-test pass on the `aws/cloudformation` feature (4 stacked phase branches
culminating in `osterman/cfn-phase4-migration-graduation`): running `deploy --tags=group-a -s local`
against a local Floci emulator spun at ~55% CPU for 90+ seconds producing no output at all — not
even the command's usual first-line banner. Sampling the running process (`sample <pid>` on macOS)
showed an unbounded repeating stack trace: `executeBulk → executeGraph → ComponentProvider.Execute →
Execute → executeBulk → ...`. Killing the process and confirming via `list` showed no stack was ever
actually created — the recursion happens before any node's real work begins.

`pkg/component/graph.go`'s `ExecuteGraph`/`executeGraphNode` is shared infrastructure used by three
callers: `pkg/component/aws/cloudformation/executor_bulk.go`, `pkg/component/kubernetes/executor.go`,
and `pkg/component/helm/executor.go` — all three have the byte-identical bulk-trigger condition in
their own `Execute()`, so the bug affected all three component types identically, not just
CloudFormation.

## Changes

- `pkg/component/graph.go`: `executeGraphNode` now also clears `nodeInfo.Tags = nil` and
  `nodeInfo.Labels = nil` alongside the existing `nodeInfo.All = false`/`nodeInfo.Affected = false`,
  before dispatching each node. Safe because `nodeInfo` is a fresh per-node copy (not the shared
  `opts.Info`), and tag/label-based graph filtering (`filterGraphByTagsAndLabels`) already ran
  earlier in `prepareExecutionOrder`, before any node is selected for dispatch — clearing them on the
  dispatched copy has no effect on selection, only on preventing the same node's own `Execute()` call
  from re-triggering the bulk path.
- `pkg/component/graph_test.go`: added `TestExecuteGraphNodeDispatchClearsTagsAndLabels` — a fixture
  component with matching `metadata.tags`/`metadata.labels` (so it survives the pre-dispatch filter
  and actually reaches the provider), asserting every dispatched call's
  `ConfigAndStacksInfo.Tags`/`.Labels` are empty. Written and confirmed failing against the
  pre-fix code first, per this repo's Bug Fixing Workflow.

A CFN-package-level "end-to-end" regression test (mocking through the real `executeBulk` +
`component.ExecuteGraph` with a bounded call-count guard) was considered and deliberately skipped —
it would only re-exercise the same `graph.go` clearing logic through extra mocking layers without
adding real confidence, which is closer to coverage theater than a meaningful regression guard.

## Validation

- `go test ./pkg/component/graph_test.go ./pkg/component/...` — new test fails on pre-fix code
  (confirmed), passes after the fix; no regressions in existing graph/bulk-dispatch tests.
- `go test ./pkg/component/aws/cloudformation/... ./cmd/aws/cloudformation/... ./pkg/component/kubernetes/... ./pkg/component/helm/...`
  — all pass.
- Live, against a real local Floci AWS emulator (`examples/cloudformation/`, extended with a second
  tagged component during the field-test pass): `atmos aws cfn deploy --tags=group-a -s local` and
  `--tags=group-b` both complete in ~6s (previously hung indefinitely), each correctly matching only
  its own tagged component.
- `atmos lint --changed` — clean for this change (one pre-existing, unrelated finding in
  `cmd/terraform/utils.go` remains, confirmed via `git log` to predate this session).
- `atmos build && go build ./...` — clean.

## Follow-ups

None. The blast radius (kubernetes/helm) is covered by the single shared `graph.go` fix; no
per-package changes were needed, and existing kubernetes/helm test suites pass unchanged.
