# Fix: `restart:`/`healthcheck:` silently dropped on `type: container, action: run` steps

**Date:** 2026-08-07

## Summary

A `type: container, action: run` step (in a workflow file or a custom
command) that set `restart:` and/or `healthcheck:` under its `with:` block
decoded without error, but the values never reached the container runtime:
the resulting `docker create`/`podman create` invocation carried zero
`--restart`/`--health-*` flags. The persistent-component container path
(`components.container` and the `background: true` workflow step) already
wired these fields correctly; only the one-shot ephemeral `run` step path
dropped them.

## Context

`schema.ContainerRunStep` (`pkg/schema/workflow.go`) has had `Restart
*ContainerRestart` and `HealthCheck *ContainerHealthCheck` fields for a
while, and `pkg/container/runspec.go` already provided fully tested
conversion helpers — `RestartPolicyFromStep`, `HealthCheckFromStep`, and
`ValidateRunStep` — shared with the `pkg/emulator` kind, which does call
them. `pkg/runner/step/container_run.go`'s `buildRunConfig` never called
either helper, and `container.EphemeralConfig`
(`pkg/container/ephemeral.go`) — the config type the one-shot `run` step
actually uses — had no `Restart`/`HealthCheck` fields at all, so there was
nowhere for the values to go even if `buildRunConfig` had tried. The
runtime layer itself was fine: `buildCreateArgs`
(`pkg/container/common.go`) already emits `--restart`/`--health-*` from
`CreateConfig.Restart`/`.HealthCheck` for both Docker and Podman, and that
plumbing is exercised correctly by the persistent-component
(`pkg/container/lifecycle.go` → `buildNamedCreateConfig`) and
background-container (`pkg/workflow/background_container.go`) paths. The
only gap was `EphemeralConfig` never being given the values to forward
into `CreateConfig`.

No test anywhere in the repo covered `Restart`/`HealthCheck` for a
step-based container run — only the persistent-component path had
coverage.

## Changes

- `pkg/container/ephemeral.go`:
  - Added `Restart *RestartPolicy` and `HealthCheck *HealthCheck` fields
    to `EphemeralConfig`, mirroring `CreateConfig`'s fields.
  - `buildEphemeralCreateConfig` now copies `config.Restart`/
    `config.HealthCheck` onto the `CreateConfig` it builds (mirrors
    `lifecycle.go`'s `buildNamedCreateConfig` at lines 206-207).
  - `appendEphemeralPreviewFlags` now also emits the `--restart`/
    `--health-*` flags (via the existing `addRestartFlag`/
    `addHealthFlags` helpers from `pkg/container/common.go`) so
    `step.DryRun` preview output matches the real invocation.
- `pkg/runner/step/container_run.go`: `buildRunConfig` now populates
  `Restart: container.RestartPolicyFromStep(&run)` and
  `HealthCheck: container.HealthCheckFromStep(&run)`, reusing the
  existing, already-tested conversion helpers from
  `pkg/container/runspec.go` instead of duplicating the Compose `test:`
  resolution logic.
- `pkg/runner/step/container_runtime_fake_test.go`: added
  `TestContainerHandlerExecuteRunPassesRestartAndHealthCheckToDocker`,
  which runs a `type: container, action: run` step with both `restart:`
  and `healthcheck:` set against the fake logging `docker` shim and
  asserts the recorded `create` argv line contains `--restart
  on-failure:3`, `--health-cmd true`, `--health-interval 5s`, and
  `--health-retries 2`.

## Validation

- New test fails against pre-fix code:
  ```text
  --- FAIL: TestContainerHandlerExecuteRunPassesRestartAndHealthCheckToDocker
      does not contain "--restart\ton-failure:3"
      does not contain "--health-cmd\ttrue"
      does not contain "--health-interval\t5s"
      does not contain "--health-retries\t2"
  ```
- Passes after the fix:
  ```text
  --- PASS: TestContainerHandlerExecuteRunPassesRestartAndHealthCheckToDocker (19.35s)
  ```
- `go build ./...` — clean.
- `gofumpt -l` on all changed files — clean (no output).
- `go test ./pkg/runner/... ./pkg/container/...` — full packages pass, no
  regressions against the existing persistent-component `Restart`/
  `HealthCheck` tests (`pkg/container/lifecycle_test.go`,
  `pkg/container/runspec_test.go`, `pkg/container/common_test.go`).

## Follow-ups

- `validateRunAction` in `pkg/runner/step/container_run.go` still does not
  call `container.ValidateRunStep`, so an invalid `restart:`/`healthcheck:`
  value on a `run` step (e.g. an unknown policy or a negative retry count)
  surfaces as a raw docker/podman CLI error at execution time instead of
  the friendly `ErrInvalidContainerRestartPolicy`/
  `ErrInvalidContainerHealthCheck` Atmos error the emulator kind already
  gets via the same helper. Out of scope for this fix (which targets the
  silent value drop, not validation), but a natural next step since
  `ValidateRunStep` already exists and is fully tested.
