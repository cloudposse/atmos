# Fix: Retry the Semgrep image pull so Harden Runner's egress allowlist cannot fail the scan

**Date:** 2026-10-21

## Summary

The `Semgrep scan` job in `.github/workflows/codeql.yml` failed intermittently because `docker run` could not pull
the Semgrep image from Docker Hub. The pull is now a separate step that re-resolves the registry domains before each
attempt and retries with backoff.

## Context

The job runs under Harden Runner with `egress-policy: block`. Harden Runner allows the IP addresses it recorded when
it resolved each allowed domain. `registry-1.docker.io` resolves to many rotating IP addresses with a 30 second TTL.
In the failing run the agent recorded `44.215.247.20`, but `dockerd` connected to other registry addresses
(`98.94.205.75`, `50.16.26.52`, `44.212.194.123`, among others), which the agent logged as `ip address dropped`.
The pull failed with `dial tcp 98.94.205.75:443: connect: connection refused` and exit code 125.

The `Upload SARIF to code-scanning` step then failed with `Path does not exist: semgrep.sarif`, because the scan never
ran. That second error is a cascade of the failed pull, not a separate defect.

All of the required domains were already in `allowed-endpoints`, so the allowlist itself was correct.

## Changes

- Added a `Pull Semgrep image` step to the `semgrep` job in `.github/workflows/codeql.yml`. It resolves
  `registry-1.docker.io`, `auth.docker.io`, and `production.cloudfront.docker.com` immediately before each
  `docker pull`, so the agent records the current addresses, and retries up to five times with a growing delay.
- `Run Semgrep` is unchanged and finds the image already pulled. Harden Runner stays in `block` mode.

## Validation

- Parsed the workflow with a YAML loader and confirmed the step order: Harden Runner, Checkout, Pull Semgrep image,
  Run Semgrep, Upload SARIF.
- Ran the step's shell with a fake `docker` binary: it succeeded on the third attempt after two failures, and exited 1
  with the error annotation after five failures.
- Not validated: the fix against a real Harden Runner egress block. The failure depends on live Docker Hub DNS
  rotation and can only be confirmed by CI runs on this change.

## Follow-ups

If the pull still fails after the retries, the next step is to avoid Docker Hub for this job, for example by mirroring
the pinned image to a registry with a stable address. None is planned unless the failure recurs.
