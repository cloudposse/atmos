# Fix: `provision.backend.enabled: true` now actually auto-provisions for CloudFormation

**Date:** 2026-08-25

## Summary

`website/docs/migration/from-rain.mdx` documented `provision.backend.enabled: true` as an
alternative to running `atmos aws cloudformation backend create` manually — the S3 artifact bucket
would be auto-provisioned on `apply`/`deploy`. This was false: the only existing auto-provision
mechanism (`pkg/provisioner/backend_hook.go`'s `autoProvisionBackend`) is registered exclusively for
Terraform's `before.terraform.init` hook event and was never invoked anywhere in CloudFormation's
apply/deploy code path. A user following the documented migration path would deploy, hit a
missing-bucket failure, and have to run `backend create` manually anyway.

## Context

Found and confirmed during a field-test pass this session (see the CFN field-test report and
`2026-08-25-cfn-small-fixes-and-docs.md`). The user chose to implement the real behavior rather than
just correct the doc, since the promised UX (auto-provision on first apply) is genuinely valuable and
already fully specified by the doc's own example.

## Changes

`pkg/component/aws/cloudformation/backend.go`:
- `isBackendProvisionEnabled(componentConfig)` — reads `provision.backend.enabled` (top-level,
  sibling to `targets`) from the raw component config. Duplicated (not exported/imported) from
  Terraform's own private equivalent in `pkg/provisioner/backend_hook.go`, since it's a two-key
  raw-map read not worth a cross-package export.
- `autoProvisionBackendIfEnabled(ctx, autoProvisionArgs)` — the new auto-provision entry point.
  Existence-check-then-create (via `pkg/provisioner/backend.S3BackendExists` +
  `pkg/provisioner.ProvisionWithParams`), mirroring Terraform's own hook rather than calling
  `ProvisionWithParams` unconditionally on every apply — `ProvisionWithParams` always reconciles
  (overwrites versioning/encryption/public-access/tags on an existing bucket, per
  `2026-08-25-cfn-confirmation-gates.md`), so calling it on every apply would cost an AWS round-trip
  and spinner even once the bucket is already provisioned, and would silently re-apply defaults to a
  bucket a user may have customized.
- Failure handling, matching Terraform hook's own convention: `enabled` absent → silent no-op
  (zero behavior change for every existing component). Existence-check failure → logged via
  `log.Debug` and deferred to `uploadPackage`'s own error one step later (the real answer will
  surface momentarily regardless). Creation failure → hard error via
  `errUtils.Build(ErrInvalidAwsCloudFormationSettings)`, naming the bucket and hinting at
  `backend create`/disabling the flag — apply must not silently continue into `uploadPackage` against
  a bucket that may not exist.

`pkg/component/aws/cloudformation/provision.go`'s `deliverApply`: wired the call between
`resolvePackagingTarget` (target resolution) and `uploadPackage`/`needsPackaging` (the first S3
write) — the only point downstream of resolution and upstream of every S3 write in this function.
Reuses the *same* already-resolved `s3Target`, never independently re-resolving via
`ResolveS3BackendTarget` (a different resolution algorithm used only by the manual `backend`
commands) — auto-provisioning can never target a different bucket than the one `apply` is about to
upload to.

`errors.go`: no new sentinel needed — reuses the existing `ErrInvalidAwsCloudFormationSettings`.

## Validation

- New tests: `backend_autoprovision_test.go` — `TestIsBackendProvisionEnabled` (7 cases),
  `TestAutoProvisionBackendIfEnabled_Disabled_NoOp/AlreadyExists_NoOp/Missing_Creates/
  CreateFails_ReturnsError/ExistenceCheckFails_DefersSilently`, using a `createTrackingS3Client`
  (wraps this package's existing `fakeS3Client` test double, adding a `createBucketCalled` flag) via
  `pkg/provisioner/backend`'s existing `SetS3ClientFactory` test seam — no real AWS calls.
- `provision_test.go`: `TestDeliverApply_AutoProvisionsMissingBackend` and
  `TestDeliverApply_AutoProvisionFailure_AbortsBeforeUpload` — end-to-end through the real
  `deliverApply` call chain, proving the wiring order (auto-provision before upload) and abort
  behavior on failure, not just the unit in isolation.
- `go test ./pkg/component/aws/cloudformation/... ./pkg/provisioner/...` — all pass.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding remains).
- Live, against a real local Floci AWS emulator: `atmos aws cfn deploy demo -s local --target
  artifacts` with `provision.backend.enabled: true` and no prior `backend create` printed
  `✓ Provisioned S3 backend atmos-cfn-demo-artifacts-local for demo in stack local` and the bucket
  was confirmed created (`backend describe` before/after).

## Follow-ups

Live verification surfaced a **separate, pre-existing bug**, unrelated to this fix and confirmed to
reproduce identically with auto-provisioning uninvolved (bucket already existing): `apply`'s template
upload (`packaging.go`'s `uploadPackage` → `pkg/ci/artifact`'s S3 store) fails against Floci with
`no EC2 IMDS role found` — its S3 client isn't routed through the active identity's Floci-scoped
credentials/endpoint the way `pkg/provisioner/backend`'s S3 client (used by both `backend create` and
this fix's auto-provision path) correctly is. Not fixed here — out of scope for this change, and per
the user's standing preference this isn't filed as a GitHub issue without explicit authorization.
Worth a dedicated fix pass: compare `packaging.go`'s S3 client construction against
`pkg/provisioner/backend/s3.go`'s (which does honor the identity's endpoint override, per
`s3_endpoint_e2e_test.go`'s regression test) to find where the wiring diverges.
