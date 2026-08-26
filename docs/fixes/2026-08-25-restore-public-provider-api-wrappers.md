# Fix: Restore Public API Wrappers Used by `terraform-provider-utils`

**Date:** 2026-08-25

## Summary

Restored two public functions that external consumers of the Atmos Go library depend on:
`pkg/aws.ExecuteAwsEksUpdateKubeconfig` and `pkg/utils.JSONToMapOfInterfaces`. Both are now
covered by tests so they are not re-flagged as dead code.

## Context

`cloudposse/terraform-provider-utils` embeds the Atmos Go library to back its data sources. Its
`utils_aws_eks_update_kubeconfig` data source calls `pkg/aws.ExecuteAwsEksUpdateKubeconfig`, and
its test suite calls `pkg/utils.JSONToMapOfInterfaces`.

PR #2608 (`refactor(utils): drop dead helpers and hand-rolled SliceContainsString`) removed both
functions because the `deadcode` sweep reported zero callers *inside* the Atmos repository. The
reachability analysis does not see external module consumers, so these were public-API removals,
not truly dead code. As a result the provider fails to build against Atmos v1.222.0 and later:

- `pkg/aws.ExecuteAwsEksUpdateKubeconfig` moved into `internal/exec`, which external modules cannot
  import.
- `pkg/utils.JSONToMapOfInterfaces` was deleted outright.

This pins downstream consumers to Atmos v1.221.1 (the last release exposing the public API) and
prevents the provider from tracking newer Atmos releases, which is important because the provider
must embed the same Atmos deep-merge semantics as the Atmos CLI it is paired with.

## Changes

- `pkg/aws/aws_eks_update_kubeconfig.go` — re-added as a thin public wrapper delegating to
  `internal/exec.ExecuteAwsEksUpdateKubeconfig`, so external consumers can build an EKS kubeconfig
  from an Atmos component/stack context without importing Atmos internal packages.
- `pkg/utils/json_utils.go` — re-added `JSONToMapOfInterfaces`, which always decodes a JSON string
  into a `schema.AtmosSectionMapType` (and therefore errors on non-object top-level values, unlike
  `ConvertFromJSON`).
- `pkg/aws/aws_eks_update_kubeconfig_test.go` — added
  `TestExecuteAwsEksUpdateKubeconfig_ProfileAndRoleArnConflict`, which exercises the wrapper via its
  deterministic `profile`/`role-arn` conflict validation (no AWS credentials or network required).
- `pkg/utils/json_utils_test.go` — added `TestJSONToMapOfInterfaces` covering simple, nested, and
  empty objects plus the malformed-JSON, non-object top-level, and empty-string error paths.

The added tests double as live callers, keeping both functions out of the `deadcode -test` set so
the lint gate stays green while the API remains available to external consumers.

## Validation

`go build ./...` passed.

`go test ./pkg/aws/ ./pkg/utils/ -count=1` passed.

`go run golang.org/x/tools/cmd/deadcode@latest -test ./...` no longer reports
`ExecuteAwsEksUpdateKubeconfig` or `JSONToMapOfInterfaces`.

Building `cloudposse/terraform-provider-utils` against this branch (via a local `replace` and
`atmos@v1.226.1`) — `go build ./...` and `go vet ./internal/...` — both passed, confirming the
restored wrappers unblock the provider upgrade past v1.221.1.

## Follow-ups

Consider a lightweight guard (an exported-API golden list or a `deadcode` allowlist for
known external-consumer entry points) so future dead-code sweeps do not silently remove public
functions relied on by downstream modules such as `terraform-provider-utils`.
