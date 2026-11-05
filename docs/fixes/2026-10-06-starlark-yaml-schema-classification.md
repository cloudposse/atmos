# Fix: Classify Starlark YAML values as stack-manifest-only

**Date:** 2026-10-06

## Summary

Record `!starlark` in the schema generator's YAML-function classification.

## Context

`TestEveryYamlFunctionIsClassified` failed in acceptance and race jobs because
the new `AtmosYamlFuncStarlark` constant was absent from the classification
table. The runtime supports this tag in stack manifests, which supply the
component context, and rejects it in `atmos.yaml`.

## Changes

Add the constant to the classification table and its stack-manifest-only set.
The generated `atmos.yaml` schema continues to exclude `!starlark`.

## Validation

- The complete `pkg/config/schema` test suite passed.
- `go generate ./pkg/config/schema` completed with no changes to the embedded
  schema artifact.

## Follow-ups

None.
