# Fix: Guide reviews toward extracting legacy package responsibilities

**Date:** 2026-10-24

## Summary

CodeRabbit review instructions now support incrementally shrinking `internal/exec` and `pkg/utils`.

## Context

The user requested a general review policy that moves touched behavior into focused owning packages as work proceeds, including existing code and its tests.

## Changes

Add path instructions for both legacy packages. Request extraction of touched domain behavior and tests, thin compatibility or orchestration adapters where necessary, and dependency injection to avoid import cycles. Keep refactors scoped to the current change.

## Validation

Parsed `.coderabbit.yaml` and checked the `reviews.path_instructions` structure against the published CodeRabbit schema. This is advisory review configuration; no runtime behavior or tests were added.

## Follow-ups

None.
