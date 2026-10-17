# Fix: Model an existing stack in CI summary tests

**Date:** 2026-10-14

## Summary

Model an existing stack in CI summary tests.

## Context

The native CI changeset-name test modeled a missing stack but did not expect the extra CREATE-stub cleanup lookup.

## Changes

Use an existing UPDATE_COMPLETE stack for the changeset-name fixture, matching its intended scope and the corrected fixture already present in later layers.

## Validation

Focused race tests for TestOperationSummary_ChangesetNameReachesCI passed.

## Follow-ups

Verify the published commit in CI before considering the stack complete.
