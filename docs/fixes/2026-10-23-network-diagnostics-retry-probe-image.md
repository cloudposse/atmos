# Fix: runner network diagnostics retry the probe image download in the hardened phase

**Date:** 2026-10-23

## Summary

The "DNS and egress (repair first)" diagnostics job failed intermittently with
`hardened: incomplete`. The `prepare` step's `docker pull` ran immediately after the pre-job DNS
repair, before Docker's daemon had picked up the repaired resolver, so the pinned curl image was
missing and the hardened phase skipped its container probes. Only the repaired phase retried the
download. The hardened phase now also waits for Docker's resolver cache window and retries the
download when the image is still missing.

## Context

`.github/actions/harden-runner/diagnostics/diagnostics.cjs` marks a phase `incomplete` when the
pinned image is unavailable, and `report` with `repair-first: true` requires both phases to be
complete. The evidence artifact for run 39887430529, sample 6, recorded "Container probes
unavailable: pinned image was not downloaded." in `hardened.log`, while the repaired phase
downloaded the image and passed. Samples 4 and 5, with the same repair-first shape, passed because
their first pull happened after Docker's five-second resolv.conf cache had expired. The race was
introduced with the shared hardening action in #3493 and reproduces on any branch.

## Changes

- `diagnostics.cjs`: before snapshotting, any phase whose pinned image is missing waits 6 seconds
  and downloads it again; the repaired phase keeps its unconditional wait and download. A phase
  whose image is already present no longer waits.
- `diagnostics.test.cjs`: a hardened probe whose first download failed retries once after the
  cache window and completes all twelve probes; a download that keeps failing still reports
  `hardened: incomplete`; the lifecycle test asserts only one cache wait when the image is present.
- `diagnostics/README.md`: documents the retry.

## Validation

- `npm test` in `.github/actions/harden-runner`: 87 tests pass.
- The failing run's artifact (`runner-network-6-1`) confirmed the missing-image cause; the fix
  cannot be reproduced locally without the StepSecurity agent, so the next diagnostics runs on
  #3471 and #3481 are the live check.

## Follow-ups

None.
