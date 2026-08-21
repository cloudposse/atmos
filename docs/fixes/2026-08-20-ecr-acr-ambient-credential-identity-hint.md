# Fix: AWS ECR / Azure ACR ambient-credential login failures now hint at configuring an Atmos identity

**Date:** 2026-08-20

## Summary

`atmos aws ecr login --registry`/`--public` and `atmos azure acr login --registry` use an
ambient-credential fallback (the plain AWS/Azure SDK default credential chain) instead of Atmos's
identity system. When no Atmos identity was configured and the ambient chain also had nothing to
offer, the chain's last resort (EC2 IMDS for AWS, managed identity for Azure) would time out off
those clouds, and the user only saw the raw SDK error — e.g. `no EC2 IMDS role found ... context
deadline exceeded` — with no indication that configuring an Atmos identity was the fix.

## Context

A user ran `atmos app build` and hit:

```
ECR authentication failed: failed to retrieve AWS credentials: failed to refresh cached credentials, no EC2
IMDS role found, operation error ec2imds: GetMetadata, request canceled, context deadline exceeded
```

Diagnosis traced this to `pkg/auth/cloud/aws/ecr.go`'s `LoadDefaultAWSCredentials`, which is only
invoked on the ambient-credential ECR login paths (`cmd/aws/ecr/login.go`'s
`executeExplicitRegistries` and `executePublicLoginAmbient`) — it never touches Atmos's own
`pkg/auth` identity system. A follow-up check found the identical pattern in
`pkg/auth/cloud/azure/acr.go`'s `LoadDefaultAzureCredentials` (used by `cmd/azure/acr/login.go`'s
`executeExplicitRegistries`), whose `azidentity.NewDefaultAzureCredential` default chain includes a
Managed Identity Credential that probes Azure IMDS. GCP has no container-registry (Artifact
Registry/GCR) integration at all yet, so there was nothing to fix on that cloud.

The ambient-credential escape hatch itself is intentional (for callers already authenticated
outside Atmos, e.g. a CI job running under an IAM role) and was kept as-is. The actual defect was
the failure mode: a bare SDK error with no pointer to the fix, which reads as an Atmos bug rather
than a config gap.

## Changes

- `pkg/auth/cloud/aws/ecr.go`: `LoadDefaultAWSCredentials`'s `awsCfg.Credentials.Retrieve(ctx)`
  failure branch now uses `errUtils.Build(errUtils.ErrECRAuthFailed).WithCause(err).WithExplanation(...).WithHint(...)`
  instead of a plain `fmt.Errorf`. The hint points at `atmos aws ecr login --identity <name>` or
  configuring `via.identity` under `auth.integrations`.
- `pkg/auth/cloud/azure/acr.go`: `LoadDefaultAzureCredentials`'s `cred.GetToken(...)` failure branch
  gets the same treatment, hinting at `atmos azure acr login --identity <name>`.
- Both changes preserve the original sentinel errors (`ErrECRAuthFailed`, `ErrACRAuthFailed`) via
  `errUtils.Build(...)`'s auto-sentinel marking, and preserve the underlying SDK error text via
  `WithCause`, so `errors.Is()` checks against either the sentinel or the original error still hold.
- The other early-return branches in each function (`config.LoadDefaultConfig` /
  `azidentity.NewDefaultAzureCredential` construction failures) were left as plain wraps — those
  indicate malformed SDK config, not "no identity configured," so a hint there would mislead.

## Validation

- Added `TestLoadDefaultAWSCredentials_RetrieveFailureHasIdentityHint`
  (`pkg/auth/cloud/aws/ecr_test.go`) and `TestLoadDefaultAzureCredentials_RetrieveFailureHasIdentityHint`
  (`pkg/auth/cloud/azure/acr_test.go`), each using a pre-canceled `context.Context` to force the
  failure branch deterministically without a real network/IMDS call. Both assert `errors.Is` against
  the sentinel still holds and that `cockroachdb/errors.GetAllHints` contains the new hint text.
- `go test ./pkg/auth/...` — all packages pass.
- `atmos lint --changed` — 0 issues.

## Follow-ups

None.
