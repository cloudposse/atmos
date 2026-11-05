# Fix: Stack creation recovery and lifecycle field-test findings

**Date:** 2026-10-19

## Summary

Address the six findings from the local field test of PR #3429: partial creation,
inherited CI mode, missing empty-selection CI results, named templates ignored by
configuration selection, silent grouped execution, and a destroy example missing
approval.

## Context

A late scaffold write failure left a manifest without a generation record and
blocked retry. Stack lifecycle children could re-enable CI reporting despite the
parent's `--ci=false`. Successful selections matching nothing omitted the CI
result contract. Stack creation reread `atmos.yaml` instead of using templates
from selected configuration files and profiles. Concurrent grouped logs could
remain silent for the entire operation. The documented concurrent CI destroy
example correctly failed approval preflight.

## Changes

- Journal exclusively created files and roll them back on write failure or
  cancellation. Preserve replaced or edited files and report residual paths.
  Retain empty directories and hook side effects. Stage stale generation records
  and restore them without overwriting concurrent files; retain a recovery backup
  if restoration fails. After-hook failures explicitly report that completed
  generation remains on disk. This provides error recovery, not crash atomicity.
- Carry resolved CI mode in child environments and honor it in Terraform,
  CI hooks, reporters, and grouping without removing provider metadata.
- Publish the normal CI summary and zero execution counts, `success=true`, and
  `results=[]` for valid empty selections. Dry runs remain read-only and do not
  publish execution results.
- Resolve scaffold templates from invocation-local configuration sources,
  including imports and profiles. Preserve names and anchor each relative source
  to its declaring file even when an overlay changes only the description.
- Emit immediate grouped-execution status on stderr. Component start is reported
  after its resource guard is acquired; child logs retain their grouped order.
- Document recovery, effective configuration, empty CI results, and progress.
  Add `--auto-approve` to the concurrent CI destroy example.

## Validation

- Fresh `atmos build` and regression tests for late/partial writes, cancellation,
  changed-file preservation, stale-record recovery, CI overrides, empty results,
  configuration provenance, and live progress.
- `atmos test --full` with `TEST` restricted to the eight affected packages:
  `pkg/generator/engine`, `pkg/generator/catalog`, `pkg/stack/create`,
  `pkg/stack/lifecycle`, `cmd/stack`, `cmd/terraform`, `pkg/ci`, and `pkg/config`.
- `env -u NO_COLOR go test ./cmd -short` passed. The first run with inherited
  `NO_COLOR=1` failed an unrelated standalone-script color test; clearing that
  environment variable made both the isolated test and complete package pass.
- Targeted `pkg/hooks` tests exercised real local CI output, including the worker
  override. Race tests passed for `pkg/stack/lifecycle`, `pkg/generator/engine`,
  and `pkg/ci`.
- All five runtime reproductions passed twice with corrected assertions in fresh
  fixture copies. Additional local runs verified CI true/false under `CI` and
  simulated GitHub, profile-only creation and preview purity, stale-record
  restoration, preserved hook-created files, after-hook failures, styled stderr
  progress, repeat deployments, approval rejection, and approved reverse teardown.
  The teardown removed all four local Terraform resources; rejection changed none.
  Grouped progress appeared at 0.28 seconds in the latest 12-second PTY operation.
- Patch-scoped custom golangci-lint passed, including new files, using
  `--new-from-patch`. PR-wide `atmos lint --changed` also checks unrelated changes
  already on the branch and reports 20 findings outside this fix.
- `npm run build` in `website` passed after the documentation edits, including
  62 prebuild tests. It reported existing broken-anchor and MDX normalization
  warnings on other pages.
- Regenerated and validated all 373 CLI screengrabs using an isolated copy of
  the repository workflow. Stack creation/deploy/destroy help text was unchanged.
  Kept regenerated copies in local evidence and restored original casts to avoid
  unrelated output changes and nondeterministic color/timing churn.
- Local evidence and reusable scripts are preserved in
  `.context/fix-3429-5g2wdup4/`; the original field-test artifacts remain intact.
  Cloud and Kubernetes deployment were outside this local pass.

## Follow-ups

None for the six field-test findings. PR-wide validation is not represented as
clean: unrelated lint findings are recorded separately in the local evidence.
No GitHub issues or comments were published.
