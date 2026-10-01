# Fix: Remove the deprecated `atmos version track render` command

**Date:** 2026-08-06

## Summary

Deleted the `atmos version track render` subcommand. It was marked `Deprecated`/`Hidden` in the
same commit that introduced it and has never had a non-deprecated existence in any release.

## Context

Found during a field-test pass of the Version Tracker (`atmos version track`). `git log
--diff-filter=A -- render.go apply.go` showed both files were added in the same commit
(`cba5d2a064`, PR #2664, "feat: format-preserving YAML edits + Atmos Version Tracker"). That commit
is already contained in `v1.223.0-rc.6`, and `v1.225.0` shipped this week, so `render` has been
live in released builds for multiple releases while permanently deprecated. Rather than continue
carrying a command that was superseded by `apply`/the file-managers architecture within its own
introducing PR, the user asked to delete it outright.

## Changes

- Deleted `cmd/version/track/render.go` (the `trackRenderCmd` definition and its `init()`
  registration).
- Relocated the `renderTemplate` helper — shared by `apply`/`verify` for the `template` file
  manager — from `render.go` into `cmd/version/track/apply.go`, its remaining real consumer.
- Removed the now-dead `manager.RenderFile` helper (and its now-unused `os` import) from
  `pkg/version/manager/context.go`; it was only reachable from the deleted command.
- Removed the `ErrRenderFileRequired`/`ErrRenderDrift` sentinel aliases from
  `cmd/version/track/track.go` and the underlying `ErrVersionRenderFileRequired`/
  `ErrVersionRenderDrift` errors from `errors/errors.go` (no other callers).
- Removed the `TestTrackRenderCommandCheckMode` test and its `newRenderCommand` helper from
  `cmd/version/track/track_test.go`.

## Validation

- `go build ./...` — clean.
- `go test ./cmd/version/... ./pkg/version/... ./errors/...` — all pass.
- Rebuilt `./build/atmos` and confirmed live: `atmos version track render --help` no longer
  resolves to a `render` subcommand (falls through to the parent `track` help, exit 0);
  `atmos version track verify`/`apply` still work correctly against a real fixture, confirming the
  relocated `renderTemplate` helper still wires into the `template` file manager.
- Grepped the public docs, the `atmos-version` skill, and the PRD — none referenced `render`, so
  nothing else needed updating.

## Follow-ups

None.
