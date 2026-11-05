# Fix: Preserve secret selector processing context

**Date:** 2026-10-16

## Summary

Preserve secret selector processing context.

## Context

Lazy secret backend selectors lost the calling skip list and explicit cycle-detection context.

## Changes

Forward skip functions and the caller resolution context through secret selector evaluation. Retain the goroutine context fallback for callers without an explicit context. Add regressions through !secret for skipped environment and CloudFormation selectors, explicit cycles, and fallback cycles. Align render help with the lower PR correction so cumulative merging preserves both changes without conflict.

## Validation

Regression reproduced against the original code. Focused race tests, CloudFormation command tests, the full Go build, and applicable pre-commit checks passed with Go 1.26.9.

## Follow-ups

None.
