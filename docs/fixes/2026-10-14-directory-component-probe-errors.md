# Fix: Preserve directory component probe errors

**Date:** 2026-10-14

## Summary

Report invalid component configuration during directory-stack type detection instead of silently selecting a different type with the same component name.

## Context

The directory presence probe converted every processing error into an absent component. A Terraform component with an invalid flag value or retry duration could therefore disappear from detection while a same-named Helmfile component was selected.

## Changes

Distinguish missing component/type sections by inspecting the merged manifest before processing. Propagate processing errors for existing definitions, preserving the original error chain. Add regressions using real temporary directory-stack manifests for invalid fields, valid duplicate definitions, and missing types or components.

## Validation

- Reproduced the invalid flag and retry cases returning no error before the fix.
- Focused race tests passed for the directory presence probe, directory-stack processing, flag decoding, and component-type resolution.
- Lint against the owner HEAD reported zero issues for this patch. The broader stack check found two inherited CloudFormation complexity findings already fixed by later stack layers.
- Whitespace and fix-document validation passed.

## Follow-ups

None.
