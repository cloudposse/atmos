# Fix: Route concurrent provisioning status through UI

**Date:** 2026-08-10

## Summary

Concurrent provisioning status now uses UI formatting and masking while writing to its component-specific stderr stream.

## Context

Direct writes to component writers bypassed the UI layer. The initial writer-aware package functions also made call sites less clear than a destination-bound output object.

## Changes

Added `ui.New(writer)` with semantic success, warning, and info methods. Updated provisioning call sites and tests to use one bound output object per operation. Normalized formatted-output assertions and Windows `cmd /C` test-binary quoting.

## Validation

Passed `go build ./...`, focused provisioning and UI tests, and `atmos lint --changed`. Component-output tests strip ANSI sequences before semantic assertions so they are stable on color-capable macOS runners.

## Follow-ups

None.
