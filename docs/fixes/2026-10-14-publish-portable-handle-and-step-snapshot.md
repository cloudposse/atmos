# Fix: Update publish test expectations

**Date:** 2026-10-14

## Summary

Update publish test expectations.

## Context

Windows closed-handle Stat errors differ from Unix. The workflow error snapshot also omitted the registered publish step.

## Changes

Check rejected file handles with a second Close and include publish in the registered-step snapshot. Production behavior and resource-closure requirements remain unchanged.

## Validation

Focused publish identity race tests and the real CLI invalid-step snapshot test passed. Windows execution will be checked in CI.

## Follow-ups

Verify the published commit in CI before considering the stack complete.
