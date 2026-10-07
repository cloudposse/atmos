# Fix: Avoid summed allocation capacities in environment helpers

**Date:** 2026-10-06

## Summary

Remove three summed length expressions reported by CodeQL's
`go/allocation-size-overflow` check in environment merging.

## Context

The PR check reported six alerts across three expressions: one in
`MergeGlobalEnv` and two in `ApplyAmbientEnv`. Each expression added collection
lengths to preallocate a result, allowing integer overflow before allocation.

## Changes

Use the first collection's length as the initial capacity and let Go grow the
result during append or map insertion. Input copying, declared-environment
precedence, and literal-field preservation keep their existing behavior.

## Validation

Targeted tests passed for global environment merging, ambient environment
merging, input immutability, and script environment rendering. CodeQL will
recheck the changed expressions after pushing.

## Follow-ups

None.
