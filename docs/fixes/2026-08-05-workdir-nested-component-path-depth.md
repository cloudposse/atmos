# Fix: nested component names no longer shift the workdir root

**Date:** 2026-08-05

## Summary

A source-provisioned Terraform component with a nested name (e.g.
`ecs/cluster`) got a workdir one directory level deeper than a flat
component (e.g. `vpc`) at the same stack, purely because of the `/` in its
name. Any path computed relative to the workdir — most visibly a relative
`backend.local.path` template — therefore resolved a different real
ancestor for the nested component, silently writing its Terraform state
under a different root even with identical backend config.

## Context

`workdir.BuildPath` (`pkg/provisioner/workdir/types.go`) is the single
formula every workdir-path consumer uses: the source provisioner
(`pkg/provisioner/source/source.go`'s `buildWorkdirPath`) and
`internal/terraform_backend`'s JIT-workdir local-backend state lookup
(`resolveLocalBackendComponentPath`) both call it directly. It builds the
workdir directory name as `fmt.Sprintf("%s-%s", stack, componentName)` and
`filepath.Join`s that into the path — but never sanitized `/` out of
`componentName` first. For `vpc` that yields the single segment
`fixtures-vpc`; for `ecs/cluster` it yields `fixtures-ecs/cluster`, which
`filepath.Join` treats as *two* real directory levels
(`fixtures-ecs/cluster/`), one deeper than the flat case.

Terraform runs with that workdir as its CWD, and a relative
`backend.local.path` (e.g. `../../../.context/tfstate/...`) is resolved
from there. The same number of `..` therefore lands on a different real
ancestor depending on whether the component name happens to contain `/` —
`<repo>/.context/tfstate/...` for `vpc`, but
`<repo>/.workdir/.context/tfstate/...` for `ecs/cluster`.

## Changes

- `pkg/provisioner/workdir/types.go`: `BuildPath` now replaces `/` with `-`
  in the resolved component name before formatting the workdir directory
  name, mirroring the sanitization `internal/exec/terraform_generate_backends.go`
  already applies for backend template context
  (`strings.Replace(componentName, "/", "-", -1)`). Since `BuildPath` is the
  one formula both the source provisioner and the JIT-workdir state lookup
  call, this single change fixes both.

## Validation

- New regression tests, confirmed failing pre-fix and passing post-fix:
  - `TestBuildPath` (`pkg/provisioner/workdir/types_test.go`) — two new
    table cases (`component`, and `atmos_component` instance-name override)
    with a nested `ecs/cluster` name, asserting the resulting path is a
    single sanitized segment.
  - `TestReadTerraformBackendLocal_JITWorkdir` (`internal/terraform_backend/terraform_backend_local_test.go`) —
    a new subtest exercising the real `ReadTerraformBackendLocal` state-read
    path (not just the isolated `BuildPath` unit) with a nested component
    name, confirming state is found at the sanitized root. Refactored the
    table's common setup into a shared `assertJITStateFound` helper to avoid
    the `dupl` linter's near-duplicate-code flag between this case and its
    sibling.
- `go build ./...`, `gofumpt` — clean.
- `go test ./pkg/provisioner/... ./internal/terraform_backend/...` (full
  packages) — all pass. Also ran the `internal/exec` backend-generation and
  workdir-path tests (`-run 'TestGenerateBackend|TestConstructTerraformComponentWorkingDir|TestWorkdir'`)
  to check for side effects on the wider backend-generation pipeline — pass.
- `./custom-gcl run` via the repo's pre-commit hook — pass.
- Did not run a full real `terraform apply` against a live nested + flat
  component pair (the dogfood report's original manual reproduction) — the
  unit and JIT-workdir-level tests exercise the exact formula and real
  state-read code path the bug lives in, which is sufficient to prove and
  guard the fix without provisioning real cloud/local Terraform state in CI.

## Follow-ups

None.
