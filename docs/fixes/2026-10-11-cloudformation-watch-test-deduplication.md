# Fix: Align CloudFormation watch and preview test expectations

**Date:** 2026-10-11

## Summary

Watch tests share their ordered progress-to-terminal polling expectations, satisfying the stricter duplicate-code lint check inherited from main.

## Context

The watch success, watch failure, and operation-dispatch tests repeated the same mock setup. Their status values and assertions differ, but their expected API ordering is identical.

## Changes

- Extract the ordered event/status polls into `expectWatchStatusTransition`.
- Keep each test's success or rollback statuses and assertions unchanged.
- Require diff dispatch to delete its preview changeset and then inspect the owned stub.
- Expect packaging-test cleanup only for diff; explicit changeset creation keeps its review artifact, so unexpected cleanup remains a test failure.

## Validation

- Go 1.26.9 package build and patch-scoped custom GolangCI lint passed with zero patch findings.
- An overlay using the original owning-layer test files reproduced the changeset cleanup mock failures before their correction.
- After correction, the full owning-layer CloudFormation race suite passed with Go 1.26.9. The later field-test layer and stack top also passed their full CloudFormation race suites.

## Follow-ups

None.
