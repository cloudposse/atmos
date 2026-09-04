# Fix: upload the DNS guard's own log so a residual hang is diagnosable

**Date:** 2026-09-04

## Summary

`docs/fixes/2026-09-03-harden-runner-windows-dns-restore-race.md` shipped `atmos-dns-guard` (a
scheduled task that resets a Windows runner's DNS if harden-runner's post step kills its agent
before the restore finishes) and explicitly noted its log (`$RUNNER_TEMP\atmos-dns-guard.log`)
was not uploaded anywhere - "the observable signal is the job-level outcome" only.

A residual occurrence surfaced on an unrelated PR (#3043, a trivial `runs-on/action` version
bump): `Acceptance Tests (windows, shard 5/10)` ran every step successfully, then hung during
post-job cleanup and was cancelled ~15 minutes later - the same symptom the guard exists to
prevent. The job's own runner-agent log showed the guard's scheduled task had registered and run,
but with no uploaded guard log, there's no way to tell whether it detected the stuck condition and
acted, detected it and *failed* to act, or never saw the condition this specific run - i.e.
whether this is the same race with a residual gap, or a distinct failure mode that only looks the
same from the outside.

## Fix

Added an `if: always()` `actions/upload-artifact` step immediately after every
`./.github/actions/windows-dns-guard` invocation (`build`, `terraform-registry-cache`, `test`,
`mock` in `test.yml`; the cache-warmup workflow's own job), uploading
`${{ runner.temp }}\atmos-dns-guard.log` as `dns-guard-log-${{ github.job }}-${{
strategy.job-index }}` (unique per job/shard), `if-no-files-found: ignore` (the guard is a no-op,
and produces no log, on non-Windows legs or when DNS never needed repair), 3-day retention.

This does not fix a known root cause - none is confirmed yet. It closes the diagnostic gap so the
next occurrence of "guard ran, job still hung" produces the guard's own log instead of only the
job-level outcome, which is what's actually needed to tell whether this is the original DNS race
recurring past the guard's coverage, or something new.

## Validation

- `atmos ci validate .github/workflows/test.yml .github/workflows/setup-go-cache-warmup.yml` -
  both valid.
- Not validated live: needs a real Windows job run (ideally one where the guard actually fires) to
  confirm the artifact uploads as expected.

## Follow-ups

- Once a guard log from an actual hang is captured, revisit whether this is the same DNS race with
  a coverage gap or a distinct failure mode, and fix accordingly.
