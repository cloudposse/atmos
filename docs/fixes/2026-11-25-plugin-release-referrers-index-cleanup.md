# Fix: Re-signing a plugin release on GHCR fails when the old referrers index can't be deleted

**Date:** 2026-11-25

## Summary

Issue #3760. The `Official plugin packages / release` job (`Tests` workflow, push to `main`) failed on every push
from 2026-11-22. The last success was `094635ab1e` (#3600); the first failure was `2d7822f214` (#3726). Every run
failed while re-signing the unchanged `auth-secrets` 1.1.3 release:

```text
failed to delete dangling referrers index sha256:5de46fcd... for referrers tag sha256-6505f035...:
DELETE ".../manifests/sha256:5de46fcd...": response status code 405: unsupported: The operation is unsupported.
```

`atmos plugin release --inventory` stops at the first package error, and `auth-secrets` is first in
`plugins/releases.yaml`. Because of that, `terragrunt` 0.1.2 (bumped in #3754) was never published, and the
`cloudposse` catalog was never reconciled. Atmos now treats this specific error as success: the registry refused to
delete the replaced referrers index, but the attachment and the new index that lists it are already in place.

## Context

- `atmos plugin publish/release --sign` re-signs a release even when it is unchanged (`cmd/plugin/publish.go`,
  `publishLayout`). This is intentional: existing referrers may be untrusted or from another signer.
- GHCR doesn't serve the OCI 1.1 referrers API, so oras-go (`v2.6.2`) indexes attachments through the referrers
  **tag schema**. When a manifest with a `subject` is pushed (or such a manifest is deleted), oras-go:
  1. pushes the manifest;
  2. pushes a new referrers index and tags it `sha256-<subject digest>`;
  3. deletes the index it replaced (`registry/remote/repository.go`, "delete the dangling original
    referrers index").
- GHCR answers that manifest `DELETE` with `405`. oras-go returns a `*remote.ReferrersError` whose
  `IsReferrersIndexDelete()` is true, even though steps 1 and 2 already succeeded.
- The first signature of a release has no previous index to delete, so it succeeds. Every later re-sign of the
  same release fails, and re-running the job can't help.
- Atmos built the `remote.Repository` with the default `SkipReferrersGC: false` and wrapped every error from
  `Push`, `PushReference` and `Delete` as a failure.

## Changes

- `pkg/oci/transport_registry.go`: new `manifestWriteResult` used by `registryStore.Push`, `PushReference` and
  `Delete`. It returns success when the only failure is a `*remote.ReferrersError` with
  `IsReferrersIndexDelete()`, and logs it at debug level. The replaced index is left untagged, as it would be with
  `SkipReferrersGC`. Every other error is still wrapped with `wrapRegistry`, so a refused manifest push or a
  refused new referrers index still fails.
- The cleanup DELETE is still attempted, so registries that allow it (most of them) keep garbage-collecting
  replaced indexes. Setting `SkipReferrersGC` globally was rejected for that reason.
- No plugin package version bump: `pkg/oci` is not an input of either package in `plugins/releases.yaml`.

## Validation

- New `pkg/oci/referrers_index_cleanup_test.go`. It uses a fixture registry without the referrers API that answers
  manifest `DELETE` with 405, as GHCR does.
  - `TestPushAttachmentSucceedsWhenTheRegistryRefusesToDeleteTheReplacedReferrersIndex`: `Push` and
    `PushReference` of later attachments succeed, two index deletes were refused, and a fresh store lists all
    three attachments.
  - `TestPushAttachmentFailsWhenTheRegistryRefusesTheNewReferrersIndex` (negative path): when the new index tag is
    refused (403), the push still fails with `ErrOCIRegistry`, and no index delete is attempted.
  - `TestDeleteAttachmentSucceedsWhenTheRegistryRefusesToDeleteTheReplacedReferrersIndex`: deleting an attachment
    succeeds, and the index lists only the remaining one.
  - Before the fix, both success-path tests failed with the exact CI error (`failed to delete dangling referrers
    index ... 405`).
- New `pkg/plugin/sign/sign_resign_test.go`. `TestReSignSucceedsWhenTheRegistryRefusesToDeleteTheReplacedReferrersIndex`
  signs a published plugin index twice through `sign.Service` against the same GHCR-like registry. Both signatures
  succeed, the second re-sign hits a refused delete, and `signature.Fetch` returns both bundles. With the fix
  disabled, this test fails on the re-sign.
- `pkg/plugin/sign/sign_test.go`: the plugin index fixture was moved into a shared `fixtureIndex` helper; the
  existing test is unchanged otherwise.
- `go build ./...` and `go test ./pkg/oci/... ./pkg/plugin/... ./cmd/plugin/...` (36 packages) pass.
- `golangci-lint` could not run locally (permission denied in this environment); CI runs it.
- Not verified against GHCR itself. The next `main` push after merge should publish `terragrunt` 0.1.2 and
  reconcile the catalog; check the `Official plugin packages / release` job there.

## Follow-ups

- `atmos plugin release --inventory` still stops at the first failing package, so one bad package blocks the
  packages after it. This is pre-existing behavior and is noted in #3760; it is not changed here.
