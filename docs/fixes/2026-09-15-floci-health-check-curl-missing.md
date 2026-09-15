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
  `/usr/local/bin/healthcheck.sh` readiness script when present, and falls back to a bare TCP
  connect via Bash (present in every Floci image, unlike `curl`) when that script is absent. A
  failing native probe is not masked by the fallback — it must remain a failure.
- `pkg/emulator/driver/floci_health_test.go` (new): `TestFlociHealthCheck_NativeScript` exercises
  the native-readiness-script branch against a faked `healthcheck.sh` on a temp `PATH`, covering
  native success and native failure not falling back. Skipped on Windows since the probe runs in a
  POSIX shell.

### Merge note (2026-09-15)

While rebasing onto `main`, an independent, differently-shaped fix for the same underlying bug had
already landed via PR #3165 (`fix: probe Floci readiness without curl`), replacing the `curl` probe
with a bare TCP connect for every image. Merging in `main` produced a real conflict in
`flociHealthCheck` (curl-fallback vs. TCP-fallback), not just a textual one: `curl` doing a plain
HTTP GET can hang or fail against a TLS-enabled endpoint (`floci-az` supports
`FLOCI_AZ_TLS_ENABLED`) or against a socket that's TCP-accepting but not yet serving HTTP, which is
exactly the false-negative pattern `main`'s new `TestFlociHealthCheck_Readiness` test asserts must
still count as healthy. Resolution: keep the native-script preference from this fix (the strongest,
vendor-provided signal, validated against real containers via PR #3171's `[floci] go e2e` CI job),
but use `main`'s TCP-connect approach as the fallback instead of `curl` — dropping the `curl` tier
entirely. This satisfies both test suites without reintroducing an HTTP/TLS-dependent probe.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/emulator/driver/... -run 'Floci|HealthCheck' -v` — all pass, including `main`'s
  own `TestFlociHealthCheck_Readiness` (TCP-listener-based) and this fix's new
  `TestFlociHealthCheck_NativeScript`.
- `go test ./pkg/emulator/...` — all pass.
- `atmos lint --changed` — 0 issues.
- Live floci E2E confirmation: `[floci] go e2e` passed on PR #3171 against real GCP/Azure
  containers before this merge; not re-run live post-merge in this session (no Docker/Podman
  available here) — relying on CI on the PR to reconfirm.

## Follow-ups

None.
