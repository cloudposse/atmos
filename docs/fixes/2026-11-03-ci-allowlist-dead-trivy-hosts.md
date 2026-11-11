# Fix: CI runner hardening no longer fails on unresolvable Trivy hostnames

**Date:** 2026-11-03

## Summary

The `[lint] Dockerfile` job in `test.yml` failed on every run, including on `main`, because the runner-hardening
agent aborted when it could not resolve two hostnames in the shared allowlist. The two hostnames are removed
from the allowlist, and the `trivy` endpoint group they formed is deleted along with its only reference.

## Context

The hardening agent resolves every allowed hostname when it starts. In the failing log it reported
`unable to resolve domain check.trivy.dev., status 3` (NXDOMAIN), reverted its changes, and exited. The
follow-up step then reported `DNS repair failed: StepSecurity agent is not active`, and the later SARIF upload
steps failed with `Path does not exist`, which is a consequence of the agent failure and not a separate problem.

`check.trivy.dev` and `get.trivy.dev` no longer resolve (checked against 8.8.8.8, the resolver the agent uses),
while the parent domain `trivy.dev` still does. Earlier runs on `main` passed, so the records disappeared
upstream recently. `check.trivy.dev` was also listed in `terraform-policy-scanners` and `terraform-scanner-cache`,
which `native-ci.yml` selects, so those jobs would have failed the same way.

## Changes

- `.github/allowlist.yaml`: removed `check.trivy.dev:443` from `terraform-policy-scanners` and
  `terraform-scanner-cache`, and removed the `trivy` group, which contained only the two dead hostnames.
- `.github/workflows/test.yml`: removed `trivy` from the policies of the `[lint] Dockerfile` job. The job keeps
  the GitHub groups it needs to download the Trivy binary.

Hardening stays enabled. Only entries that point at hostnames which no longer exist were removed.

## Validation

- Resolved every hostname in the allowlist against 8.8.8.8: after the change none are unresolvable. Before it,
  exactly these two were.
- `node --test .github/actions/harden-runner/runtime/*.test.cjs`: 99 tests pass.
- Loaded every remaining group through `readPolicy`: all 47 are valid, and the `[lint] Dockerfile` selection
  resolves to the four GitHub endpoints.
- The fix has not yet run in CI at the time of writing.

## Follow-ups

None.
