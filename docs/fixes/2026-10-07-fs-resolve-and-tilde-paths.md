# Fix: fs.resolve and tilde paths in the Automation Language

**Date:** 2026-10-07

## Summary

Scripts can now call `fs.resolve(path)` to turn a user-supplied path into a cleaned absolute path, and every `fs.*` function accepts a leading `~` for the home directory. The two private `expandHome` copies were collapsed into one shared helper.

## Context

Scripts hand-rolled tilde expansion and absolute-path checks with `env["HOME"]` and `regex.search`. Atmos already had `pkg/config/homedir.Expand`, but `pkg/workflow/container.go` and `pkg/emulator/mounts.go` each kept a private `expandHome` copy that swallowed errors, and the Starlark `fs.*` functions had no tilde support at all.

## Changes

- Added `filesystem.ExpandHome` in `pkg/filesystem/homedir.go`. It returns `homedir.Expand` output and falls back to the input on error, so `~user` stays unchanged.
- Replaced both private `expandHome` functions (workflow container mounts and emulator mounts) with `filesystem.ExpandHome`.
- `session.filesystemPath` in `pkg/script/starlark/filesystem_metadata.go` expands a leading `~` first, then joins relative paths onto the working directory and cleans. `fs.read_file`, `fs.exists`, `fs.glob`, `fs.stat`, and `fs.readlink` all gain tilde support. `fs.glob` reports absolute paths for `~` patterns. `load()` is unchanged.
- Added the `fs.resolve(path)` builtin: no filesystem access, honors cancellation, and an empty path fails with `fs.resolve: path must not be empty`.
- Documented `fs.resolve`, the tilde rule, and the agent-skill references.

## Validation

- `go build ./...`
- `go test ./pkg/filesystem/... ./pkg/script/starlark/ ./pkg/workflow/ ./pkg/emulator/ -count=1`
- `./custom-gcl run --new-from-rev=origin/main ./pkg/filesystem/... ./pkg/script/starlark/ ./pkg/workflow/ ./pkg/emulator/`
- New tests cover `ExpandHome`, `fs.resolve` (tilde, `..`, relative, absolute, empty, cancellation), tilde in `fs.exists`, `fs.read_file`, `fs.stat`, `fs.glob`, and tilde mount sources in the workflow and emulator packages.
- `cd website && npm run build`

## Follow-ups

None.
