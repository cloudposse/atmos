# Fix: Initialize import metadata scope in the batch stack loader

**Date:** 2026-10-27

## Summary

Restore compilation of scoped stack loading after integration with conditional imports.

## Context

Conditional imports added a metadata-scope argument to the recursive stack loader.
The scoped-auth branch called that loader directly to propagate static-loading
restrictions, so its merged code was missing the new argument. Builds failed
across platforms, and dependent acceptance and Kubernetes jobs could not start.

## Changes

Initialize the batch loader's import metadata scope with the invocation's Atmos
configuration, matching the single-file entry points. This also preserves the
configured list-merge strategy when inherited tags control nested imports.
Add runtime and static regression cases for matching and skipped imports under
append and replace strategies.

## Validation

- Reproduced the missing-argument compiler error on GitHub's cumulative merge.
- The new runtime/static metadata-scope regression passes with the fix.
- Full build and stack-integration validation results are recorded with the PR.

## Follow-ups

None.
