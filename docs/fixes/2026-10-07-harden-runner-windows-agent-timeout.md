# Fix: Bump `step-security/harden-runner` to v2.22.0 for the Windows agent timeout

**Date:** 2026-10-07

## Summary

Pin `step-security/harden-runner` to v2.22.0 (`351661ca32ac09a36dc5ee2d536e3128f2a3c8ed`) in every workflow so Windows jobs stop failing during the action's own startup step.

## Context

Across the `ci` automation module stack (#3298, #3307, #3308), Windows jobs (`Build (windows)`, `Acceptance Tests (windows, shard N/10)`, `Terraform registry cache test (windows)`, `[mock-windows] examples/*`) failed before any Atmos step ran. The job logs show the harden-runner Windows agent v1.0.7 reporting `Error Sending runner status Started: API call error, status code: 400` and the step ending with `timed out`. Dependent jobs were then skipped and the aggregate jobs failed. Re-running cleared each occurrence, which pointed at the agent rather than the repository. The pinned action version was v2.21.1; v2.22.0 bumps the Windows agent to v1.0.10.

## Changes

- Replaced all 66 `step-security/harden-runner@e14015d5… # v2.21.1` references under `.github/` (workflows with both `.yml` and `.yaml` extensions) with `351661ca… # v2.22.0`.
- No workflow logic or egress policy changed.

## Validation

- `atmos ci validate` reports all GitHub Actions workflow files valid.
- `grep -r e14015d583714f6e62063499dc959a02595150a1 .github` returns nothing.
- Confirmed in CI by re-running the Windows jobs on the stack after the bump (tracked on the PRs).

## Follow-ups

None.
