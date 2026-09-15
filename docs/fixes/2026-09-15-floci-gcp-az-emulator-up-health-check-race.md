# Fix: `atmos emulator up gcp/azure` flipped to "unhealthy" under CI load — give floci/gcp and floci/az a longer health check start_period

**Date:** 2026-09-15

## Summary

`[floci] go e2e` failed twice in a row (two independent CI runs of the same
PR) with:

```text
--- FAIL: TestScaffoldGCPLandingZoneFlociE2E (54-56s)
    Error: component execution failed: up "gcp": container did not become healthy: dev/emulator/gcp reported unhealthy
--- FAIL: TestScaffoldAzureLandingZoneFlociE2E (53-56s)
    Error: component execution failed: up "azure": container did not become healthy: dev/emulator/azure reported unhealthy
```

Both failures landed immediately after `TestScaffoldAWSLandingZoneFlociE2E`
(166-172s) and `TestScaffoldAWSAppFlociE2E` (61-62s) in every run, and both
failed at almost exactly the same elapsed time (~54-56s) both times — not
random jitter, a deterministic budget being exceeded.

## Context

`pkg/emulator/driver/floci.go`'s `flociHealthCheck` (via
`pkg/emulator/driver/builtin.go`'s `shellHealthCheck`) gave every Floci
variant (`floci/aws`, `floci/gcp`, `floci/az`) the same Docker health check
timing: `start_period: 10s`, `interval: 10s`, `retries: 5`, `timeout: 5s`.
Docker only starts counting failed probes toward `retries` after
`start_period` elapses, so the total time-to-terminal-"unhealthy" is
`start_period + retries * interval` = 60s. `pkg/container/wait.go`'s
`WaitHealthy` deliberately fails fast the moment the container runtime
reports the terminal "unhealthy" state (see its doc comment) rather than
waiting out its own 90s poll budget — that's by design, since Docker itself
already gave the container a start_period + retries budget before flipping
to that terminal state.

`floci/gcp` (a Quarkus JVM with a warmup cost) and `floci/az` (which
additionally generates a TLS cert on first boot when `FLOCI_AZ_TLS_ENABLED`
is set, per `tests/scaffold_floci_test.go`) are known to be slower to start
listening than `floci/aws` — `docs/fixes/2026-08-31-floci-azure-health-check-race.md`
already documented and fixed the identical symptom (both images racing their
readiness check) for a *different* chokepoint: the CI-level
`requireFlociEndpoint` gate in `tests/floci_harness_test.go`, which now polls
for up to 90s instead of checking once. That fix did not touch this second,
independent readiness gate: `atmos emulator up`'s own Docker health check for
containers it spins up fresh per test (`pkg/container.WaitHealthy`), which
`TestScaffoldGCPLandingZoneFlociE2E`/`TestScaffoldAzureLandingZoneFlociE2E`
exercise via the scaffolded landing-zone stack's `emulator` component. The
60s budget here was tight enough that CI-runner load right after the two
heavy AWS scaffold tests (which together run ~4 minutes of real Terraform
work against the AWS emulator, saturating the Docker daemon/CPU on the shared
runner) consistently pushed `floci/gcp`/`floci/az` past it, twice in a row.

Ruled out before concluding this was a real, fixable timing gap rather than a
one-off flake:
- Checked the CI-level service containers' own logs (`docker logs` from the
  job's "Stop containers" step) — both `floci-gcp` and `floci-az` started
  cleanly with no errors, ruling out an image/config regression.
- Confirmed `floci/aws` has never shown this race across every run examined,
  narrowing the fix to the two images already known (from the prior fix) to
  be startup-slow.
- Confirmed the test's own context timeout (`3*time.Minute` in
  `tests/scaffold_floci_test.go`'s `runScaffoldTerraformE2E`) is nowhere near
  the actual constraint — the bottleneck is strictly Docker's own
  `start_period`/`retries` math, confirmed by both failures landing within a
  couple seconds of the computed 60s budget.

## Changes

- `pkg/emulator/driver/builtin.go`: added `shellHealthCheckWithStartPeriod`,
  factored out of `shellHealthCheck` (which now just calls it with the
  existing `"10s"` default) so a driver can opt into a longer start_period
  without a new health-check builder.
- `pkg/emulator/driver/floci.go`: `flociHealthCheck` now takes a
  `startPeriod` parameter. `floci/aws` keeps `"10s"`; `floci/gcp` and
  `floci/az` get the new `flociGCPAzStartPeriod = "40s"`, bringing their
  total budget to ~90s (40s + 5 * 10s) — matching the tolerance the prior fix
  already established works for the same two images.
- `pkg/emulator/driver/health_restart_test.go`: added
  `TestFlociHealthCheck_GCPAzGetLongerStartPeriod`, asserting `floci/gcp` and
  `floci/az` get `flociGCPAzStartPeriod` while `floci/aws` keeps the shared
  `"10s"` default — guards against silently regressing back to the tight
  shared budget.

## Verification

- `go build ./...` clean.
- `go test ./pkg/emulator/... -v` — all pass, including the new
  `TestFlociHealthCheck_GCPAzGetLongerStartPeriod` and the existing
  `TestBuiltinDrivers_HealthCheckDefault`/`TestFlociHealthCheck_UsesItsOwnPort`
  (unaffected — neither asserts a specific `start_period` value).
- `gofumpt -l` clean on all changed files.

## Follow-ups

None. This is a self-contained timing-tolerance fix with a regression test;
no further tracking needed.
