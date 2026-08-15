# Fix: containers and emulators now join a shared per-stack network, like Docker Compose

**Date:** 2026-08-14

## Summary

`components.container` (Atmos's native single-container component) had no automatic networking:
`ExecuteUp`'s `NamedConfig` and `ExecuteRun`'s `EphemeralConfig` never populated `Networks`, so every
plain container landed on the default bridge with no inter-container DNS — only host-published ports
worked. The `emulator` component type already solved this for itself (`pkg/emulator/manager.go`'s
`attachSharedNetwork`: every emulator in a stack joins a per-stack network and gets a DNS alias). Every
container component, one-shot `atmos container run`, and stack-scoped workflow `type: container` step now
joins that same shared network automatically, with no configuration required — similar to the default
network `docker compose` creates for a project.

## Context

Investigating an unrelated field-test finding surfaced the gap, and it was confirmed as an oversight: a
container-based service and an emulator in the same stack should be able to resolve each other by name,
the way services in one `docker-compose.yaml` project can. Two design decisions were made before
implementing:

1. **One shared network per stack**, used by both `components.container` instances and emulators — not
  two separate per-kind networks, which would defeat cross-kind resolution.
2. **Both long-lived (`up`) and ephemeral (`run`) containers** join the network (full parity with
  `docker compose up` vs `docker compose run`).

An initial pass left workflow `type: container` steps out of scope, reasoning that `WorkflowStep` had no
stack/component identity to key a network alias off of. That reasoning was wrong: `WorkflowStep.Stack`
already exists and is already the established pattern three other step types
(`pkg/runner/step/emulator.go`, `tflint.go`, `store.go`) use to resolve an effective stack (step-level
override → `--stack`/`ATMOS_STACK` → empty). Leaving workflow-driven one-shot containers unable to reach
the very services this change makes reachable would have made the feature far less useful for its most
common real use case — a workflow step running tests or commands against `components.container`/emulator
services — so this was corrected in the same fix rather than deferred.

Still explicitly out of scope: Compose's top-level `networks:`/`external:`/per-network `aliases:`,
scaled/replica DNS, and network teardown/reference-counting (matches the existing emulator precedent of
"create once via idempotent `network create`, never delete" — no code in the repo deletes networks).

## Changes

- New `pkg/container/stack_network.go`: `StackNetworkName(stack)` (`"atmos-" + sanitized stack`, renamed
  and unified from the emulator-only `"atmos-emulator-" + stack`), `StackNetworkAlias(stack, name)`
  (`<stack>-<component>`), and `AttachSharedNetwork(ctx, runtime, *[]NetworkAttachment, stack, name)` —
  generalized from the emulator's own `attachSharedNetwork`, so both component kinds call the same
  implementation instead of maintaining parallel copies.
- New `pkg/container/current_network.go`: `ProcessRunsInContainer` (exported var), `PreferCurrentContainerNetwork()`,
  and `CurrentContainerNetwork(ctx, runtime)` — moved (not duplicated) from `pkg/emulator/endpoint_host.go`,
  since `components.container` needs the identical "prefer Atmos's own current container network" check
  emulators already use, so a job container that starts a sibling container can still reach it. Same env
  var (`ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK`) and fallback semantics, unchanged behavior.
- `pkg/emulator/manager.go` / `endpoint_host.go`: shrink to thin delegation over the new `pkg/container`
  functions; `pkg/emulator/endpoint_host.go` keeps only what's genuinely emulator-specific
  (`reachableHostForPublishedPorts`, `linuxDefaultGateway` and friends — gateway-guessing for reading a
  published port, unrelated to network *attachment*).
- `pkg/container/ephemeral.go`: `EphemeralConfig` gains a `Networks []NetworkAttachment` field, wired into
  `buildEphemeralCreateConfig`; `addNetworkFlags` already applied `CreateConfig.Networks` for both Docker
  and Podman, so no runtime-level changes were needed.
- `pkg/component/container/executor.go`: `ExecuteUp` and `ExecuteRun` both call `AttachSharedNetwork`
  after the runtime is resolved.
- `pkg/runner/step/container_run.go`: new `resolveContainerStepStack(step, vars)` resolves the effective
  stack for a `type: container, action: run` step (`step.Stack` → `vars.Flags["stack"]` →
  `vars.Env["ATMOS_STACK"]` → `""`, mirroring the existing pattern in `emulator.go`/`tflint.go`/`store.go`);
  `executeRun` calls `AttachSharedNetwork` with that stack and `containerStepIdentity(step.Name)` (defaults
  to `"step"`) as the alias, skipping attachment entirely when no stack resolves.
- `pkg/container/stack_network.go`: new `HasExplicitNetworkOverride(runArgs)` — Docker/Podman reject
  combining a network mode like `host`/`none` with an additional `--network` attachment, so a workflow step
  already using `run_args: [--network=host]` would otherwise start failing outright once it also got the
  shared-network `--network` flag. `executeRun` now checks this before calling `AttachSharedNetwork`, so an
  explicit `--network` in `run_args` always wins and the shared network is skipped for that step.
- Docs: `docs/prd/git-server-emulator.md` and the relevant `website/docs/*.mdx` pages updated for the
  renamed/unified network, the new automatic-networking behavior, and workflow step coverage.

## Validation

- `go build ./...` and `go vet ./...` — clean.
- `go test ./pkg/container/... ./pkg/emulator/... ./pkg/component/container/... ./pkg/runner/step/... -v`
  — all pass, including new `TestAttachSharedNetwork_SharesNetworkAcrossKinds` (proves a container-style
  and an emulator-style call for the same stack land on the identical network), `TestExecuteUp_AttachesSharedNetwork`,
  `TestExecuteRun_AttachesSharedNetwork`, `TestBuildEphemeralCreateConfig_PassesThroughNetworks`,
  `TestResolveContainerStepStack` (table-driven, covers the full precedence chain and template-error
  propagation), `TestContainerHandlerExecuteRunAttachesSharedNetwork`/`_NoStackSkipsSharedNetwork` (real
  subprocess test via the fake-docker-on-PATH helper, asserting the actual recorded `network create` and
  `--network`/`--network-alias` CLI args), and the migrated `pkg/container/current_network_test.go`
  coverage.
- `atmos lint --changed` — 0 issues.
- `gofmt`/`gofumpt` — clean on all changed files.
- `cd website && npm run build` — succeeds; the only broken-anchor warnings reported are pre-existing and
  unrelated to this change (a different changelog post).
- Not yet run: a real Docker/Podman end-to-end smoke test (bring up two `components.container` instances
  in one stack, or a workflow `run` step alongside a persistent container, and confirm `getent hosts
  <stack>-<component>` resolves) — this environment has no container runtime available; recommend running
  this manually before release.

## Follow-ups

- Compose-parity features intentionally deferred: top-level `networks:`, `external: true` networks,
  per-network `aliases:`, scaled/replica DNS, and network teardown/reference-counting on `down`/`rm`.
- Native `components.container` (`ExecuteUp`/`ExecuteRun` in `pkg/component/container/executor.go`) never
  wires `run.run_args` into the container create args at all — a pre-existing gap, not introduced by this
  change, confirmed by grep. This means there's currently no way to request host networking (or any other
  raw runtime flag) for a native container component the way workflow `type: container` steps already can
  via `run_args`. Not fixed here since it's a separate, unrelated gap — the `AttachSharedNetwork` guard this
  change adds (`HasExplicitNetworkOverride`) is ready to use once `run_args` is wired there too.
- Workflow `type: container, action: build`/`push`/`inspect` steps don't join the network (only `run`
  starts a container that could reach peers). This matches the actual need — build/push/inspect don't
  talk to other stack services — so it isn't tracked as a gap.
