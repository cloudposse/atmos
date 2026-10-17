# Fix: Preserve and validate CloudFormation packaging selectors

**Date:** 2026-10-14

## Summary

Preserve the existing `packaging` target selector through typed configuration decoding and validate its type in the shared JSON schemas.

## Context

Runtime delivery supported selecting among S3 targets, but `ProvisionTarget` omitted the selector and silently dropped it on typed round trips. The manifest schemas accepted invalid selector types. An explicit invalid selector was also ignored when only one S3 target existed.

## Changes

Add the string field to `ProvisionTarget`, regenerate the configuration schema, and declare the field in the manifest and stack-config schemas. Synchronize the fixture's previously missing provision-target definition. Resolve explicit selectors before the implicit single-target default so missing or non-S3 names fail consistently.

## Validation

- Reproduced dropped selectors in JSON, YAML, and mapstructure decoding and invalid-type acceptance before the fix.
- Focused race tests passed for typed round trips, all three manifest schema copies, generated-schema freshness, and packaging target selection.
- `go test -race -count=1 ./pkg/datafetcher` passed in full.
- Patch-scoped lint and the same focused regressions at the combined stack tip passed.
- Tests cover both valid explicit selection and rejection of missing or non-S3 targets with a single S3 destination.

## Follow-ups

None.
