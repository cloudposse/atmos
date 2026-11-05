# Fix: AWS ambient identities honor configured environment overrides

**Date:** 2026-10-11

## Summary

Fix [#3361](https://github.com/cloudposse/atmos/issues/3361): `aws/ambient`
identities now export their configured environment variables and apply them to
authenticated subprocesses, including Terraform.

## Context

The identity's `Environment()` returned an empty map, and `PrepareEnvironment()`
copied the inherited environment and applied `principal.region` without reading
the identity's `env` entries. Configuration was accepted but silently ignored.

Terraform also copied only a fixed set of auth-managed variables back into its
component environment. A custom identity variable such as `FOO` could therefore
be overwritten when the component environment was appended later.

Auth identity `env` already uses `[]EnvironmentVariable` with `key`/`value`
entries across identity kinds. This fix retains that interface. It differs from
global and stack `env` maps: the config loader has Viper case-preservation
machinery, and custom commands accept map syntax through
`CommandEnvDecodeHook()` while retaining an internal list. Auth identity env has
not adopted that map decoder or the corresponding case-restoration path. The
list is an existing auth convention, not a new requirement introduced by this fix.

## Changes

- Populate a fresh map from identity `env` in `Environment()`.
- Apply identity `env` last in `PrepareEnvironment()`, after inherited variables
  and principal-region settings. Preserve empty values and let the last duplicate
  key win, matching existing AWS identity behavior.
- Preserve explicitly configured AWS ambient keys in the Terraform auth hook's
  component environment, using the prepared values.
- Enforce subprocess precedence: identity `env` > global Atmos `env` > inherited
  shell environment. Retain unrelated inherited credentials and endpoint variables.
- Document precedence and a LocalStack example in the stack auth documentation.

AWS SDK authentication, generic `ambient` identities, and other identity kinds
retain their existing behavior. No public API or schema changes are included.

## Validation

- Confirmed the new identity and manager regression tests failed before the fix.
  Separately reproduced the Terraform custom-variable overwrite before updating
  the hook. All targeted regression tests pass after the changes.
- `go test -short ./pkg/auth ./pkg/auth/identities/aws -timeout 5m` passed.
- `go build ./...` and `atmos lint --changed` passed; lint reported zero issues.
- `npm run build` in `website/` passed with warnings in unchanged documentation
  and components.
- `atmos test` did not pass: `pkg/ci/startup` failed its startup-banner assertion,
  and the top-level CLI test package reached its five-minute timeout. The startup
  package passed directly; setting `ATMOS_STARTUP_NOTICES_SHOWN=1` reproduced its
  failure independently of this fix.
- `git diff --check` passed.

## Follow-ups

None.
