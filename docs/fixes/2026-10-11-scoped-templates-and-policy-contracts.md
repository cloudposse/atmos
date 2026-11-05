# Fix: Preserve template scope and declared configuration contracts

**Date:** 2026-10-11

## Summary

Preserve template scope and declared configuration contracts.

## Context

Review found scoped template actions deferred outside their execution context, raw YAML file output changing scalar types, unsupported target-auth schema keys, and policy-body support only on a later layer.

## Changes

Keep variable-dependent and rebound-dot actions in their original template execution. Encode YAML files with the YAML serializer. Reject unsupported require_identity target settings in both schemas. Bring existing inline stack-policy serialization, loading, and regressions into the layer that exposes the policy-body schema.

## Validation

New scope and scalar regressions failed before the fixes. Focused race tests for template, utils, target auth, schemas, and stack policies pass; changed-code lint reports zero issues. Full race suites pass for template (91.7% package coverage), utils (81.6%), and CloudFormation (97.8%). The scope-preservation helper is 100% covered and inline policy parsing is 94.1% covered; these local figures do not replace the final Codecov patch report.

## Follow-ups

None.
