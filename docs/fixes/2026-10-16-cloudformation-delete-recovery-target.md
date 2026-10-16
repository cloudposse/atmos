# Fix: Show the resolved target in delete recovery hints

**Date:** 2026-10-16

## Summary

Show the resolved target in delete recovery hints.

## Context

DELETE_FAILED recovery instructions used component and stack placeholders even when the resolved stack specification knew both values.

## Changes

Pass the stack specification into the recovery helper and build its hint with commandTarget. Keep AWS lookups and error messages keyed to the physical stack name. Test the complete delete failure flow with distinct physical and Atmos names.

## Validation

Regression reproduced against the original code. Focused race tests, the full Go build, and all applicable pre-commit checks passed with Go 1.26.9.

## Follow-ups

None.
