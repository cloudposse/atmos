# Fix: Align secret selector regression tests with YAML editions

**Date:** 2026-10-26

## Summary

Secret selector regression tests now distinguish native YAML functions from quoted function-like strings under current and legacy editions.

## Context

The CI `TestSecretDiscoveryDefersExecutableSelectors/native=false` case expected a quoted `!exec` selector to execute on explicit access. Since edition `2026-10-20`, automatic function dispatch requires native YAML tags. Unpinned projects use this current behavior; quoted strings remain literal. Reproducing the failure and observing all six combinations of unpinned, current, and legacy editions with native and quoted selectors confirmed that production behavior already follows this contract.

## Changes

Replace the two-case selector test with an explicit edition matrix. Assert that native selectors execute only on an explicit read, quoted selectors execute only with legacy edition `2026-10-19`, and current quoted selectors remain literal even during an explicit read. Retain list, enumeration, and service-loading side-effect checks. Add an unused native selector and verify that accessing another declaration never executes it. Assert the selector-result error instead of accepting any error. No production behavior changes.

## Validation

The unchanged test reproduced the CI failure locally. An observational matrix then confirmed the actual marker-file effects and selector-result errors before expectations were updated. The final regression matrix, executable dependency cases, and Starlark selector case passed with race detection, randomized order, and three repetitions:

```shell
go test -race -shuffle=on ./cmd/secret -run '^TestSecretDiscoveryDefers' -count=3 -v
```

The complete secret command package also passed with race detection and randomized order:

```shell
go test -race -shuffle=on ./cmd/secret -count=1
```

## Follow-ups

Propagate this test correction from its owning PR through dependent stack layers. The marker fixtures use `touch` and retain their existing Windows skip; they do not require cloud credentials or secret backends.
