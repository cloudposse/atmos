# Fix: Recognize landed stack members in release history

**Date:** 2026-10-24

## Summary

Release eligibility and release planning now recognize merged stack members whose original base remains another feature branch.

## Context

GitHub retains a native stack member's original parent branch in its merged PR metadata. After PR #3467 landed in the default branch, its commit appeared in the unreleased commit range but the release-history check rejected it because its recorded base was not `main`. This blocked release eligibility for unrelated PRs.

## Changes

- Collect the commit SHAs from the complete, validated published-baseline-to-source comparison.
- Accept a merged PR with a different original base only when its merge commit is in that verified source range.
- Keep rejecting unmerged PRs and branch merges that have not landed in the source range. Continue validating release labels and milestones for every included PR.
- Verify that release plans retain the notes for landed stack members.

## Validation

- Reproduced the history failure with HTTP-mocked landed-stack cases before the fix.
- Passed the complete milestone, quality-gate, and release package tests with `go test -p 2 -tags mage ./magefiles/release/milestone ./magefiles/qualitygate ./magefiles/release -count=1`.
- Regression cases cover a landed stack member, an unmerged stack member, an unrelated branch merge, missing release metadata, and release-plan entries.
- Passed patch-scoped custom `golangci-lint`, `go build -p 2 ./...`, and `git diff --check`.

## Follow-ups

The trusted-main milestone workflow must run after this fix lands to replace the failed release-policy results.
