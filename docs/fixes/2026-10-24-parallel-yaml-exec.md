# Fix: Allow bounded concurrent YAML exec evaluation

**Date:** 2026-10-24

## Summary

Allow ordinary `!exec` expressions in a document to overlap using `settings.functions.exec.max_concurrency`.

## Context

Issue #3512 reports that five independent nested Atmos commands execute serially during YAML evaluation.
The user requested configurable concurrency for existing expressions, with `1` retaining serial execution
and command side effects remaining the user's responsibility.

## Changes

- Add a positive concurrency limit that defaults to `1`.
- Preserve each expression's original result location and existing output decoding.
- Retain cancellation, error attribution, and skipped-function behavior without introducing grouping syntax.
- Document the configuration, unchanged stack syntax, and ownership of shell side effects.

## Validation

- `go test -race ./pkg/function/execbatch -count=1` passed; barriers verify overlap and the configured bound, deterministic result placement, cancellation, and policy enforcement.
- Configuration, edition, schema, and targeted existing YAML/Starlark evaluation tests passed.
- Changed-package lint passed with zero issues; the CLI build passed.
- Real CLI evaluation of three 300 ms commands returned identical values with limits `1` and `3`: approximately 1.1 seconds serially versus 0.42 seconds concurrently.
- Package coverage is 92.2%.
- Website production build and all 82 prebuild tests passed; generated configuration, function, and changelog pages were verified.

## Follow-ups

None.
