# Fix: Make CloudFormation test expectations portable

**Date:** 2026-10-14

## Summary

Make CloudFormation test expectations portable.

## Context

Windows returns an invalid-handle error from Stat on a closed file. Colored warning output can insert ANSI sequences within error text. Two added component tests omitted the required context argument.

## Changes

Check closed capture files with a second Close, which preserves the ErrClosed assertion across platforms. Strip ANSI formatting before asserting warning text. Pass each test context into componentFunc.

## Validation

Focused race tests passed for cleanup warnings with forced color, stderr capture after panic and FailNow, large captured output, and CloudFormation component output lookups. Windows execution will be checked in CI.

## Follow-ups

Verify the published commit in CI before considering the stack complete.
