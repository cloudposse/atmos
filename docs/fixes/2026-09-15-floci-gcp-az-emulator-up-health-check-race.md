# Fix: `atmos emulator up gcp/azure` reported unhealthy — floci-gcp/floci-az dropped curl, health check probed with a binary that no longer exists

**Date:** 2026-09-15

## Summary

`[floci] go e2e` failed with:

```text
--- FAIL: TestScaffoldGCPLandingZoneFlociE2E
    Error: component execution failed: up "gcp": container did not become healthy: dev/emulator/gcp reported unhealthy
--- FAIL: TestScaffoldAzureLandingZoneFlociE2E
    Error: component execution failed: up "azure": container did not become healthy: dev/emulator/azure reported unhealthy
```

Reproduced locally (`atmos emulator up gcp -s dev` against a scaffolded
`gcp/landing-zone` project): the container itself started instantly
(`floci-gcp 0.9.0 native ... started in 0.019s`), but `docker inspect`'s
health log showed every probe failing identically:

```text
"Output":"/bin/sh: line 1: curl: command not found\n"
```

`floci/floci-gcp:latest` and `floci/floci-az:latest` are GraalVM native-image
builds with no HTTP client binary at all -- `docker exec`'ing into a
container and running `command -v curl` finds nothing; the entire userland is
GNU coreutils + bash, no networking tools. The health check
(`curl -s -o /dev/null http://localhost:<port>/ || exit 1`) failed every
single probe with "command not found," which is a permanent, not transient,
condition -- no amount of waiting fixes a missing binary.

## Context (including a wrong first attempt)

The first fix attempt (same day, earlier commit on this branch) assumed the
failure was a *startup-speed* race -- the container takes longer than the
health check's 60s total budget (10s start_period + 5 retries * 10s
interval) to start listening under CI load, echoing
`docs/fixes/2026-08-31-floci-azure-health-check-race.md`, which fixed an
identical-looking symptom (`floci-az` failing a readiness check) at a
*different* chokepoint (`tests/floci_harness_test.go`'s
`requireFlociEndpoint`, which polls a *pre-existing* CI-level service
container's HTTP endpoint from the Go test process itself, not a Docker
`HEALTHCHECK`). That fix raised `floci/gcp`'s and `floci/az`'s
`start_period` to 40s (~90s total budget) and was pushed, but the next CI run
still failed -- just later (84s instead of 54s), which is exactly what a
fixed-but-not-actually-fixed timing budget looks like. That was the signal to
stop guessing and reproduce locally instead:

- `docker logs` on the CI-level *service* containers (started once at job
  setup, queried directly by `TestGCPSecretsFlociE2E`/`TestAzureSecretsFlociE2E`)
  showed clean, fast startup with no Docker `HEALTHCHECK` configured on those
  service definitions at all -- they were never exercising this code path,
  which is why they always passed and gave no signal that curl was missing.
- `atmos emulator up gcp -s dev` and `atmos emulator up azure -s dev` create
  a *separate*, fresh container each, through `pkg/container`'s own Docker
  `HEALTHCHECK`-based `WaitHealthy` (`pkg/container/wait.go`) -- this is the
  only place that actually runs the curl probe, and the only place that could
  have caught this.
- `floci/floci:latest` (AWS) does still ship curl (`docker exec ... command -v
  curl` → `/usr/bin/curl`, and a probe returns `200`), which is why
  `floci/aws` never showed any symptom and why the original bug report looked
  identical to a "gcp/az are just slower" pattern.

## Changes

A second session (`osterman/floci-health-check-failures`) hit and fixed the
same root cause independently and landed first (squash-merged into `main` as
part of #3165); this branch's own fix was superseded by that version on
merge rather than kept side-by-side. The adopted fix:

- `pkg/emulator/driver/floci.go`: `flociHealthCheck`'s probe command changed
  from `curl -s -o /dev/null http://localhost:<port>/ || exit 1` to
  `bash -c 'exec 3<>/dev/tcp/127.0.0.1/<port>'` -- a pure bash TCP-connect
  test using bash's built-in `/dev/tcp` pseudo-device, which needs only
  `bash` (present in all three Floci images) and no external HTTP client
  binary. `exec 3<>...` opens the connection on fd 3 without writing
  anything into the socket (unlike this branch's own first pass at the same
  idea, which wrote a newline via `echo >`) -- purely a connect test, and
  bash's own exit status on a failed connection makes the command fail
  without needing an explicit `|| exit 1`. Applied to all three variants
  (not just gcp/az) for consistency and so a future image change on any of
  them can't silently reintroduce this failure mode.
- `pkg/emulator/driver/builtin.go`: no `start_period` change was needed or
  kept -- `floci-gcp`/`floci-az` start in ~0.02s, so the original uniform
  `10s` default was never the problem.
- `pkg/emulator/driver/health_restart_test.go`: `TestFlociHealthCheck_Readiness`
  actually executes the probe command via `exec.CommandContext` against a
  real listening `net.Listen` port (expect success) and then the same port
  after closing it (expect failure without hanging) -- a stronger regression
  test than this branch's own first pass, which only asserted the probe
  string didn't contain `"curl"`/`"wget"`.

## Verification

- Reproduced the original failure locally: `atmos emulator up gcp -s dev`
  against a scaffolded project failed identically to CI
  (`container did not become healthy: dev/emulator/gcp reported unhealthy`),
  and `docker inspect --format '{{json .State.Health}}'` on the resulting
  container showed the `curl: command not found` output directly.
- After merging in the adopted fix, rebuilt `atmos` and re-ran
  `atmos emulator up gcp -s dev` and (with `FLOCI_AZ_TLS_ENABLED=true`)
  `atmos emulator up azure -s dev` against freshly scaffolded projects --
  both now come up in a few seconds, then torn down cleanly with
  `emulator down`.
- `go build ./...` clean.
- `go test ./pkg/emulator/...` — all pass, including
  `TestFlociHealthCheck_Readiness` and `TestFlociHealthCheck_UsesItsOwnPort`.
- `gofumpt -l` clean; patch-scoped `custom-gcl run --new-from-rev=origin/main`
  reports 0 issues.

## Follow-ups

None. Root-caused independently via local reproduction, confirmed the fix
that landed on `main` addresses the same root cause, and verified it directly
against both the GCP and Azure emulators.
