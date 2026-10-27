# Fix: isolate the init-hook test's selected component

**Date:** 2026-10-25

## Summary

Restore the selected component after the init-hook preparation subtest so later tests cannot inherit its selection.

## Context

The CI race-test shard failed `TestPrepareHookContextSkipsComponentResolutionForMultiComponent`: it expected no selected component but received `myapp`. `TestInitCmd_HookWiring` invokes `PreRunE`, which intentionally persists the selected component for subsequent hooks and execution. The test did not restore that package-level state. Shuffling tests exposed the leak when the multi-component test ran afterward.

## Changes

Capture and restore `preResolvedComponent` with subtest cleanup. Assert that preparation selects `myapp`, preserving coverage of the intended production behavior. Production code is unchanged.

## Validation

Before the fix, running the init and multi-component hook tests together reproduced the exact CI assertion failure locally. Afterward, the focused tests passed three times under the race detector with CI's shuffle seed (`1792974955209863846`). The full `cmd/terraform` package also passed under the race detector with the same seed (`go test -race ./cmd/terraform -shuffle=1792974955209863846 -count=1 -timeout=20m`). Patch-scoped lint of `./cmd/terraform/...` reported zero issues.

## Follow-ups

None.
