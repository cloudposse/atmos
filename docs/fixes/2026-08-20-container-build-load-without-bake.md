# Fix: `container` build step can `load` into the local image store without `bake`

**Date:** 2026-08-20

## Summary

`with.load: true` existed only under `with.bake` (`bake.load`), so getting `docker buildx build
--load` (loading a built image into the local Docker image store) required adopting Docker Bake
and an external `docker-bake.hcl` file, even for users who only wanted the plain
`context`/`dockerfile`/`tags` build path. Added a standalone `load` field to the plain (non-bake)
`engine: buildx` build config so `--load` works without `bake:`.

## Context

A `commands.yaml` diff (reviewed via screenshot on branch `osterman/verify-works-without-bake`)
rewrote a plain buildx build step into a `bake:` block purely to get `load: true`, so a
following `push` step could find the image — with a non-default Buildx driver (e.g.
`docker-container`), the build result lands in BuildKit's own cache rather than the local Docker
daemon's image store, and `push` reads from that local store
(`pkg/container/docker.go`). Tracing the implementation confirmed `Load` was defined only on
`ContainerBuildBakeStep` (`pkg/schema/workflow.go`), and `buildBuildArgs`'s plain path
(`pkg/container/common.go`) never emitted `--load` under any configuration — forcing a
disproportionate migration (an external `.hcl` file, with no atmos-side existence/schema check)
onto anyone who just needed `--load`.

## Changes

- `pkg/schema/workflow.go`: added `Load bool` to `ContainerBuildStep`.
- `pkg/container/runtime.go`: added `Load bool` to `BuildConfig`.
- `pkg/container/common.go`: `buildBuildArgs` emits `--load` in the `engine: buildx` branch when
  `config.Load` is set (mirrors `buildBakeArgs`'s existing `--load` handling).
- `pkg/runner/step/container_build.go`: `buildBuildConfig` threads `build.Load` through;
  `validateBuildAction` now requires `engine: buildx` (or `bake:`) when `load: true` is set,
  mirroring the existing `Driver`/`Cache` buildx-requirement check.
- `pkg/container/common_test.go`, `pkg/runner/step/container_test.go`: added coverage for the new
  field (arg building, config wiring, and the validation error/success cases).
- `website/docs/workflows/workflows/workflow/steps/type/container.mdx`: documented `load` in the
  plain build field list and example.

## Validation

- `go build ./...`
- `go test ./pkg/container/... ./pkg/runner/step/... ./pkg/schema/... -count=1`
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (run with a fresh
  `GOLANGCI_LINT_CACHE` to rule out a corrupted shared cache after killing stale lock-holding
  processes from an earlier lint run).

## Follow-ups

None.
