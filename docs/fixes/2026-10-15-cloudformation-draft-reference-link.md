# Fix: Remove a published example link to draft documentation

**Date:** 2026-10-15

## Summary

Remove the CloudFormation example README's link to the draft command reference.

## Context

The examples browser publishes this README, while the command reference retains
`draft: true` and is excluded from production. Its absolute URL therefore led
readers to a missing page even though the documentation source exists.

## Changes

Remove the Learn More section until the reference is published. Keep the lifecycle
instructions and draft documentation unchanged.

## Validation

Confirmed that the link target is marked draft and the example is published by
the examples browser. All 16 focused example-browser, draft-documentation, and
stack-sidebar tests pass. Applicable pre-commit checks pass.

## Follow-ups

Restore the reference link when the command documentation is published as part
of the active CloudFormation stack's documentation release.
