# Fix: Synchronize the lifecycle test runner counter

**Date:** 2026-10-27

## Summary

Make the grouped-output failure test's runner counter safe for concurrent calls.

## Context

The test verifies that unavailable temporary files prevent grouped execution,
then runs an ungrouped control with two workers. Those workers incremented the
same ordinary integer without synchronization. The race detector reported the
conflicting accesses in CI, and local repeated execution reproduced them.

## Changes

Use an atomic counter in the test runner and load its value for both assertions.
Keep the two-worker control and its expected three executions, preserving the
behavior being tested.

## Validation

The original test fails under the race detector with CI's shuffle seed. The
fixed test passes repeated race-enabled runs; the lifecycle package and scheduler
regressions also pass under the CI Go toolchain.

## Follow-ups

None.
