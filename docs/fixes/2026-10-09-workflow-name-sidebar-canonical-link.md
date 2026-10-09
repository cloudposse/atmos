# Fix: Preserve the canonical workflow naming guide in the sidebar

**Date:** 2026-10-09

## Summary

Keep the workflow `<name>` sidebar entry linked to the existing `/workflows/name` guide.

## Context

The CloudFormation generation documentation added a second `link` property to the same sidebar category. JavaScript silently used that later property, overriding the canonical naming guide. The navigation suite contained contradictory expectations for the same entry and failed on the final documentation layer.

## Changes

Remove the duplicate property and the newly added conflicting duplicate test. Preserve the existing regression that checks the canonical guide and its expandable step fields. Both documentation routes remain available.

## Validation

The complete sidebar test suite reproduced the conflicting-link failure before this fix and passed after it. `npm run test:navigation` passed all 57 tests. `npm run build` passed, including all 10 frontend prebuild tests at this layer. The fix-record validator and `git diff --check` passed.

## Follow-ups

None.
