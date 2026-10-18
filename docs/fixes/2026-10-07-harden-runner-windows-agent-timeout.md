# Fix: Bump `step-security/harden-runner` to v2.22.0 (Windows agent timeout is intermittent upstream)

**Date:** 2026-10-07

## Summary

Pin `step-security/harden-runner` to v2.22.0 (`351661ca32ac09a36dc5ee2d536e3128f2a3c8ed`) in every workflow. The bump is a harmless upgrade and stays, but it does **not** fix the Windows agent startup failure: the same signature was seen again with the new agent after the bump. The failure is intermittent on StepSecurity's side and clears on re-run.

## Context

Across the `ci` automation module stack (#3298, #3307, #3308), Windows jobs (`Build (windows)`, `Acceptance Tests (windows, shard N/10)`, `Terraform registry cache test (windows)`, `[mock-windows] examples/*`) failed before any Atmos step ran. The job logs show the harden-runner Windows agent reporting `Error Sending runner status Started: API call error, status code: 400` and the step ending with `timed out`. Dependent jobs were then skipped and the aggregate jobs failed. Re-running cleared each occurrence, which pointed at the agent rather than the repository.

The pinned action version was v2.21.1 (Windows agent v1.0.7). v2.22.0 bumps the Windows agent to v1.0.10. After the bump, a Windows job on the same stack failed with the identical signature under agent v1.0.10, so the agent version is not the cause.

## Changes

- Replaced all 66 `step-security/harden-runner@e14015d5… # v2.21.1` references under `.github/` (workflows with both `.yml` and `.yaml` extensions) with `351661ca… # v2.22.0`.
- No workflow logic or egress policy changed.

## Validation

- `atmos ci validate` reports all GitHub Actions workflow files valid.
- `grep -r e14015d583714f6e62063499dc959a02595150a1 .github` returns nothing.
- Windows jobs pass on the stack after re-running the affected jobs. One job failed with the same `status code: 400` + `timed out` signature under agent v1.0.10 and passed on re-run, which confirms the failure is intermittent and independent of the agent version.

## How to handle recurrences

- Signature: within the first lines of the job log, before any repository step, `Windows Agent process started`, then `timed out`, then `Error Sending runner status Started: API call error, status code: 400`. Linux and macOS jobs are unaffected.
- Re-run the failed jobs (`gh run rerun <run-id> --failed`). Do not bump the action again for this signature, and do not remove or `continue-on-error` the harden-runner step.
- If it becomes chronic, raise it on the `step-security/harden-runner` issue tracker.

## Follow-ups

None.
