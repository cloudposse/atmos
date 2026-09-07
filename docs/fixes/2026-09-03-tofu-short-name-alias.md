# Fix: `atmos toolchain exec -- tofu ...` failed in the pinned atmos.ci container

**Date:** 2026-09-03

## Summary

`atmos.ci`'s "Format HCL" job runs `atmos toolchain exec -- tofu fmt -recursive ...` inside the
pinned `ghcr.io/cloudposse/atmos:1.215.0` container image. It failed with `tool 'tofu' not
configured in .tool-versions`, even though `.tool-versions` declares `opentofu/opentofu 1.12.6`.

## Context

An earlier commit on this branch (`c1c2c8c119`) removed a `tofu v1.11.6` line from
`.tool-versions`, treating it as a stale duplicate of `opentofu/opentofu`. On a current build,
`atmos toolchain exec -- tofu ...` resolves fine without that line: `findBinaryPath` ->
`LookupToolVersion` falls back to `ToolResolver.Resolve("tofu")`, which searches the aqua registry
index for a package whose binary name matches (`pkg/toolchain/registry/aqua/resolve.go`,
`ResolveShortName`, added in `9323b8e514`). `git merge-base --is-ancestor 9323b8e514 v1.215.0`
confirms that commit is *not* an ancestor of `v1.215.0` -- the pinned container image predates
short-name search entirely, so `Resolve("tofu")` errors there and the raw-name lookup in
`.tool-versions` is the only path left.

## Changes

- `pkg/toolchain/installer/installer.go`: added `"tofu": "opentofu/opentofu"` to `BuiltinAliases`,
  alongside the existing `"atmos": "cloudposse/atmos"` entry. `Resolve` checks builtin aliases
  before falling back to registry short-name search, so this makes `tofu` resolve deterministically
  on any atmos build carrying this change, without depending on the aqua registry index being
  populated.
- `pkg/toolchain/types_test.go`: `TestBuiltinAliases` now also asserts the `tofu` entry.
- `.tool-versions`: restored `tofu 1.12.6` (matching `opentofu/opentofu`'s pinned version). Still
  required because the `atmos.ci` job's container image predates this alias; remove once that
  image is bumped to a release built with it.

## Verification

- `go build ./...` and `go test ./pkg/toolchain/...` pass.
- `./custom-gcl run --new-from-rev=origin/main` reports 0 issues.
- Confirmed locally: `atmos toolchain exec -- tofu version` resolves and runs without a `tofu` line
  in `.tool-versions` on a current build (via the alias now, previously via short-name search).
- `atmos.ci`'s `autofix` job passed on the commit restoring the `.tool-versions` line.
