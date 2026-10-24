# Fix: Clarify CloudFormation recovery targets and missing parameter values

**Date:** 2026-10-16

## Summary

Show the resolved target in delete recovery hints and identify missing `ParameterValue` fields directly.

## Context

DELETE_FAILED recovery instructions used component and stack placeholders even when the resolved stack specification knew both values.

Parameter-list entries without `ParameterValue` used the null-value diagnostic intended for unresolved output dependencies, hiding the missing field behind unrelated recovery advice.

## Changes

Pass the stack specification into the recovery helper and build its hint with commandTarget. Keep AWS lookups and error messages keyed to the physical stack name. Test the complete delete failure flow with distinct physical and Atmos names.

Reject an absent `ParameterValue` with the parameter name and missing field before converting its value. Preserve explicit null-value handling, `UsePreviousValue`, and mutual exclusion. Extend the existing invalid-parameter table with missing-value cases for absent and false `UsePreviousValue`.

## Validation

Regression reproduced against the original code. Focused race tests, the full Go build, and all applicable pre-commit checks passed with Go 1.26.9.

Both new missing-value cases failed against the published code and passed after the diagnostic fix. Focused parameter race tests and `go build ./...` passed with Go 1.26.9.

## Follow-ups

None.
