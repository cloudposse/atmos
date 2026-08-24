# Fix: container bake `vars` no longer emitted as `--var` (unsupported on older buildx)

**Date:** 2026-08-19

## Summary

The `container` step's `action: build` with `with.bake.vars` built a
`docker buildx bake --var NAME=value ...` command. `docker-buildx` 0.13.1, shipped by Debian
Trixie (released 2025-08-09), does not implement `--var` for `bake` — that flag was added in a
materially newer upstream buildx release (`docker/buildx#3610`) — so on Trixie hosts the build
died with `unknown flag: --var`. Fixed by injecting bake vars as `NAME=value` entries in the
`docker buildx bake` subprocess environment instead of a CLI flag, since HCL
`variable "NAME" { default = ... }` blocks have always resolved from a same-named OS
environment variable, with no `--var` flag required, across every buildx version.

## Context

While verifying that the `container` step's `build.load` feature worked without requiring
`bake` (branch `osterman/verify-works-without-bake`), a `with.bake.vars` build was traced on a
Debian Trixie host and failed with `unknown flag: --var` from `docker buildx bake`. The Debian
Trixie `docker-buildx-bake(1)` manpage (package `docker-buildx` 0.13.1+ds1-3) confirms the
supported flags are `-f/--file`, `-h/--help`, `--load`, `--metadata-file`, `--no-cache`,
`--print`, `--progress`, `--provenance`, `--pull`, `--push`, `--sbom`, `--set`, `--builder` — no
`--var`. Upstream `docker/buildx#3610` added `--var` as an explicit-override alternative to the
pre-existing, universally-supported mechanism: `docker buildx bake` HCL `variable "NAME" {}`
blocks resolve their value from a same-named OS environment variable by default. Docker's own
docs even describe `BUILDX_BAKE_DISABLE_VARS_ENV_LOOKUP=1` for users who want to *disable* that
env lookup (e.g. to keep values out of build provenance attestations) — Atmos does not set that
opt-out, matching the pre-existing `--var`-flag behavior, which also had no such control.

Both mechanisms require the same precondition (the bake file must already declare
`variable "NAME" {}`), so switching from the CLI flag to environment-variable injection changes
nothing about what's expressible in a bake file — only how portably the value reaches the bake
evaluator. Since env-var resolution works uniformly across every buildx version, old and new,
this avoids adding any buildx-version detection.

## Changes

- `pkg/container/common.go`: removed the `--var` construction from `buildBakeArgs`; added
  `bakeVarEnv` (bake vars map → sorted `NAME=value` slice) and `bakeCommandEnv` (base env +
  `*BuildConfig` → full subprocess env, or `nil` when there's nothing to inject).
- `pkg/container/docker.go`: `DockerRuntime.Build` now calls
  `applyCommandEnv(cmd, bakeCommandEnv(d.env, config))` after building the base command, so bake
  vars are appended to that command's environment without mutating `d.env` — a later
  `Push`/`Inspect` call on the same runtime instance is unaffected.
- `pkg/container/common_test.go`: updated the `buildBakeArgs` "buildx bake" table case to no
  longer expect `--var`, added a case proving `Bake.Vars` leaves no trace in the CLI args, and
  added `TestBakeVarEnv` / `TestBakeCommandEnv`.
- `website/docs/workflows/workflows/workflow/steps/type/container.mdx`: documented that `vars`
  is injected as environment variables, not `--var`, and still requires a matching
  `variable "NAME" {}` block in the bake file.

Bake is Docker-buildx-only in this codebase (`pkg/container/podman.go` rejects `config.Bake !=
nil` outright), so Podman is unaffected. `pkg/schema/workflow.go`'s `Vars map[string]string`
field is unchanged — only its downstream consumption changed.

## Validation

- `go build ./...`
- `go test ./pkg/container/... -run 'TestBuildBuildArgs|TestBakeVarEnv|TestBakeCommandEnv' -v`
- `go test ./pkg/container/...`
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.

## Follow-ups

None. If a future user needs to avoid bake-var values leaking into build provenance
attestations, that mirrors the upstream `BUILDX_BAKE_DISABLE_VARS_ENV_LOOKUP=1` concern and can
be revisited on request.
