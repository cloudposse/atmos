# Fix: `describe affected` now detects deleted native `helm` (and `kubernetes`) components

**Date:** 2026-09-22

## Summary

`atmos describe affected` reported deleted Terraform, Helmfile, and Packer components but silently
ignored deleted native `helm` components (and `kubernetes` components). A deleted native Helm
release produced an empty affected list, so CI/CD pipelines never learned it needed to be
destroyed. The deletion-detection loops now iterate over the same component-type set as the
added/modified path. Fixes cloudposse/atmos#3199.

## Context

`describe affected` finds deletions in `detectDeletedComponents`
(`internal/exec/describe_affected_deleted.go`) by walking the BASE (remote) stacks and reporting
any non-abstract component missing from HEAD (current). Two helpers do the per-type iteration:
`processAllComponentsAsDeleted` (entire-stack deletion) and `processDeletedComponentsInStack`
(single-component deletion). Both hardcoded the list
`{terraform, helmfile, packer}`.

The added/modified path (`processStackAffected` in `describe_affected_utils_parallel.go`) handles
five types: `terraform`, `helmfile`, `packer`, `kubernetes`, and `helm`. Because `helm` was in the
added path but not the deletion path, an *added* native Helm component was detected while a
*deleted* one was not - exactly the asymmetry reported in #3199.

## Changes

- `internal/exec/describe_affected_deleted.go`:
  - Added a package-level `deletableComponentTypes` slice (`terraform`, `helmfile`, `packer`,
    `kubernetes`, `helm`) with a comment tying it to the added/modified path so the two lists stay
    in sync.
  - Both `processAllComponentsAsDeleted` and `processDeletedComponentsInStack` now range over
    `deletableComponentTypes` instead of the hardcoded three-type literal.
- `internal/exec/describe_affected_deleted_test.go`:
  - `TestDetectDeletedComponents_NativeHelmComponentDeleted` - a deleted native `helm` component is
    reported (regression test for #3199).
  - `TestDetectDeletedComponents_EntireStackDeletedNativeHelm` - a native `helm` component in a
    fully deleted stack is reported with `deletion_type: stack`.
  - `TestDetectDeletedComponents_AllProvisionableTypes` - all five component types are detected on
    deletion.
  - `TestDetectDeletedComponents_EntireStackDeletedSkipsAbstractAndMalformed` - the entire-stack
    path still skips abstract and malformed component sections (closes the last coverage gap in
    `processAllComponentsAsDeleted`).

## Validation

```bash
# The three regression tests fail before the fix (deleted helm/kubernetes not detected),
# pass after.
go test ./internal/exec/ -run 'TestDetectDeletedComponents_(NativeHelmComponentDeleted|EntireStackDeletedNativeHelm|AllProvisionableTypes)' -v

# No regressions in the deletion-detection suite.
go test ./internal/exec/ -run 'TestDetectDeletedComponents|TestIsAbstractComponent'

# Coverage of describe_affected_deleted.go is 90-100% per function
# (processAllComponentsAsDeleted went 85.7% -> 100%).
go build ./...
atmos lint --changed
```

All listed tests pass; `atmos lint --changed` reports 0 issues.

## Follow-ups

- #3203 - extend `describe affected` to also evaluate `ansible`, `container`, and `emulator`
  component types (across both the added/modified and deleted paths), matching the full
  eight-type canonical set that `describe component` and `describe dependents` already use.
