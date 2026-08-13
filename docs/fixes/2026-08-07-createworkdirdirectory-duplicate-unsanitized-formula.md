# Fix: `createWorkdirDirectory` duplicated the pre-fix unsanitized workdir formula

**Date:** 2026-08-07

## Summary

`docs/fixes/2026-08-05-workdir-nested-component-path-depth.md` fixed
`workdir.BuildPath` so that nested component names (e.g. `ecs/cluster`)
sanitize `/` to `-` and no longer add an extra real directory level to the
workdir path. That fix did not cover every caller: `createWorkdirDirectory`
(`pkg/provisioner/workdir/workdir.go`, called from `Service.Provision`, used
for LOCAL/non-source components with `provision.workdir.enabled: true`)
independently re-implemented the exact same pre-fix formula inline and was
never touched. A local component with a nested name (e.g.
`app/local-nested`) still got a workdir one directory level deeper than a
flat sibling at the same stack, reproducing the original bug's symptom
(a relative `backend.local.path` resolving to the wrong ancestor) through a
different code path.

## Context

`workdir.BuildPath`'s doc comment states it is meant to be "the single
formula every provisioner (workdir, source, JIT) shares" specifically so
this class of bug can't recur. `createWorkdirDirectory`, however, computed
the workdir name itself instead of delegating:

```go
workdirName := fmt.Sprintf("%s-%s", stack, component)
workdirPath := filepath.Join(basePath, WorkdirPath, "terraform", workdirName)
```

For `component == "app/local-nested"` this yields
`.workdir/terraform/dev-app/local-nested/` — `filepath.Join` treats the `/`
in the component name as a real path separator, producing two directory
levels where every other caller (via the now-fixed `BuildPath`) produces
one sanitized segment (`dev-app-local-nested`). Confirmed via a real
`atmos terraform apply "app/local-nested" --stack dev` run against the
`tests/fixtures/scenarios/source-provisioner-workdir-nested` fixture: the
workdir landed nested one level too deep, and state written to a relative
`backend.local.path` landed a directory short of the intended root
(`.workdir/.context/tfstate/...` instead of `.context/tfstate/...`).

Other `WorkdirPathKey` writers (`pkg/provisioner/source/provision_hook.go`,
`pkg/component/workdir_path.go`, `pkg/ci/plugins/terraform/handlers.go`)
were audited and confirmed to already call `workdir.BuildPath` (directly or
transitively) rather than re-deriving the formula; `createWorkdirDirectory`
was the only duplicate.

## Changes

- `pkg/provisioner/workdir/workdir.go`: `createWorkdirDirectory` now calls
  `BuildPath(basePath, "terraform", component, stack, nil)` instead of
  re-deriving the workdir name with `fmt.Sprintf("%s-%s", ...)` +
  `filepath.Join`. `component` here is already the resolved instance name
  (the caller in `Provision` already applies the `atmos_component` /
  `extractComponentName` fallback precedence before calling
  `createWorkdirDirectory`), so passing a `nil` `componentConfig` to
  `BuildPath` is safe — its `atmos_component` lookup is a no-op fallback
  that would only re-derive what the caller already resolved.

## Validation

- New regression test, confirmed failing pre-fix and passing post-fix:
  `TestServiceProvision_NestedComponentName_SanitizesLikeBuildPath`
  (`pkg/provisioner/workdir/workdir_test.go`) — drives the real
  `Service.Provision` → `createWorkdirDirectory` path with a nested
  `atmos_component` ("app/local-nested"), asserts the resulting
  `WorkdirPathKey` path matches `BuildPath`'s canonical (sanitized) output,
  and asserts no nested directory tree was created on disk.
  - Pre-fix failure:
    ```text
    Error: Not equal:
    expected: .../.workdir/terraform/dev-app-local-nested
    actual: .../.workdir/terraform/dev-app/local-nested
    ```
  - Post-fix: `PASS`.
- `go build ./...` — clean.
- `gofumpt -l pkg/provisioner/workdir/workdir.go pkg/provisioner/workdir/workdir_test.go` — clean (no output).
- `go test ./pkg/provisioner/... ./internal/terraform_backend/...` (full
  packages, not just the new test) — all pass, no regressions.
- Manual field-test fixture
  (`tests/fixtures/scenarios/source-provisioner-workdir-nested`, component
  `app/local-nested`) retained for future manual verification; not re-run
  as part of this fix (see the fixture's README for the exact repro
  commands).

## Follow-ups

None.
