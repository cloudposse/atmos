# Fix: hooks resolve $ATMOS_COMPONENT_PATH to the wrong directory for non-JIT components using metadata.component

**Date:** 2026-07-25

**Issue:** [#2799](https://github.com/cloudposse/atmos/issues/2799)

## Summary

For a component that points at a different local component folder via
`metadata.component` (the long-standing shared-module/alias pattern, not the JIT
`source.uri` provisioner), every built-in hook kind that consumes
`$ATMOS_COMPONENT_PATH` (`infracost`, `trivy`, `checkov`, `kics`, `tflint`, and
`kind: command` hooks referencing it) resolved the path from the **stack-facing
component name** instead of the resolved `metadata.component` target, and ran
against a directory that does not exist. Fixed by backfilling the resolved
Terraform component onto the hook `ConfigAndStacksInfo` in `GetHooks`, reusing
the describe output it already fetches.

## Context

The hook context is built in `cmd/terraform/utils.go` `prepareHookContext()`
from `ProcessCommandLineArgs` only — it never runs stack processing
(`ProcessStacks`), so `info.FinalComponent` and `info.ComponentFolderPrefix`
stay empty. `componentPathFor()` in `pkg/hooks/command_engine.go` resolves the
scan path as:

1. The provisioned JIT workdir, if one resolves and exists (covers the
  `source.uri` family of fixes: #2364/#2371, #2134/#2137, #2309 Bug 2, #2684).
2. Otherwise `TerraformDirAbsolutePath / ComponentFolderPrefix / FinalComponent`,
  falling back to `ComponentFromArg` when `FinalComponent` is empty.

For a plain (non-JIT) aliased component, branch 1 finds no workdir and branch 2
hits the empty-`FinalComponent` fallback, joining the raw CLI/stack-facing name
(e.g. `nat-gateway-alias`) onto the components dir — a folder that never exists
for an alias. For `kind: infracost` the failure is silent: infracost exits 0
with `Could not autodetect any projects from path <dir>` and reports `$0.00`,
indistinguishable from a genuinely cost-free component.

`GetHooks` (`pkg/hooks/hooks.go`) already calls `ExecuteDescribeComponent` to
discover the hooks section, and that output's `component` key carries the
resolved Terraform component (the `metadata.component` value — exactly what
`atmos describe component <alias>` shows as `component:`). The information was
in hand; it just was never copied onto the `info` the hook engines receive.

## Changes

- `pkg/hooks/hooks.go`: added `enrichInfoFromDescribe(info, sections)`, called
  from `GetHooks` right after `ExecuteDescribeComponent` succeeds. It backfills
  `info.FinalComponent` (and `info.ComponentFolderPrefix` for
  `metadata.component` values containing a folder prefix, e.g.
  `shared/nat-gateway`), mirroring the `ProcessStacks` split in
  `internal/exec/utils.go`. Both fields are only set when **both** are empty, so
  a fully populated info (e.g. the CI hook path, which sets `FinalComponent`
  itself in `pkg/ci/plugins/terraform/handlers.go`) is never overwritten.
- No change to `componentPathFor()` itself: its JIT-workdir branch still wins
  when a provisioned workdir exists, and its fallback now receives a populated
  `FinalComponent`.

## Validation

- `go test ./pkg/hooks/ -run TestGetHooks_ResolvesMetadataComponentForHookPath -count=1`
  with the `enrichInfoFromDescribe` call removed — **fails** exactly as the
  issue describes (`.../components/terraform/nat-gateway-alias` instead of
  `.../components/terraform/nat-gateway`), confirming the test reproduces
  #2799.
- Same test with the fix — passes.
- `TestEnrichInfoFromDescribe` (new, table-driven): backfill, folder-prefix
  split, both no-overwrite guards, and missing/non-string/empty `component`
  section no-ops.
- `go test ./pkg/hooks/ -count=1 -v` — all 477 tests pass, 0 failures.
- `go build ./...` — passed.
- `./custom-gcl run pkg/hooks/...` — 15 findings, all pre-existing (hugeParam,
  file-length, godot, err113 on untouched lines/files); none on changed lines.
- `gofumpt -l` on changed files — clean.

## Follow-ups

- The issue also suggests that scanner kinds (infracost et al.) should surface
  "no projects detected at the computed path" as a hook-level warning distinct
  from a successful `$0.00` run, so a broken path can never masquerade as an
  empty component. Not addressed here (separate UX change).
