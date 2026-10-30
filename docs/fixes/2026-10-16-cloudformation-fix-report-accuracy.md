# Fix: Align CloudFormation fix reports with the companion implementation

**Date:** 2026-10-16

## Summary

Correct three stale statements in the phase 4 documentation reports and clarify when backend provisioning becomes available.

## Context

CodeRabbit identified an invalid-format claim that incorrectly included apply, an obsolete nonfatal cleanup description, and a labels follow-up already implemented in the companion PR.

## Changes

Limit the invalid-format claim to output. Describe returned cleanup errors and reference the current regression test name. Mark the CloudFormation labels work implemented in #3137 and pending merge. Mark both automatic and explicit backend provisioning as requiring phase 4; earlier phases require creating the bucket outside Atmos. Repeat that prerequisite in the migration guide’s artifact-bucket section and FAQ so readers can follow either section independently. Document the existing-bucket confirmation and `--auto-approve` flag on both backend create and update pages.

## Validation

Compared the reports against the published #3137 runDiff, joinCleanupError, output format registration, and labels parsing code. The fix-record validator and git diff whitespace check passed.

## Follow-ups

None.
