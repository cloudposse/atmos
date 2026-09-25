# Fix: Subsystem-owned deferred resolution

**Date:** 2026-09-24

## Summary

Auth, stores, and secrets implement a shared deferred-value contract, while each
subsystem owns its dependencies. The common package no longer imports any of
those implementations or stack configuration.

## Context

Moving all authentication preparation into `pkg/auth/deferred` left auth parsing
secret declarations and configuring store clients. The common evaluation package
also owned store retrieval and auth-dependent stack cache keys. These dependencies
reversed ownership and made deferred evaluation look like an auth-specific system.

## Changes

- Added `deferred.Resolver[T]`, its closure adapter `Func[T]`, and opt-in `Once[T]`
  memoization. The mechanism knows neither credential configuration nor backends.
- Auth owns credential requests, identity selection, and invocation-keyed cached
  authentication successes/failures. Authentication remains serialized across
  effective identities because it can touch shared credential files/process state.
- `pkg/store/deferred` owns lazy YAML/template store values and store auth binding.
- `pkg/secrets/deferred` owns lazy secret values and backend credential preparation;
  masked secrets validate declarations without retrieving values or authenticating.
- `pkg/stack/deferred` owns configuration-specific evaluated-value cache keys.
- YAML/template adapters compose these implementations only for requested values.
  Display recovery remains at the value boundary; providers still classify errors.
- Added dependency-direction tests and regressions for unused values, composition,
  cached failures, concurrent resolution, caller identity preservation, and masking.

## Validation

- Package tests and race checks passed for the shared mechanism and all four
  subsystem adapters. Local coverage is 99.5% for `pkg/deferred`, 97.3% for
  `pkg/auth/deferred`, and 100% for the store, secret, and stack adapters. Subsequent
  manager-state consolidation and its validation are recorded in
  [Manager-owned deferred authentication](2026-09-24-manager-owned-deferred-authentication.md).
- Existing auth, store, secret, and cache regression tests moved with their owners.
- Focused deferred, demand, evaluation, inventory, explicit-identity, computed-value,
  and secret tests passed in `internal/exec`, `cmd/list`, `cmd`, and `pkg/list`.
- Repository `atmos test` passed with `TEST` scoped to the five changed packages;
  `go build ./...` and patch-scoped `atmos fix lint` passed.
- Website production build and fix-record validation passed.
- The final workspace binary returned all three `/tmp/my-landing-zone` stacks
  without auth/degradation warnings in strict default, JSON, and tree output,
  including `--identity=false`. Explicit `--identity=local` still failed with the
  emulator stopped. Demo configuration, the emulator, and installed Atmos were
  not changed.

## Follow-ups

None.
