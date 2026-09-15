# Fix: Floci health check no longer assumes `curl` is present in every image

**Date:** 2026-09-15

## Summary

`flociHealthCheck` in `pkg/emulator/driver/floci.go` probed floci containers exclusively with
`curl`. The minimal GCP and Azure floci images no longer ship `curl`, so the health check probe
itself failed to execute against those images (the emulator was up; the check couldn't run),
breaking CI jobs that spin up the GCP/Azure floci emulator across every open PR.

## Context

Reported as "floci broke something, tests are failing on the floci health check" with the failure
observed in CI. Investigation traced it to the floci vendor dropping `curl` from newer minimal
images (confirmed via a failing floci E2E job:
https://github.com/cloudposse/atmos/actions/runs/34981357896/job/104426326374). An equivalent fix
already existed, unmerged, bundled inside an unrelated docs/website PR (#3170, branch
`osterman/roadmap-changelog-view`). Because the underlying bug affects every open PR that exercises
the GCP/Azure floci emulator in CI, this fix is extracted and landed on its own so it can merge to
`main` independently of that larger PR.

## Changes

- `pkg/emulator/driver/floci.go`: `flociHealthCheck` now prefers the image's own
  `/usr/local/bin/healthcheck.sh` readiness script when present, and falls back to the previous
  `curl`-based probe only when that script is absent (older/legacy images). A failing native probe
  is not masked by a curl fallback — it must remain a failure.
- `pkg/emulator/driver/floci_health_test.go` (new): `TestFlociHealthCheck_Readiness` exercises the
  generated shell probe against faked `healthcheck.sh`/`curl` executables on a temp `PATH`,
  covering native-only success, native failure not falling back to curl, legacy curl-only success,
  and legacy curl-only failure. Skipped on Windows since the probe runs in a POSIX shell.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/emulator/driver/... -run 'TestFlociHealthCheck|TestBuiltinDrivers_HealthCheckDefault|TestFlociHealthCheck_UsesItsOwnPort' -v` — all pass, including the new `TestFlociHealthCheck_Readiness` subtests.
- `atmos lint --changed` — 0 issues.
- Not run: a live floci E2E test against real GCP/Azure containers (no Docker/Podman available in
  this session). Relying on CI's floci E2E job on the PR to confirm the native probe path executes
  against the real images.

## Follow-ups

None.
