# Fix: Document interactive CloudFormation source selection

**Date:** 2026-10-09

## Summary

The source-delete reference and source overview distinguish interactive selection from explicit
arguments for unattended use.

## Context

The source-delete handler accepts an omitted component or stack and attempts interactive selection,
but the reference marked both inputs unconditionally required. The overview also incorrectly marked
the source-list command's component and stack filters as required.

## Changes

- Show optional component and stack syntax for source deletion, with an interactive example.
- Explain selection prerequisites, `ATMOS_STACK`, missing-value errors, and explicit unattended use.
- Distinguish prompted values for pull, describe, and delete from optional list filters.
- Pair these docs with the owning layer's type-aware source-selector correction so CloudFormation
  prompts offer CloudFormation components rather than Terraform components.

## Validation

- The complete `pkg/provisioner/source/cmd` test suite passed on the owning head.
- Existing selection and CI/terminal-gating tests in `pkg/flags` passed.
- Fix-log validation and `git diff --check` passed.
- The combined type-aware selector validation and website build run on the owning stack branch
  when the companion implementation and documentation patches are combined.

## Follow-ups

None.
