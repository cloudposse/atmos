# Fix: Guard CI-mode reads against concurrent theme configuration

**Date:** 2026-10-28

## Summary

CI-mode detection now reads global Viper configuration through the shared Viper guard.

## Context

The race-enabled CI suite found `ModeEnabled` reading Viper's environment-binding map while theme rendering updated that map through `viperguard.BindEnv`. Locking only the writer did not protect the unguarded reader. This race also exists in the inspected `origin/main` revision, `952cef312368b1bf0ae9d84a6fb554dca94174dd`.

## Changes

- Replace the direct `viper.GetBool` call with `viperguard.GetBool` so reader and writer share the same lock.
- Add a concurrent regression that reads CI mode while another goroutine binds the theme environment variables. Check that both the theme binding and CI configuration still take effect.
- Retain existing explicit-flag, worker-mode, provider-detection, and reporting-gate behavior.

## Validation

- Reproduced the race in two separate focused runs before the fix with Go 1.26.10.
- Passed the concurrent regression, CI-mode semantic controls, and the two runner tests from the CI race trace under `-race -shuffle=on -parallel=4 -count=10`.
- Passed `go build ./pkg/ci ./pkg/runner/step`, fix-record validation, and `git diff --check`. Go files were formatted with `gofumpt`.

## Follow-ups

None.
