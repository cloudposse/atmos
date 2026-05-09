# Fix: `--all` flag crashes with NoSuchBucket when backend provisioning is enabled

**Date:** 2026-03-26

## Problem

When running `atmos terraform plan --all -s <stack>` with `provision.backend.enabled: true`,
the command crashes with a `NoSuchBucket` error before any components are executed. The same
components work correctly when executed individually (e.g., `atmos terraform plan vpc -s <stack>`).

### Error

```text
Error: failed to read Terraform state for component `vpc` in stack `dev-ue1`
  in YAML function: `!terraform.state vpc dev-ue1 .bucket_name`
    Failed to get object from S3: operation error S3: GetObject,
    https response error StatusCode: 404, api error NoSuchBucket:
    The specified bucket does not exist
```

### Reproduction

1. Configure a stack with `provision.backend.enabled: true` (S3 backend auto-provisioning)
2. Have at least one component that references another component's state via `!terraform.state`
3. Run `atmos terraform plan --all -s <stack>` before any backends have been provisioned

Single-component mode works because provisioners create the S3 bucket during `terraform init`
before any state reads happen. The `--all` path calls `ExecuteDescribeStacks()` first to
enumerate all components, which eagerly evaluates YAML functions like `!terraform.state` for
ALL components — before any provisioners have run.

## Root Cause

The error chain:

```text
S3 GetObject returns NoSuchBucket
  → ReadTerraformBackendS3Internal retries 3 times (NoSuchBucket was not an early-exit case)
  → Returns ErrGetObjectFromS3
  → GetTerraformBackend wraps as ErrReadTerraformState
  → GetTerraformState wraps as "failed to read..."
  → processTagTerraformState checks isRecoverableTerraformError()
  → ErrGetObjectFromS3 is NOT recoverable → error propagates
  → ExecuteDescribeStacks fails → entire --all operation crashes
```

The S3 backend reader (`ReadTerraformBackendS3Internal`) already handled `NoSuchKey` (missing
state file) gracefully — returning `nil, nil` to indicate "not provisioned." But `NoSuchBucket`
(missing bucket) was not handled, causing it to fall through to the retry loop and eventually
return a non-recoverable error.

The existing recovery mechanism works correctly once the error is classified properly:

```text
nil content from S3 reader
  → GetTerraformBackend returns nil backend
  → GetTerraformState returns ErrTerraformStateNotProvisioned
  → isRecoverableTerraformError() returns true
  → If YQ default exists (e.g., `.output // "fallback"`): uses fallback value
  → If no YQ default: returns recoverable error, --all can handle gracefully
```

## Fix

Added `NoSuchBucket` handling in `ReadTerraformBackendS3Internal` alongside the existing
`NoSuchKey` check. If the bucket doesn't exist, the component's state cannot exist either —
return `nil, nil` (same as `NoSuchKey`).

Handles both error forms:
- `*types.NoSuchBucket` — typed error from AWS SDK v2
- `smithy.APIError` with code `"NoSuchBucket"` — generic error from S3-compatible backends (MinIO, Wasabi, etc.)

This also avoids unnecessary retries — `NoSuchBucket` returns immediately instead of retrying
3 times with exponential backoff.

## Files Changed

- `internal/terraform_backend/terraform_backend_s3.go` — Add `NoSuchBucket` early-exit + `isNoSuchBucketError()` helper
- `internal/terraform_backend/terraform_backend_s3_test.go` — Update test: `NoSuchBucket` expects `nil, nil` (not error); add typed error variant

## What This Does NOT Change

- The `--all` execution path (`terraform_query.go`, `terraform_all.go`) — untouched
- Auth propagation from #2140 — still works, still needed for YAML functions during enumeration
- Auth threading from #2250 — unrelated codepath (`--affected`)
- Single-component behavior — already worked correctly

## Related

- `docs/fixes/2026-03-03-yaml-functions-auth-multi-component.md` — #2140: auth propagation for YAML functions in `--all` path
- `docs/fixes/2026-03-25-describe-affected-auth-identity-not-used.md` — #2250: auth threading for `--affected` path
