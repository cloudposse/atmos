# Fix: Avoid copying backend request arguments

**Date:** 2026-10-11

## Summary

Pass backend create/update request arguments by pointer after the target setting enlarged the request struct.

## Context

Rebasing the stack onto current main enables stricter lint checks for the changed code.

## Changes

Update the internal function signature and all production/test callers without changing request fields or AWS behavior.

## Validation

Go 1.26.9 full backend package race tests passed (3.037 seconds).

## Follow-ups

None.
