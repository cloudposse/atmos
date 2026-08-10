# Fix: Route concurrent provisioning status through UI

**Date:** 2026-08-10

## Summary

Concurrent provisioning status now uses UI formatting and masking while writing to its component-specific stderr stream.

## Context

Direct writes to component writers bypassed the UI layer. The initial writer-aware package functions also made call sites less clear than a destination-bound output object.

## Changes

Added `ui.New(writer)` with semantic success, warning, and info methods. Updated provisioning call sites and tests to use one bound output object per operation.

## Validation

Passed `go build ./...`, focused provisioning and UI tests, and `atmos lint --changed`. `atmos test` has unrelated failures from an inherited `TF_PLUGIN_CACHE_DIR` under `/Users/...` and a CLI cleanup timeout. The website build is blocked by the existing `website` lockfile/dependency mismatch.

## Follow-ups

None.
