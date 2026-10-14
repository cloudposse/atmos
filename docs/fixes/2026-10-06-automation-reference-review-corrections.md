# Fix: Correct automation process and Git-hook documentation

**Date:** 2026-10-06

## Summary

Correct the Atmos Pro examples from PR #3274 and the Git-hook and concurrent-step
references from PR #3283. This consolidated record lives on the top stack layer
so PR #3274 retains its existing 149 changed files.

## Context

The Pro reference overstated cleanup after deployment failures and excluded
configured-token authentication. The Git-hook reference promised preparation and
stdin behavior beyond what command and step handlers provide. One concurrent-step
validation note omitted the supported `script` type.

## Changes

- PR #3274: limit the unlock guarantee with `check = False` to ordinary nonzero
  exits. Explain that process-start failures, cancellation, and signal termination
  can stop execution before unlock, leaving the lock until its 300-second TTL.
- PR #3274: document `settings.pro.token` and `ATMOS_PRO_TOKEN` for bearer-token
  authentication of lock and unlock outside connected GitHub Actions pipelines.
  The API still determines whether the supplied token authorizes the request.
- PR #3283: describe command hooks as using the shared shell runner with the
  inherited process environment. Scope argument and stdin forwarding to command
  hooks and state that step stdin handling depends on the step type.
- PR #3283: include `script` in the concurrent-child validation note.

Rename the SDK PRD to `atmos-sdk.md` and describe the integration problem and
full Atmos capability scope. Keep implemented execution interfaces distinct from
the proposed public SDK, and update the related PRD links.

## Validation

- Checked `NewAtmosProAPIClientFromEnv`, the environment-variable binding, and
  existing process-result tests covering ordinary exits, startup failure,
  cancellation, and signals.
- Checked Git-hook command and step execution paths and
  `validateConcurrentChild` against the revised descriptions.
- Both patches apply cleanly to their owning PR heads and pass
  `git diff --check`.
- The consolidated website build has not run as part of these wording changes;
  it is handled with the complete stack.

## Follow-ups

None.
