# Fix: `atmos secret` CLI inherits the component's default identity

**Date:** 2026-07-13
**Related:** #2662, `docs/fixes/2026-06-27-store-hook-inherit-default-identity.md`, `docs/fixes/2026-07-09-atmos-component-nested-auth-cache-collision.md`

## Problem

Running the `atmos secret` CLI (`set`, `get`, `init`, `validate`) against a store-backed secret whose store declares no explicit `identity` failed off-EC2 with:

```text
failed to get parameter '/acme/secrets/dev/app/API_KEY': operation error SSM: GetParameter,
... no EC2 IMDS role found, ... dial tcp 169.254.169.254:80: connect: host is down
```

The failure hit only the mutating/verifying secret commands; `atmos terraform ...` and `atmos secret list` worked - the tell-tale of a code-path divergence.

## Root Cause

`injectSecretStoreAuthResolver` (`cmd/secret/shared.go`) called `atmosConfig.Stores.SetAuthContextResolver(resolver)`, which passes an empty identity to every store. An identity-less store therefore kept no identity and fell back to the AWS default credential chain (EC2 IMDS). The terraform paths (`cmd/terraform/utils.go`, `internal/exec/terraform_execute_helpers.go`) already call `SetAuthContextResolverWithDefaultIdentity(resolver, defaultIdentity)`; the secret CLI even computed the same `DefaultIdentity` (into `SecretsAuth`) but never applied it to the stores.

## Fix

`injectSecretStoreAuthResolver` now calls `SetAuthContextResolverWithDefaultIdentity(resolver, defaultIdentity)`, reusing the `defaultIdentity` it already computes.

### Why

An identity-less store-backed secret backend must inherit the run's effective identity - the same behavior the terraform-hook path already has (`docs/fixes/2026-06-27-store-hook-inherit-default-identity.md`; the 2026-07-09 nested-auth write-up explicitly flagged `cmd/secret/shared.go` as injecting a resolver *without* the default identity). It also matches the documented rule that a store-backed secret with no explicit `identity` inherits the component's effective identity.

## Backward compatibility

No config or API changes. Stores that declare an explicit `identity` keep it (`defaultIdentityForStore` only fills empty-identity stores). An explicit `--identity` still wins. `atmos terraform` and `atmos secret list` behavior is unchanged.

## Tests

- `cmd/secret.TestInjectSecretStoreAuthResolver_AppliesDefaultIdentity` - with no `--identity`, an identity-less SSM store resolves the chain-tail default onto `SecretsAuth`; the store-level application is asserted by the existing `pkg/store.TestSetAuthContextResolverWithDefaultIdentity_DefaultsOnlyEmptyStores`.
- `cmd/secret.TestInjectSecretStoreAuthResolver_ResolverOnly` - unchanged: an explicit-identity mock store still receives its own identity (a mock is not a concrete store type, so the default is not applied to it).

```shell
go test ./cmd/secret/ -run 'InjectSecretStoreAuthResolver' -count=1
```

## Expected Behavior

- `atmos secret set`/`get`/`init`/`validate` against a store with no explicit `identity` inherits the component's effective identity (per stack) instead of falling back to EC2 IMDS.
