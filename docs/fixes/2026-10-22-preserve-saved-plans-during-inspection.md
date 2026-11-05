# Fix: Preserve saved Terraform plans during inspection

**Date:** 2026-10-22

## Summary

Read-only Terraform operations preserve saved binary plans. Plan comparisons copy external inputs to unique temporary files without overwriting existing component files.

## Context

Field testing reproduced two data-loss paths on the unchanged baseline. After `atmos terraform plan demo -s dev`, comparing the default plan to itself with `plan-diff --orig=<plan> --new=<plan>` removed it and failed. Generating JSON with `terraform generate planfile` also removed the prior binary plan. Comparing external inputs could additionally overwrite and delete an existing same-basename component file.

## Changes

- Restrict default-plan cleanup to successful apply/destroy commands that consume it.
- Preserve plans during inspection, formatting, validation and generation commands.
- Use unique temporary files for external plan comparisons; clean up only files created by that comparison.
- Use path-relative containment instead of string-prefix directory matching.

## Validation

Baseline CLI reproductions recorded plan SHA256 and existence before and after each operation, repeated twice. Fresh plan followed immediately by `apply --from-plan` succeeded as a control. A same-basename regression test failed before the copy fix. Focused cleanup/plan-diff tests verify saved bytes remain unchanged, external source files survive, and temporary copies are removed. Existing apply/destroy cleanup tests remain covered.

## Follow-ups

None.
