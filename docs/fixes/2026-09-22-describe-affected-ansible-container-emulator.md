# Fix: `describe affected` now evaluates `ansible`, `container`, and `emulator` components

**Date:** 2026-09-22

## Summary

`atmos describe affected` evaluated only five of the eight component types Atmos supports
(`terraform`, `helmfile`, `packer`, `kubernetes`, `helm`), silently ignoring `ansible`,
`container`, and `emulator`. A changed or deleted component of those three types was never reported
as affected, so CI/CD pipelines built on `describe affected` skipped them. Both the added/modified
path and the deleted path now cover the full eight-type canonical set that `describe component` and
`describe dependents` already use. Fixes cloudposse/atmos#3203.

## Context

`describe affected` has two paths:

- **Added/modified** (`processStackAffected` in `describe_affected_utils_parallel.go`) dispatched to
  a per-type processor for `terraform`, `helmfile`, `packer`, `kubernetes`, and `helm` only.
- **Deleted** (`detectDeletedComponents` in `describe_affected_deleted.go`) iterated
  `deletableComponentTypes`, which #3199/#3202 had brought to the same five types.

Neither path handled `ansible`, `container`, or `emulator`, even though all three are stack-processed
component types with their own CLI commands, and `describe component`/`describe dependents` iterate
the full eight-type list.

`emulator` is special: it is a stack-defined service with no filesystem source tree
(`getComponentBasePath` in `describe_stacks.go` returns an empty base path for it), so it has no
component folder to match changed files against - only its config sections (vars, env, settings,
metadata, dependencies) can mark it affected.

## Changes

- `internal/exec/describe_affected_components.go`:
  - Replaced the byte-identical `processHelmfileComponentsIndexed` and
    `processPackerComponentsIndexed` with a single generic `processSimpleComponentsIndexed(componentType, ...)`
    (removing the `//nolint:dupl` duplication). `terraform`, `kubernetes`, and `helm` keep their
    dedicated processors (Spacelift/Atlantis, Kubernetes manifests, Helm values files).
  - Added `simpleAffectedComponentTypes` (`helmfile`, `packer`, `ansible`, `container`, `emulator`).
- `internal/exec/describe_affected_utils_parallel.go`: `processStackAffected` now loops over
  `simpleAffectedComponentTypes` and calls the generic processor for each, instead of hardcoding
  helmfile/packer.
- `internal/exec/describe_affected_pattern_cache.go`: `getComponentPathPattern` now resolves
  `ansible` and `container` base paths, and returns an empty pattern for `emulator` (no source tree).
- `internal/exec/describe_affected_utils_optimized.go`: `isComponentFolderChangedIndexed` treats an
  empty pattern as "no folder to match" (returns `false`), so `emulator` never errors and is never
  flagged by a file edit.
- `internal/exec/describe_affected_changed_files_index.go`: `getRelevantFiles` and
  `buildNormalizedBasePaths` include the `ansible` and `container` base paths.
- `internal/exec/describe_affected_deleted.go`: `deletableComponentTypes` now lists all eight types,
  so the deleted path stays in sync with the added/modified path.

## Validation

```bash
# New added/modified-path coverage: vars/env changes detected for ansible, container, emulator.
go test ./internal/exec/ -run TestProcessComponentsIndexedVarsEnvChanges -v

# Pattern cache resolves ansible/container paths; emulator has no source pattern.
go test ./internal/exec/ -run TestComponentPathPatternCache_GetComponentPathPattern -v

# Deleted path detects all eight component types.
go test ./internal/exec/ -run TestDetectDeletedComponents_AllProvisionableTypes -v

# No regressions in the existing describe-affected suite (the helmfile/packer refactor is behavior-preserving).
go build ./...
go test ./internal/exec/ -run 'Affected|DetectDeleted|IsAbstract|ProcessComponents'
atmos lint --changed
```

All listed tests pass; `atmos lint --changed` reports 0 issues.

## Follow-ups

None.
