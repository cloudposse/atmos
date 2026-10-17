# Fix: Make lint and process cancellation validation reliable

**Date:** 2026-10-14

## Summary

Let Linux lint type-check the release build and make process cancellation tests wait for an actual child startup.

## Context

The Windows cancellation test expired its 20 ms context before process startup on a loaded runner, then incorrectly expected a wait error. Separately, the Linux lint job uploaded 17 type-check errors rooted in the optional native HID dependency while reporting success because SARIF collection intentionally uses an issue exit code of zero.

## Changes

The cancellation helper now emits a startup marker. Its output writer cancels the context only after the process starts; a separate deadline bounds the test. Keep assertions for startup, cancellation, a failed wait, and a nonzero exit.

Set `CGO_ENABLED=0` for the lint job, matching release builds and the existing vulnerability-check job. Preserve SARIF upload, security permissions, and the lint reporting policy.

## Validation

- The cancellation regression passed 20 repetitions with the race detector using Go 1.26.9.
- Workflow validation with `actionlint` passed.
- Patch-scoped lint with CGO disabled reported zero issues.

## Follow-ups

Verify the final published commits in CI. The separate Windows acceptance runtime crash still requires diagnosis; these changes do not claim to fix it.
