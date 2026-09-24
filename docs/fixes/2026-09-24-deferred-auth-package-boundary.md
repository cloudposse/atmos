# Fix: Keep deferred authentication under `pkg/auth`

**Date:** 2026-09-24

## Summary

Move demand-driven authentication to `pkg/auth/deferred`, while keeping value
selection, evaluation, recovery, and evaluated-value caching in `pkg/deferred`.

## Context

The initial demand-driven implementation put authentication resolution and its
success/failure cache next to evaluation policy. That obscured ownership: deferred
authentication is ordinary Atmos authentication requested later, not a separate
authentication system for list/describe commands.

## Changes

- Moved the resolver, factory, generated mock, authentication tests, and store/secret
  authentication preparation into `pkg/auth/deferred`.
- Updated list/describe callers and evaluator adapters to use the auth package.
- Separated evaluated values from cached authentication results. Evaluation owns a
  runtime-only context, and replacing an invocation's resolver invalidates its value
  cache. Nested configuration copies retain their initialized evaluation context.
- Made the disabled state part of the resolver interface instead of asserting a
  concrete implementation type.
- Kept provider error classification in provider implementations and retained
  existing explicit-identity, disabled-auth, and graceful-degradation behavior.

## Validation

- Package tests and race checks passed for `pkg/auth/deferred`, `pkg/deferred`, and
  `pkg/store/authbridge`.
- Local package coverage: 98.1%, 99.6%, and 100%, respectively.
- Added a regression for sharing evaluated values within an invocation and
  isolating values when a nested configuration starts a new invocation.
- Focused deferred-authentication, demand, evaluation, computed-value, inventory,
  and explicit-identity tests passed in `internal/exec`, `cmd/list`, `cmd`, and
  `pkg/list`; `go build ./...` and patch-scoped `atmos fix lint` passed.
- The website production build and fix-record validation passed.
- Built `.context/atmos` and verified `/tmp/my-landing-zone` with the emulator
  stopped: strict default, JSON, tree, and disabled-auth listings returned all
  three stacks without degradation warnings. Explicit `--identity=local` failed
  as expected. No demo configuration, emulator, or installed binary was changed.

## Follow-ups

None.
