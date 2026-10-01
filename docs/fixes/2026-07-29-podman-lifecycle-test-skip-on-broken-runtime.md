# Fix: `TestPodmanRuntime_ContainerLifecycle_Integration` skips on a broken container runtime instead of failing CI

**Date:** 2026-07-29

## Summary

CI's Linux acceptance-test job failed with `podman start failed: exit status 125: Error: OCI runtime error:
... crun: unknown version specified` — a podman/`crun` version mismatch on the CI runner, unrelated to any
code change in the PR that triggered the run. `TestPodmanRuntime_ContainerLifecycle_Integration` hard-failed
on that error via `require.NoError`, even though the sibling `TestPodmanRuntime_Exec_Integration` already
treats the exact same `Start` failure as an environment issue worth skipping, not a code bug. Brought the two
tests in line: a `Start` failure now skips this test too.

## Context

The failing CI run's diff only touched `internal/yq`, `pkg/utils`, `pkg/yaml`, `pkg/merge`, and
`website/package.json` — nothing in `pkg/container`. `git log` on `pkg/container/podman_test.go` confirms
no relation to those changes either. The failure is a real infrastructure issue: podman was able to pull
the `alpine:latest` image (so `Pull` succeeded and the test proceeded past its existing "podman not
available" skip), but the `crun` OCI runtime installed on that particular CI runner rejected podman's
invocation with `crun: unknown version specified` — a version-compatibility problem between the two
binaries on the runner image, not something a stack-processing or yq-logging PR could cause or fix.

`TestPodmanRuntime_Exec_Integration` (`pkg/container/podman_test.go`) already anticipates this class of
failure: `if err = runtime.Start(ctx, containerID); err != nil { t.Skipf(...); return }`. This test only
hard-fails on `Pull`/`Create` failures, but treated `Start` as an unconditional `require.NoError`, so the
same underlying environment problem that `Exec`'s test tolerates gracefully takes down CI here instead.

## Changes

- `pkg/container/podman_test.go`: `TestPodmanRuntime_ContainerLifecycle_Integration`'s `Start` call now
  skips the test (`t.Skipf` + early return) on error, instead of `require.NoError`, matching
  `TestPodmanRuntime_Exec_Integration`'s existing handling of the identical step. The deferred
  `runtime.Remove` cleanup still runs regardless, since it was already registered before the `Start` call.

## Validation

- `go build ./...` and `go vet ./pkg/container/...` — clean.
- `go test ./pkg/container/...` — passes locally (this sandbox's podman/crun install is healthy, so `Start`
  succeeds for real here rather than skipping — confirms the change doesn't mask a real regression when the
  runtime works).
- `gofumpt -l pkg/container/podman_test.go` — no output.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
- Did not touch `pkg/container/docker_test.go`'s equivalent `TestDockerRuntime_ContainerLifecycle_Integration`
  even though it has the same unconditional `require.NoError` on `Start`: Docker wasn't the runtime that
  failed in this CI run, and scoping the fix to what's actually broken avoids an unrelated, unverified change.

## Follow-ups

None. If `TestDockerRuntime_ContainerLifecycle_Integration` ever hits an analogous Docker-runtime CI flake,
the same skip-on-`Start`-failure pattern applies there too.
