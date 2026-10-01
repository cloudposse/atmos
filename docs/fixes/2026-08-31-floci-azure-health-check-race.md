# Fix: retry the Floci endpoint health check instead of checking once

**Date:** 2026-08-31

## Summary

`TestAzureSecretsFlociE2E` failed in the `[floci] go e2e` CI job (GitHub Job ID 99670860774) with
`Get "http://localhost:4577": context deadline exceeded` / `Floci HTTP endpoint is not reachable at
http://localhost:4577`, even though the TCP dial to that port had already succeeded. Every other
Floci test in the same run passed, including the Azure landing-zone scaffold test, which also
exercises the Azure emulator successfully. `tests/floci_harness_test.go`'s `requireFlociEndpoint`
checked the endpoint exactly once with a 2-second timeout; it now polls for up to
`flociStartupTimeout` (90s), reusing the same budget the local testcontainers auto-start path
already grants for exactly this kind of cold start.

## Context

CI runs Floci as three separate GitHub Actions `services:` containers (`floci`, `floci-gcp`,
`floci-az`), started via `.github/workflows/test.yml`. GitHub Actions only waits for a service
container's process to start, not for the application inside it to actually begin serving
requests -- none of the three service definitions have a `--health-cmd` configured. The Go test
suite's own `requireFlociEndpoint` is therefore the only readiness gate in CI, and it did a single
TCP dial plus a single HTTP GET, each with a 2-second timeout.

The failure log shows the TCP dial succeeded (no "Floci is not reachable" error) but the
subsequent HTTP GET timed out -- the socket was accepting connections before the HTTP handler
inside the container was ready to respond, a well-known startup-race pattern for emulator/proxy
servers that bind their listener early. `tests/floci_containers_test.go`'s `startFlociContainer`
(used when tests auto-start Floci locally via testcontainers, not in CI where endpoints are
pre-supplied) already accounts for this: it waits via `wait.ForHTTP("/").WithStartupTimeout(flociStartupTimeout)`
with a 90-second budget, specifically for `floci-az`, which additionally sets
`FLOCI_AZ_TLS_ENABLED: "true"` and likely needs to generate a TLS certificate on startup, making it
plausibly slower to become ready than the other two emulators. The CI-path check never had an
equivalent tolerance.

## Changes

- `tests/floci_harness_test.go`: `requireFlociEndpoint` now polls the combined TCP-dial-then-HTTP-GET
  check via a new `pollUntil` helper, bounded by the existing `flociStartupTimeout` (90s, declared in
  `floci_containers_test.go`) instead of trying once. The final error message includes the elapsed
  budget so a genuine misconfiguration (wrong port, service never starts) still fails loudly, just
  with the same patience the local auto-start path already has.
- Added `pollUntil`, a small generic retry-until-success-or-timeout helper (500ms interval),
  and three unit tests (`TestPollUntilSucceedsImmediately`,
  `TestPollUntilRetriesUntilSuccessWithinBudget`, `TestPollUntilReturnsLastErrorOnTimeout`)
  covering it directly, independent of any real Floci endpoint.

## Verification

- `go build ./tests/...` and `go vet ./tests/...` clean.
- `go test ./tests -run TestPollUntil` passes (3/3).
- `go test ./tests -run TestAzureSecretsFlociE2E` skips cleanly locally (no `ATMOS_TEST_FLOCI`
  set), confirming the skip path is unaffected.
- `gofumpt -l` clean; patch-scoped `./custom-gcl run --new-from-rev=origin/main ./tests/...`
  reports 0 issues.
