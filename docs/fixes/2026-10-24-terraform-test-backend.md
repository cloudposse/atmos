# Fix: Run Terraform tests without generating a backend

**Date:** 2026-10-24

## Summary

`atmos terraform test` no longer requires `backend_type` solely because automatic backend-file generation is enabled.

## Context

On main at `cb23b3f701`, a provider-free test module failed with `backend_type is required` before its tests ran.

## Changes

Skip backend-file generation for `test` and preserve existing backend files. Move generation and shared rendering into `pkg/terraform/backend`, replacing duplicated implementations and keeping thin callers in `internal/exec`. Document the test behavior; other commands keep their existing backend requirements.

The `init`, `plan`, and `apply` regression checks assert `ErrBackendTypeRequired` with `assert.ErrorIs`, so unrelated errors cannot satisfy the missing-backend contract.

## Validation

Regression tests failed before the change. Ran `go test ./pkg/terraform/backend ./pkg/terraform/output -count=1` and focused existing `internal/exec` backend tests. A provider-free CLI reproduction with auto generation enabled and no backend type passed one test. Changed-code lint, `go build ./...`, and the combined website build passed.

After tightening the error assertion, `go test ./pkg/terraform/backend -count=1`, `go build ./pkg/terraform/backend`, and changed-code custom lint passed.

## Follow-ups

None.
