# Fix: Keep runtime dialect fixtures portable across operating systems

**Date:** 2026-10-04

## Summary

The runtime PR now carries the portable loaded-module dialect test previously
added in its integration child, so the runtime layer passes independently.

## Context

PR #3264 acceptance run `37205709026`, Windows shard 3, failed
`TestDialectAppliesToLoadedModules`. The injected reader indexed a map by Unix
absolute paths; Windows normalized those paths differently and received an empty
module rather than the fixture content.

## Changes

Write real modules below `t.TempDir()` with `filepath.Join`, then load them
relative to the script working directory. Preserve loop and reassignment-hint
assertions and require a hint before indexing it.

## Validation

- `go test ./pkg/script/starlark -run '^TestDialect' -count=1 -timeout=5m`
  passed on macOS.
- `go test -race ./pkg/script/starlark -run '^TestDialect' -count=5 -timeout=5m`
  passed on macOS (2.150 seconds).
- `go test -race -count=3 -timeout=5m ./pkg/script/starlark/... ./pkg/flags`
  passed with the runtime review fixes and this fixture change.
- Windows execution is verified by the next CI run; no Windows runtime is
  available locally.

## Follow-ups

None.
