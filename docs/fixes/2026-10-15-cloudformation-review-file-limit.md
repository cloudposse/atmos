# Fix: Keep CloudFormation stack layers within the review file limit

**Date:** 2026-10-15

## Summary

Move the 26 Phase 2 website documentation paths from PR #3000 to the existing next PR, #3001, keeping the stack at 17 pull requests.

## Context

CodeRabbit rejected #3000's full review because its 168 files exceeded the 150-file limit. Its previous incremental review checkpoint could not be recovered. The next layer had sufficient capacity to carry the documentation.

## Changes

- Defer Phase 2 command and output-function documentation in #3000, reducing its diff to 142 files.
- Retain those documents, including the later Phase 3 edits, in #3001 and incorporate the updated parent ancestry.
- The relocation commit preserves #3001's existing tree exactly apart from this fix record. Moving the guides changes neither code nor the combined website contents.
- Preserve the other 15 branch heads and the existing PR sequence.

## Validation

- Website prebuild, navigation tests, and production builds passed at both layers.
- Applicable pre-commit checks passed for both layers.
- Tree comparison confirmed the child retained all existing content before adding this record.

## Follow-ups

Verify published file counts, mergeability, CI, coverage, and CodeRabbit reviews on the two new heads.
