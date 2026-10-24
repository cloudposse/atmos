# Fix: SOPS cloud-KMS secrets now authenticate via the Atmos identity

**Date**: 2026-06-20
**Status**: ✅ FIXED
**Issue**: [#2637](https://github.com/cloudposse/atmos/issues/2637)

## Problem Summary

`atmos secret` commands (and `!secret` resolution during `terraform plan`) against a SOPS cloud-KMS
backend only worked when cloud credentials were already present in the process environment. They did
**not** authenticate using `--identity` / `ATMOS_IDENTITY` / the per-provider `identity` / the
stack/component effective identity, so every operation had to be wrapped in `atmos auth exec`:

```bash
# Failed (identity ignored, no ambient creds):
atmos secret set ACME_API_KEY=abc123 --stack=acme-use1-prod --component=acme/app --identity=acme-prod/terraform
# Error: failed to decrypt SOPS file: Error getting data key: 0 successful groups required, got 0

# Only worked when wrapped:
atmos auth exec --identity acme-prod/terraform -- \
  atmos secret set ACME_API_KEY=abc123 --stack=acme-use1-prod --component=acme/app
```

Track-1 store backends (AWS SSM, Secrets Manager, Azure Key Vault, GCP Secret Manager) already
authenticated via the identity; only the SOPS cloud-KMS track was affected. The decrypt error also
printed an age-key hint that does not apply to a KMS-encrypted file.

## Root Cause

The SOPS provider's key service (`pkg/secrets/providers/sops.go`, `keyClient()`) returned getsops'
`keyservice.NewLocalClient()` for every non-age key type. For AWS/GCP/Azure KMS master keys that
local client resolves cloud credentials **only from the ambient credential chain** (environment,
`~/.aws`, instance metadata). The provider had no reference to the Atmos auth system, and the
`secrets.providers.<name>.identity` field defined in the schema was never read.

Separately, the data-key **encryption** path (`writeNewFile`) called `tree.GenerateDataKey()`, which
encrypts the data key with each master key directly — also bypassing any key service.

## Fix

The cloud is now inferred from the SOPS file's actual master-key type at runtime and credentials are
resolved from the Atmos identity — there is **no per-cloud `kind`** and no process-environment
mutation. Credentials are injected into the getsops master key via its `ApplyToMasterKey` mechanism.

1. **Transient auth seam** — `schema.AtmosConfiguration.SecretsAuth` (a `*store.SecretsAuthContext`
   carrying an `AuthContextResolver` + effective default identity) is populated alongside the store
   auth resolver in both the `atmos secret` path (`cmd/secret/shared.go`) and the terraform/`!secret`
   path (`internal/exec/terraform_execute_helpers.go`).
2. **SOPS provider as its own package** — `pkg/secrets/providers/sops/` with the core provider plus a
   **registry of per-cloud key handlers** (`aws.go`, `gcp.go`, `azure.go`), each registered by
   getsops key-type identifier (`kms` / `gcp_kms` / `azure_kv`). A composed key service
   (`keyservice.go`) routes each key type to its handler when an identity resolves, and delegates age
   / pgp / everything else to a fallback local client.
3. **Provider-agnostic boundary** — the cloud-SDK credential building lives in the depguard-exempt
   `pkg/store/sopsauth/` bridge (`aws.go` / `gcp.go` / `azure.go` / `builder.go`); the SOPS package
   imports no cloud SDK directly.
4. **Identity precedence** — per-provider `secrets.providers.<name>.identity` > `--identity` /
   `ATMOS_IDENTITY` > the stack/component effective identity.
5. **Encryption path fixed** — `writeNewFile` now uses `GenerateDataKeyWithKeyServices` so a fresh
   data key is encrypted with the identity's credentials too.
6. **Kind-aware error hints** — `decryptErr` inspects the file's real key types and emits identity /
   permission hints for cloud-KMS files (and age hints for age files), removing the misleading
   age-key hint on KMS failures.

**Backward compatible**: when no identity resolves, the provider falls back to the local client, so
the ambient-credential behavior is unchanged. `kind` remains only for the legitimate age-vs-KMS
keygen distinction; it no longer gates credential handling.

## Backends audited

| Backend | Status |
|---|---|
| `sops/aws-kms`, `sops/gcp-kms`, `sops/azure-kv` | Fixed — authenticate via identity |
| `sops/age`, `sops/pgp` | Unaffected (local key material) |
| Stores: AWS SSM, AWS Secrets Manager, Azure Key Vault, GCP Secret Manager | Already identity-aware |
| Store: HashiCorp Vault | Fixed — AWS IAM auth via identity (was token-only) |
| Stores: Redis, Artifactory, 1Password, Keychain, GitHub Actions | N/A (no cloud identity) |

## Tests

- **Unit** (`pkg/secrets/providers/sops/keyservice_test.go`): key-service selection (identity present
  → wraps; absent → ambient fallback), per-cloud handler registry dispatch, identity precedence, and
  kind-aware error hints.
- **E2E** (`tests/sops_kms_floci_test.go`): `atmos secret set`/`get` against a SOPS `aws-kms` backend
  on the Floci KMS emulator with **all ambient AWS credentials cleared**, succeeding solely via the
  `floci-superuser` identity — the exact #2637 scenario, which fails before the fix.
