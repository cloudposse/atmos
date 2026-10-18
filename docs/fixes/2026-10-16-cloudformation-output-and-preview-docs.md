# Fix: Clarify CloudFormation output lookups and preview cleanup

**Date:** 2026-10-16

## Summary

Correct the changeset create and CloudFormation output function documentation.

## Changes

Explain that diff and plan discard their temporary changesets and preview-created stack placeholders that remain in REVIEW_IN_PROGRESS, returning cleanup failures. Named changesets remain available for review and execution. Remove the unsupported per-command output cache promise: each evaluation fetches Outputs through DescribeStacks.

## Validation

Compared the documentation with runDiff, discardChangeSet, removeStubStack, cloudFormationOutputsForSections, and the default output getter. Compiled both MDX pages and ran documentation navigation checks.

## Follow-ups

None.
