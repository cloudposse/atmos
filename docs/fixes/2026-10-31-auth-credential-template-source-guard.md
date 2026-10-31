# Fix: Preserve ordinary templates while protecting every credential path

**Date:** 2026-10-31

## Summary

Ordinary inline conditionals render in manifests containing auth configuration. Every occurrence of an identity's `spec.env` is inspected before general template rendering, including duplicate keys and alternate conditional branches.

## Context

The credential source guard decoded raw YAML before rendering. An ordinary value such as `stage: {{ if .enabled }}prod{{ else }}dev{{ end }}` was not valid raw YAML, so the presence of auth-shaped keys caused rejection even when the credential values were literal. Path lookup also stopped at the first matching key, leaving later duplicate credential expressions exposed to general helpers.

The [PR benchmark report](https://github.com/cloudposse/atmos/actions/runs/40971188406) identified overhead in the existing `TemplateProcessingComplex` workload. Against main `3d2d6b5a6191`, the pre-fix PR measured 286.96 microseconds versus 210.55 microseconds, 212,980 versus 173,990 bytes, and 822 versus 554 allocations per operation. These are ten-sample CI measurements on Linux ARM64 with Go 1.26.10; they are not directly comparable to measurements from another machine or toolchain.

## Changes

- Project YAML-breaking control syntax only for locating scalars; preserve original source when restoring protected values and retain template markers for alias checks.
- Visit every matching auth path, preserving valid conditional branches while shielding duplicate credential expressions.
- Match a scalar's own decoded template content before using its raw span, preventing an earlier flow scalar from consuming a later credential expression.
- Reject unquoted control prefixes, including control-only values decoded as implicit null, before helpers can execute.
- Skip YAML decoding only when an ASCII credential-path key is absent and the source contains no backslash. Escaped keys continue through full decoding.

## Validation

Regression tests reproduce the ordinary-inline and duplicate-path failures. They exercise both conditional branches, exact source restoration, quoted and block aliases, escaped keys, and all three general renderer entry points. HTTP spies assert zero requests for invalid credential forms; positive controls verify that ordinary helpers remain callable.

The unchanged curated `TemplateProcessingComplex` benchmark measures the ordinary-template path. Rebuild main `3d2d6b5a6191` and pre-fix PR `ddd373f36c20` with the same local toolchain and benchmark flags, collect the fixed candidate with `go tool mage bench:run`, and compare ten samples of that exact workload with benchstat. Local comparisons are rebuilt evidence, not historical CI snapshots. Upstream CI remains authoritative for the pushed revision.

## Follow-ups

None.
