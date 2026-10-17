# Fix: Wait for preview stack deletion

**Date:** 2026-10-11

## Summary

CloudFormation previews wait for deletion of their newly created empty stack before
reporting successful cleanup.

## Context

A CREATE changeset leaves a `REVIEW_IN_PROGRESS` placeholder stack. Cleanup sent
`DeleteStack` but returned immediately, although the CloudFormation PRD requires
waiting until deletion finishes. Asynchronous deletion failures could therefore
be missed.

## Changes

- Poll the owned stack by its immutable ID after requesting deletion.
- Accept either a missing stack or `DELETE_COMPLETE`, since deleted stacks can
  remain describable by ID.
- Surface `DELETE_FAILED`, API errors, and timeout errors with the stack ID.
- Keep the existing 30-second cleanup deadline and cancellation isolation.
- Preserve the ownership guard: existing or subsequently deployed stacks are
  never deleted by preview cleanup.

## Validation

The new deletion regression failed before the fix and passed afterward. Focused
preview tests pass, including cancelled-preview cleanup. The full owning-package race suite passes with 98.1% statement coverage; all
cleanup functions have 100% statement coverage in the local profile. The full
Go build passes and affected custom lint reports zero issues. These local
coverage measurements cover package tests; CI Codecov remains authoritative
for final PR patch coverage.

At the prepared stack tip, the Floci lifecycle passes with this production
change: a new-stack preview leaves no active placeholder, initial deployment and
unchanged redeployment pass, the changed release passes, all three HTTP checks
return 200 with the expected response body, and cleanup removes the example
stacks, artifacts, and emulator. Emulator and Lambda logs were captured before
cleanup.

## Follow-ups

None.
