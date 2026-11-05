# Fix: Manager-owned deferred authentication

**Date:** 2026-09-24

## Summary

List and describe callers pass one `AuthManager` handle. Deferred authentication
and explicit disable are manager state, not a second resolver carried beside it.

## Context

The previous implementation passed a nil manager plus `DeferredAuth` and an
authentication-disabled flag. Callers had to interpret those values together;
nil could mean either disabled authentication or authentication not attempted yet.

## Changes

- `pkg/auth/deferred.Manager` implements the existing `AuthManager` interface.
  Construction and metadata access do not initialize providers or authenticate.
- The manager owns explicit disable and the invocation-local cache of successful
  and failed authentication, keyed by effective configuration and stack.
- Removed the separate `DeferredAuth` interface and fields from command options.
  Configuration-based evaluators receive the same manager through a runtime-only
  field, not a second resolver or a separate authentication policy.
- Scoped and section-filtered describe calls derive policy from their manager.
  Existing execution/preflight compatibility entry points remain available.
- Explicit identity selection still authenticates at the command boundary.
  Unused values still do not authenticate, and requested unavailable values retain
  their existing warn, silent, or strict behavior.
- Added contract tests for manager delegation and regressions that pass only a
  manager to the evaluation pipeline, including explicit disable without skipping
  requested credential-backed functions.

## Validation

- Focused list/describe authentication, evaluation, inventory, secret, and
  degradation tests passed in `cmd`, `cmd/list`, `pkg/list`, and `internal/exec`.
- Race tests passed for `pkg/deferred` and the auth, store, secret, and stack
  deferred adapters. Local package statement coverage: 99.5%, 99.2%, 100%, 100%,
  and 100%, respectively; these are not Codecov's PR-wide patch percentages.
- Patch-scoped `atmos fix lint` passed with zero findings.
- `go build ./...`, the workspace binary build, and `atmos test` scoped to the
  five deferred packages passed. Full `cmd/list` and `pkg/list` short tests passed.
- Website production build and fix-record validation passed.
- With the emulator stopped, the workspace binary listed all three demo stacks
  in strict default, JSON, and tree/provenance output, including explicit disabled
  authentication. Explicit `--identity=local` still failed. The demo configuration,
  emulator, and installed binary were not changed.

## Follow-ups

None.
