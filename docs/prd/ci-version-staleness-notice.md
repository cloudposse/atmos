# Automatic CI version staleness notice

## Problem and evidence

CI runs can continue using old Atmos releases without seeing an upgrade signal.
Previously, automatic version notices appeared only on root help and
`atmos version`, commands that CI workflows rarely invoke.

Anonymous aggregate telemetry motivating this change indicates that approximately
one-third of CI command volume comes from versions more than four months old.
Versions 24–43 releases behind also account for high command volume. These
aggregates do not identify individual users, organizations, or pipelines.

## Product behavior

During normal configured command execution in actual CI, prepare a background
check and surface one warning annotation per job when a stable binary is at least
10 minor versions behind the latest stable release in the same major. Skip
prereleases, development/build-metadata versions, invalid versions, and different
majors; tolerate a leading `v`. Minor differences are a proxy for releases, not a
count of patch releases or elapsed time.

The title is `Atmos <current> is <n> releases behind`, followed by an approximate
age when the binary contains a valid date, for example
`Atmos 1.100.0 is 10 releases behind (about 4 months old)`. The date is embedded at
build time in `pkg/version.BuildDate`. Release builds and the native build target
use the source commit's RFC 3339 timestamp, preserving reproducible builds. This
is an approximation of release age, not the GitHub publication date. Formatting
uses `pkg/duration` with its existing 30-day month and 365-day year conventions.
Missing, invalid, or future dates omit the age; no additional API request is made.
The age is informational and does not change the minor-version threshold.

The message is
`Latest is <latest>. ` followed by installation-aware upgrade guidance. Preserve
conditions on package-manager upgrade commands. Use the provider annotation seam
and stderr exclusively, avoiding the stdout corruption described in issue #3309
and `docs/fixes/2026-10-07-ci-legacy-action-stdout-pollution.md`.

Both `version.check.enabled` and `version.check.ci.enabled` must be enabled.
`ATMOS_VERSION_CHECK_ENABLED=false` is the master opt-out. The new CI flag defaults
to true and `version.check.minor_threshold` defaults to 10, with corresponding
`ATMOS_VERSION_CHECK_CI_ENABLED` and `ATMOS_VERSION_CHECK_MINOR_THRESHOLD`
overrides. Native CI mode is not required, and local `--ci` does not qualify.

## Resource budget and authentication

Gate in order on actual CI, enable flags, effective client authentication,
already-emitted state, and the existing frequency cache. Resolve credentials once
for the notice and reuse the client for the quota and latest-release requests.
Never infer effective authentication from token environment variables alone;
transport host/scheme restrictions may withhold a token.

Unauthenticated REST requests share a 60-request/hour budget per source IP.
GitHub Actions' `GITHUB_TOKEN` ordinarily has 1,000 requests/hour per repository,
shared by work using that repository budget; some Enterprise Cloud resources have
higher limits. Optional notices must not compete aggressively with the job's
actual work. Query `/rate_limit` and proceed only with valid core data and at least
75% remaining. The endpoint does not consume primary quota, but it can count toward
secondary limits. Do not retry failed requests or wait for quota recovery.
See [GitHub's rate-limit documentation](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api).

## Lifecycle and deduplication

Use one background deadline from `version.check.timeout` (default 1000 ms),
covering authentication, cancellation-aware cache locks, HTTP requests, and local
installer detection. Nonpositive timeouts and thresholds use defaults. At command
completion, consume only an already-ready result; cancel unfinished work without
waiting. Workers never emit annotations. All failures are debug-only and never
change command status. Very short commands can finish before any notice is ready.
Completed valid checks update the frequency timestamp atomically while preserving
other cached application state; failed/canceled checks do not advance it.

An atomic exclusive-create marker prevents concurrent emissions. GitHub markers
live under `RUNNER_TEMP` and include repository, run, attempt, and job identity.
The runner clears that directory at job boundaries, separating matrix jobs on
different runners and subsequent jobs on self-hosted runners. See the
[GitHub runner variable reference](https://docs.github.com/en/actions/reference/workflows-and-actions/variables).

After emission, set `ATMOS_CI_VERSION_NOTICE_SHOWN` for children and export it
through the provider's file-backed environment exporter for later steps.
Generic CI uses available job identity with a machine-local marker. Without a
reliable identity, use frequency-cache and inherited-sentinel deduplication.
Separate containers without shared files/environment cannot guarantee job-wide
deduplication. Marker failures skip emission. No marker is claimed for unfinished
checks, so a later invocation can try again.

Preserve diagnostic/discovery configuration bypasses. Local help/version checks
and explicit `atmos version --check` retain their behavior; automatic CI checks
use this service rather than the legacy synchronous path.

## Acceptance criteria

- Every failed gate suppresses latest-release requests and annotations.
- Both feature flags and environment precedence work; the master flag wins.
- Quota boundaries work for Actions and PAT budgets; missing/invalid quota data
  or lookup failures skip silently.
- Threshold edges, prefixes, prereleases, invalid versions, and different majors
  behave as specified. Embedded dates produce approximate ages; missing, invalid,
  or future dates leave the original title intact.
- Exactly one stderr annotation is emitted; stdout stays clean, including with
  generic providers and legacy GitHub Actions.
- Nested children, independent invocations, concurrent emissions, later steps,
  and job/attempt isolation exercise deduplication.
- Slow auth, HTTP, and installer dependencies cannot delay command completion or
  emit afterward. Race checks cover the asynchronous handoff.
- Existing local checks, explicit checks, and cache fields remain intact.

Ship implementation, tests, schema/config changes, and user documentation together
in one PR. No new telemetry collection or automatic upgrades are introduced.
Eligibility continues to use minor-version distance, independently of displayed age.
