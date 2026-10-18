# Fix: Describe the label parser test inputs accurately

**Date:** 2026-10-14

## Summary

Correct the function-level comment describing `ParseLabelsFlag` test inputs.

## Context

CodeRabbit identified that the comment claimed pflag preserved an embedded comma in a single slice element. The test already supplied separate pairs, matching pflag's StringSlice processing.

## Changes

Describe comma-separated pairs and repeated flag occurrences as separate slice elements. Parser behavior and test assertions are unchanged.

## Validation

- `GOTOOLCHAIN=go1.26.9 go test ./pkg/tags -count=1` passed.
- `GOTOOLCHAIN=go1.26.9 go build ./pkg/tags` passed.

## Follow-ups

None.
