# Fix: Windows CI failures from hardcoded Unix-style plugin-cache test paths

**Date:** 2026-07-24

## Summary

The Windows Acceptance Tests job failed with `TestConfigurePluginCache` assertion
failures, a panic, and related `pkg/terraform/output` test failures. Fixed by
replacing hardcoded Unix-style literal paths (e.g. `"/atmos/cache"`) in three
test files with `t.TempDir()`-based paths that are genuinely absolute on every
platform.

## Context

The prior fix in this branch ([`2026-07-23-provider-cache-output-lookup-propagation.md`](2026-07-23-provider-cache-output-lookup-propagation.md))
added `IsValidDirectory()` validation in `pkg/terraform/plugin/cache.go` that
rejects relative Terraform provider plugin-cache directories, since a relative
`PluginCacheDir`/`TF_PLUGIN_CACHE_DIR` would resolve differently depending on
each Terraform subprocess's working directory.

That validation uses `filepath.IsAbs()`, which is platform-aware: on Windows a
path must have a drive letter (`C:\...`) or UNC prefix to count as absolute — a
Unix-style path starting with `/` (e.g. `/atmos/cache`) is not absolute there.
Three existing test files hardcoded exactly this kind of Unix-only literal as
a stand-in "valid absolute path," which is itself a violation of this repo's
cross-platform testing rule (`CLAUDE.md`: "NEVER hardcode Unix paths in
expected values"). The new validation now correctly rejected those literals
on Windows CI, producing:

- `internal/exec/terraform_plugin_cache_test.go`: `TestConfigurePluginCache/caching_enabled_with_custom_dir`
  failed with `"[]" should have 2 item(s), but has 0`, followed by a panic
  (`index out of range [0]`) when the test unconditionally indexed the
  now-empty result slice.
- `pkg/terraform/output/environment_test.go`: three `TestConfigurePluginCache`
  subtests failed comparing the hardcoded literal against an empty/XDG-default
  actual value.
- `pkg/terraform/output/executor_test.go`:
  `TestExecutor_ExecuteWithSections_AppliesAutomaticPluginCache` failed the
  same way.

GitHub Job: Acceptance Tests (windows), Job ID 89415454135.

## Changes

- `internal/exec/terraform_plugin_cache_test.go`: replaced `"/custom/terraform/plugins"`,
  `"/user/custom/cache"`, and `"/global/custom/cache"` literals with
  `filepath.Join(t.TempDir(), ...)`-derived paths.
- `pkg/terraform/output/environment_test.go`: replaced `"/atmos/cache"`,
  `"/component/cache"`, `"/process/cache"`, and `"/global/cache"` literals the
  same way.
- `pkg/terraform/output/executor_test.go`: replaced `"/atmos/plugin-cache"`
  with a `t.TempDir()`-derived path.
- Left untouched: table cases using the literal `"/"` (an explicit
  root-rejection case checked by string equality, not `filepath.IsAbs`, so it
  behaves identically on every platform) and unrelated tests in the same
  files that don't route through `Resolve()`/`IsValidDirectory()` (e.g.
  `disableTerraformPluginCacheForExecution*` tests, which only manipulate maps
  and never validate path shape).

## Validation

- `go build ./...` — passed.
- `go test ./internal/exec/... ./pkg/terraform/plugin/... ./pkg/terraform/output/... -run TestConfigurePluginCache -v -count=1` — all previously-failing subtests now pass.
- `go test ./pkg/terraform/output/... -run TestExecutor_ExecuteWithSections_AppliesAutomaticPluginCache -v -count=1` — passed.
- `go test ./pkg/terraform/plugin/... ./pkg/terraform/output/... -race -count=1` — passed.
- `go test ./internal/exec/... -count=1` (full package, matching the package that failed on Windows CI) — passed, `233.293s`.
- `atmos lint --changed` (patch-scoped, this repo's real PR gate) — passed, 0 issues.
- Not re-run on an actual Windows runner (no Windows environment available in
  this session); the fix directly addresses the platform-specific
  `filepath.IsAbs` behavior documented above, and the equivalent Unix-side
  tests continue to pass unchanged.

## Follow-ups

None.
