# Fix: local socket-mounted job containers now try the emulator's own network before guessing a gateway

**Date:** 2026-08-14

## Summary

A Terraform provider running inside a socket-mounted job container reproduced locally (not in
GitHub Actions) got `connection refused` against the emulator endpoint Atmos injected. Atmos was
guessing the docker0 bridge-gateway address (typically `172.17.0.1`) instead of using the more
reliable same-network container-to-container alias, because the alias path was gated behind an
explicit opt-in env var or `GITHUB_ACTIONS=true` — neither of which a local reproduction sets.

## Context

`pkg/emulator/endpoint_host.go`'s `useCurrentContainerNetwork()` decided whether callers
(`pkg/emulator/manager.go`'s `endpoint()` and `attachSharedNetwork()`) try
`currentContainerNetwork()` (an alias on the same Docker network as the emulator container) before
falling back to `reachableHostForPublishedPorts()`'s bridge-gateway guess. The default (no explicit
override) previously required `GITHUB_ACTIONS == "true"`, so a local containerized reproduction —
even though `processRunsInContainer()` correctly detects it — always fell straight to the
gateway guess. That guess is only reachable when Docker's userland-proxy binds the published port
on the bridge interface; with `--userland-proxy=false`, a custom network, or `--network host`,
nothing is listening there, producing `connection refused` rather than a timeout.

Both call sites already treat `currentContainerNetwork()` as best-effort: it returns `""` on any
failure (missing hostname, failed `Inspect`, or no non-`host`/`none` network), and callers fall
back to the gateway guess automatically in that case. So broadening the default gate to any
detected containerized run — not just GitHub Actions — is safe and strictly additive.

## Changes

- `pkg/emulator/endpoint_host.go`: `useCurrentContainerNetwork()`'s no-override default now
  returns `processRunsInContainer()` directly instead of
  `envString("GITHUB_ACTIONS") == "true" && processRunsInContainer()` — the same containerization
  detection `reachableHostForPublishedPorts()` already uses elsewhere in this file. Explicit
  overrides (`ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK=true/false`) are unchanged, as is
  non-containerized (`localhost`) and GitHub Actions behavior — this is a strict superset of the
  prior `true` cases.

## Validation

- `go test ./pkg/emulator/... -run 'TestUseCurrentContainerNetwork|TestCurrentContainerNetwork|TestReachableHostForPublishedPorts|TestFirstReachableNetwork|TestParseLinuxDefaultGateway' -v` —
  all pass, including a table-driven rewrite of the old
  `TestUseCurrentContainerNetworkRequiresActionsOrOverride` (renamed `TestUseCurrentContainerNetwork`)
  covering: containerized with no opt-in (now `true`, the regression proof), non-containerized
  (`false`), `GITHUB_ACTIONS=true` (unchanged `true`), and explicit overrides. New
  `TestCurrentContainerNetwork_UndeterminableFallsBackToEmpty` confirms a containerized run whose
  network still can't be determined (e.g. only a `host` network reported) returns `""`, so callers
  still fall back to the gateway guess rather than erroring.
- `go test ./pkg/emulator/...` — full package, all pass.
- `go build ./...` and `go vet ./pkg/emulator/...` — clean.
- Windows is unaffected: `processRunsInContainer()` already returns `false` there (no `/proc/*`),
  matching `reachableHostForPublishedPorts()`'s existing Windows behavior.

## Follow-ups

None.
