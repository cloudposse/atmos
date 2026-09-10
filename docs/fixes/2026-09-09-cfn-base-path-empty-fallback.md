# Fix: `aws/cloudformation` base path no longer collapses to the project root when unset

**Date:** 2026-09-09

## Summary

`pkg/config/config.go`'s absolute-path resolution for `aws/cloudformation` components joined
`atmosConfig.Components.CloudFormation.BasePath` onto the base path with no fallback for the Go
zero-value (empty string). Because every pre-existing `atmos.yaml` never declares a
`components."aws/cloudformation"` section, `BasePath` was always empty in practice, and the join
collapsed to the bare repository root instead of the documented default
`components/cloudformation/`. Atmos then looked for `<repo-root>/<component>/template.yaml`
instead of `<repo-root>/components/cloudformation/<component>/template.yaml`, producing a
confusing "no such file or directory" error instead of working out of the box.

## Context

Found via a real-AWS field test against an external, pre-existing repo (infra-live) that had
never configured `components."aws/cloudformation"` — the scenario every existing atmos.yaml is
actually in, since that section did not exist before CloudFormation support was added. The exact
same bug shape was already found and fixed for `components.container.base_path` a few lines above
this block in the same function (`AtmosConfigAbsolutePaths`), with an explicit defensive default
and an explanatory comment. `aws/cloudformation` was added later and missed the same treatment.

A correct, independent fallback already existed in
`pkg/component/aws/cloudformation/config.go` (`DefaultConfig()`, returning
`Config{BasePath: "components/cloudformation"}`) and is already consumed by
`ComponentProvider.GetBasePath()` in `pkg/component/aws/cloudformation/cloudformation.go` — but
neither is on the path that `pkg/config/config.go` uses to compute
`CloudFormationDirAbsolutePath`, so that resolution had no fallback at all.

## Changes

- `pkg/config/config.go` (`AtmosConfigAbsolutePaths`): before joining
  `atmosConfig.Components.CloudFormation.BasePath` into an absolute path, default it to
  `"components/cloudformation"` when empty, mirroring the Container fix's style and comment
  tone. Switched the `filepath.Abs` call to the same `absPathOrError` helper the Container and
  Vendor/Workflows blocks already use, for consistent error wrapping.
- `pkg/config/config_test.go` (`TestAtmosConfigAbsolutePaths`): added two sub-tests mirroring the
  existing Vendor/Workflows empty-base-path sub-test —
  `computes cloudformation absolute path from empty base_path default` (asserts
  `CloudFormationDirAbsolutePath` resolves to `<root>/components/cloudformation` and that the
  in-place default is applied) and `computes cloudformation absolute path from explicit base_path
  override` (asserts an explicit `BasePath` is respected and not clobbered by the default).

Verified no other call site double-applies or conflicts with this default:
`pkg/component/aws/cloudformation/cloudformation.go`'s `GetBasePath()` is an independent consumer
with its own empty-check fallback to `DefaultConfig()`, not reached from
`AtmosConfigAbsolutePaths`. Other readers of `Components.CloudFormation.BasePath` (e.g.
`pkg/utils/component_path_utils.go`, `internal/exec/describe_affected_changed_files_index.go`,
`internal/exec/describe_stacks.go`) run after `AtmosConfigAbsolutePaths` has already defaulted
the field in place, exactly as they already do for `Components.Container.BasePath`.

## Validation

- `go build ./...` — passes.
- `go test ./pkg/config/...` — passes, including the two new sub-tests and the full existing
  suite (no regressions).
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml --new-from-rev=origin/main
  pkg/config/...` — 0 issues on the changed lines. (The repo-wide `atmos lint --changed`
  currently fails on an unrelated, pre-existing typecheck error in
  `pkg/component/aws/cloudformation/delete.go`, which is mid-edit in a separate, concurrent fix
  and out of scope here.)

## Follow-ups

None.
