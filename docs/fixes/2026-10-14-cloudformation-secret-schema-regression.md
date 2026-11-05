# Fix: Cover component-scoped CloudFormation secrets in schema validation

**Date:** 2026-10-14

## Summary

Protect CloudFormation component-level `secrets.vars` declarations with schema regression coverage.

## Context

The PRD's shared-property list omitted `secrets`, raising concern that strict component schemas would reject declarations. All three implementation schema copies already accept `secrets`, but the CloudFormation-specific tests did not exercise it.

## Changes

Add a regression that accepts a realistic component secret declaration and rejects a scalar `secrets` value in the embedded manifest, stack-config, and fixture schemas.

## Validation

- `go test -race -count=1 ./pkg/datafetcher -run TestSchemaCloudFormationComponentSecrets` passed.
- Ran `git diff --check` and the fix-document validator.

## Follow-ups

None.
