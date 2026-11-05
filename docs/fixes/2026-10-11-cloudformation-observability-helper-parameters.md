# Fix: Simplify CloudFormation observability helper parameters

**Date:** 2026-10-11

## Summary

CloudFormation test helpers and the stack-tree entry point satisfy main's stricter unused-parameter checks without changing behavior.

## Context

The root tree builder always started at depth zero after recursion moved into a separate helper. The S3 test-target helper's optional map always received nil. Watch tests also need the terminal-event read introduced in this layer to remain ordered after their shared polling expectations.

## Changes

- Start the root tree at zero inside `buildStackTree`; retain the recursive helper's depth increment and maximum-depth guard.
- Remove the unused S3 test-target overrides parameter and update its callers.
- Extend the shared watch expectation helper with the final event read after the terminal status.

## Validation

- Go 1.26.9 full CloudFormation package race tests and package build passed.
- Patch-scoped custom GolangCI lint passed with zero issues.
- At the stack top, all five assigned duplicate-code and unused-parameter findings disappeared; only the separately assigned complexity findings remained against main.

## Follow-ups

None.
