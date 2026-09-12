# Fix: `pkg/ci/artifact/s3` store now honors the identity's `EndpointURL` override

**Date:** 2026-08-25

## Summary

The S3-backed CI artifact store (`pkg/ci/artifact/s3`) never threaded the active identity's
`EndpointURL` override into its S3 client, so an identity scoped to an emulator (e.g. Floci) still
routed uploads/downloads to real AWS. Every other AWS client in the `aws/cloudformation` feature
(CFN's own client, and `pkg/provisioner/backend`'s S3 client) already honors this override
correctly — this store was the one holdout.

## Context

Found live, while verifying the CFN backend auto-provisioning fix
(`2026-08-25-cfn-backend-auto-provisioning.md`): `apply`'s template-upload step (this store, invoked
via `pkg/component/aws/cloudformation/packaging.go`) failed against a live Floci emulator with
`no EC2 IMDS role found`, even though the bucket already existed and the active identity was
correctly resolved everywhere else in the same command. Confirmed via a second run (bucket
pre-existing, auto-provisioning uninvolved) to reproduce identically, and via code reading that
`buildAuthConfigOpts` (store.go) read `CredentialsFile`/`ConfigFile`/`Profile`/`Region` from the
resolved auth context but never `EndpointURL`, and `initIdentityClient` called
`s3.NewFromConfig(cfg)` with zero `optFns` — no way to ever set `BaseEndpoint`.

## Changes

`pkg/ci/artifact/s3/store.go`: added `buildClientOptFns(authContext)`, mirroring
`pkg/component/aws/cloudformation/client.go`'s `newClient` pattern exactly — when
`authContext.EndpointURL` is non-empty, appends an `s3.Options` functional option setting
`BaseEndpoint`. Wired into `initIdentityClient`'s `s3.NewFromConfig(cfg, ...)` call. Left
`initDefaultClient` unchanged — it has no `authContext` in scope (no identity configured), so
there's nothing to override there.

`.golangci.yml`: added `pkg/ci/artifact/s3/**` to the `provider-agnostic-auth` depguard
exclusion list — this package is the S3-backed CI artifact store and legitimately depends on the
AWS SDK, same rationale already recorded for `pkg/ci/planfile/s3`. The package's existing
non-test AWS SDK imports predate this branch so were never flagged by `--new-from-rev`; this
change's new test-file imports were the first to surface the missing exclusion.

## Validation

- New test: `TestStore_BuildClientOptFns` (store_test.go) — written first, confirmed failing
  (`undefined: buildClientOptFns`) before the fix, then passing after. Mirrors the existing
  `TestStore_BuildAuthConfigOpts` pattern: applies the returned `optFns` to a fresh `s3.Options{}`
  and asserts `BaseEndpoint`, rather than introducing a new mock/interface (`Store.client` is a
  concrete `*s3.Client`, no injectable interface exists in this package).
- `go build ./... && go test ./pkg/ci/artifact/...` — all pass.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding remains:
  `cmd/terraform/utils.go:484`).

## Follow-ups

None.
