# Fix: Propagate the Terraform provider plugin cache to output lookups

**Date:** 2026-07-23

## Summary

Internal Terraform output lookups (`!terraform.output`, `atmos.Component()`,
and `atmos terraform output`) now reuse Atmos's provider plugin cache the
same way `atmos terraform plan`/`apply` do, and a `terraform init` failure
during one of those lookups is now reported instead of silently discarded.

## Context

`configurePluginCache` in `internal/exec/terraform.go` synthesized
`TF_PLUGIN_CACHE_DIR` (and the matching
`TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE` override) only for the
normal Terraform command pipeline. The internal output executor in
`pkg/terraform/output/` builds its own subprocess environment for
`terraform init`/`output` calls used by the `!terraform.output` YAML
function, `atmos.Component()` template function, and `atmos terraform
output`, and never received that configuration. Every output lookup
therefore re-initialized providers from scratch in its component's working
directory unless the cache directory happened to be injected externally
(e.g. by CI), causing redundant provider downloads and cache misses.

## Changes

- Added `pkg/terraform/plugin` (`cache.go`), a shared package that
  centralizes provider plugin-cache policy: `Resolve()` applies precedence
  (explicit override via env or `atmos.yaml` global env, then the
  configured `PluginCacheDir`, then the XDG cache default, else disabled),
  `IsValidDirectory()` rejects empty or `/` overrides, and
  `Cache.InitLockPathForWorkdir()` derives a stable, machine-local lock path
  (outside the cache/working directories) for serializing concurrent
  `terraform init` calls that share one cache.
- Refactored `internal/exec/terraform.go`'s `configurePluginCache` to call
  the shared `plugin.Resolve()` instead of duplicating the resolution logic.
- Added `configurePluginCache()` to `pkg/terraform/output/environment.go`
  and wired it into `executor.go`'s `execute()` so output-lookup subprocess
  environments get the same cache directory and override precedence as the
  main command path.
- `executor_runner.go`'s `runInit()` now wraps `terraform init` with
  `filelock.New(pluginCache.InitLockPathForWorkdir(...)).WithExclusive(...)`
  whenever a cache directory is set, so concurrent output lookups sharing a
  cache don't race on provider installation.
- Fixed a regression introduced by the locking change: the file-lock
  wrapper only propagated a lock-acquisition error and otherwise always
  `return nil`, so a real `terraform init` failure inside the locked section
  was swallowed. `runInit()` now captures the inner `run()` error via
  closure and returns it after the locked section completes.
- Renamed `InitLockPath()` to `InitLockPathForWorkdir(workdir)`, resolving a
  relative `PluginCacheDir` against the component's working directory
  (instead of the process's current directory) so two components with
  different relative cache paths cannot collide on the same lock file.
- Added test coverage for the XDG-cache-unavailable and
  `filepath.Abs`-failure fallback paths by making `getXDGCacheDir` and
  `absolutePath` overridable package variables in
  `pkg/terraform/plugin/cache.go`.

## Validation

- `go build ./...` — passed.
- `go test ./pkg/terraform/plugin/... ./pkg/terraform/output/... -race -count=1` — passed (both packages `ok`).
- `./build/atmos lint --changed` (patch-scoped golangci-lint against
  `origin/main`, the repo's real PR gate per `CLAUDE.md`) — passed, `0
  issues`. This run took over 20 minutes to complete due to heavy CPU
  contention from concurrent builds and lints running in other worktrees on
  the same machine, not because of anything in this change.
- PR #2791's own description additionally reports `go test ./...`,
  `scripts/run-custom-golangci-lint.sh`, and pre-commit hooks
  (`go-fumpt`, Go build, custom lint) as run before this fix-log entry was
  written.

## Follow-ups

None.
