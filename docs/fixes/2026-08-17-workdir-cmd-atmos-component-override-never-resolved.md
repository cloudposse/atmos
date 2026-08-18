# Fix: `atmos terraform workdir clean/describe/show` never actually resolved an `atmos_component` override

**Date:** 2026-08-17

## Summary

`cmd/terraform/workdir/{clean,describe,show}` accept a component and pass a resolved
`componentConfig` down to `provWorkdir.BuildPath` so it can honor an `atmos_component`
instance-name override (e.g. cleaning up the right on-disk workdir for a component
provisioned under an override). In practice this never worked: `resolveComponentConfig`
always failed to resolve the component, silently logged at Debug, and returned `nil` — so
every clean/describe/show call always fell back to treating `component` as its own instance
name, exactly the failure mode the feature exists to prevent (e.g. `CleanWorkdir` finding
nothing at the wrong path and silently reporting success without removing the real workdir).

## Context

CodeRabbit flagged (PR #2879) that `workdir_clean_cmd_test.go:92`,
`workdir_describe_test.go:55-62`, and `workdir_show_test.go:131-139` mock the manager's
`componentConfig` argument with `gomock.Any()`, which accepts `nil` — so these tests would
keep passing even if the command code stopped forwarding the resolved override. While adding
the behavior-focused tests CodeRabbit asked for (execute the real command path against a real
stack fixture, assert the manager receives the actual resolved config), the fixture-driven test
consistently got `componentConfig == nil` even though the fixture and stack name were correct
and the same setup worked fine through the real `atmos` CLI binary.

Root cause: `resolveComponentConfig` called `e.ExecuteDescribeComponent` with
`AtmosConfig: atmosConfig` — the *caller's* already-loaded config. All three RunE handlers load
that config via `cfg.InitCliConfig(configInfo, false)` (`processStacks=false`, since
`BuildPath` and friends only need `base_path`, not the full stack list). But
`ExecuteDescribeComponentWithContext` only does its own full stack-processing init
(`cfg.InitCliConfig(_, true)`) when its `AtmosConfig` param is `nil`:

```go
if atmosConfig == nil {
    config, err = cfg.InitCliConfig(configAndStacksInfo, true)
    ...
}
```

Since these commands always pass a non-nil (stacks-unprocessed) config, that branch never ran,
so `ExecuteDescribeComponent` always failed with "Could not find the component `X` in the
stack `Y`" — confirmed empirically: identical fixture and code path, `processStacks=false`
always fails, `processStacks=true` always succeeds and returns `atmos_component: <component>`
in the resolved config.

## Changes

- `cmd/terraform/workdir/workdir_helpers.go`: `resolveComponentConfig` now takes a
  `*schema.ConfigAndStacksInfo` (the same struct each RunE handler already builds from CLI
  flags via `buildConfigAndStacksInfo`) instead of an already-loaded `*schema.AtmosConfiguration`.
  It copies the struct, sets `ComponentFromArg`/`Stack` for this call, and calls
  `cfg.InitCliConfig(infoCopy, true)` itself to get a fully stacks-processed config, then passes
  that into `ExecuteDescribeComponent`'s `AtmosConfig` field. This preserves `--base-path`,
  `--config`, `--config-path`, and `--profile` overrides (they come from the same
  `ConfigAndStacksInfo` the caller already derived from flags) while actually letting component
  resolution succeed. Takes the param by pointer (not value) to avoid copying the ~1.8KB struct
  on every call (`gocritic hugeParam`); it is never mutated by the callee.
- `cmd/terraform/workdir/workdir_describe.go`, `workdir_show.go`: updated call sites to
  `resolveComponentConfig(&configInfo, component, stack)`.
- `cmd/terraform/workdir/workdir_clean.go`: `cleanSpecificWorkdir` no longer resolves
  `componentConfig` internally (it didn't have `configInfo` available); the resolve now happens
  in `RunE` (matching describe/show's existing pattern) and the resolved config is passed in as
  a new `componentConfig` parameter.
- `cmd/terraform/workdir/workdir_integration_test.go`: `createTestAtmosConfig`'s fixture stack
  manifest now sets `vars.stage: dev` so `name_pattern: "{stage}"` actually resolves the
  manifest to stack `"dev"` — required for any test that resolves a component through real
  stack processing, not just tests that stub the workdir manager directly.
- Three new regression tests replacing the CodeRabbit-flagged `gomock.Any()` assertions with
  real ones, each executing the real command path against a real stack fixture and asserting
  the manager receives a `componentConfig` whose `atmos_component` key matches:
  `TestCleanCmd_RunE_ForwardsResolvedComponentConfig`,
  `TestDescribeCmd_RunE_ForwardsResolvedComponentConfig`,
  `TestShowCmd_RunE_ForwardsResolvedComponentConfig`. The describe/show tests call
  `initTestIO(t)` (existing helper in `workdir_list_test.go`) since their RunE paths write
  through `pkg/data`/`pkg/ui`, which panic if not initialized in a bare unit-test process.
- Updated all pre-existing `cleanSpecificWorkdir(...)` call sites in
  `workdir_clean_cmd_test.go` for the new 4th parameter (passing `nil` to preserve prior
  behavior where it's not under test).
- `docs/fixes/2026-08-05-workdir-nested-component-path-depth.md` and
  `tests/yaml_func_terraform_source_jit_test.go`: separately, corrected two stale
  documentation/comment references flagged in the same CodeRabbit round (obsolete
  two-`strings.ReplaceAll` description superseded by the rune-pass `-h`/`-s`/`-b` encoding;
  a fixture-path comment still saying `test-producer-from-source` instead of the actual
  `test-producer-hfrom-hsource`). No behavior change in either.

## Validation

- `go build ./...` — clean.
- `go test ./cmd/terraform/workdir/... -v` — all tests pass, including the three new
  regression tests, confirmed failing pre-fix (`componentConfig` was `nil`) and passing
  post-fix (`componentConfig["atmos_component"]` equals the described component).
- `go vet ./cmd/terraform/workdir/...` — clean.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (also caught and fixed a
  `gocritic hugeParam` finding on the new `resolveComponentConfig` signature).

## Follow-ups

None.
