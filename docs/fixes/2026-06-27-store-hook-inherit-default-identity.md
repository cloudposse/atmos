# Fix: store-output hooks inherit the run's default identity

**Date**: 2026-06-27

**Related**: [#2625](https://github.com/cloudposse/atmos/pull/2625) (added store identity support;
explicitly deferred identity-less inheritance in the hook path), and the fix doc
`docs/fixes/2026-06-17-aws-stores-secrets-auth-and-gists.md`.

## Problem

Under Atmos auth, `atmos terraform apply` on a component with an after-apply `store-outputs` hook
applies successfully, then **fails in the hook** when the target store has no `identity` of its own:

```text
INFO  Running hooks event=after.terraform.apply status=success
✓ Fetching nat_gateway_public_ips output from vpc in plat-usw2-dev
Error: failed to assume write role: failed to assume role
arn:aws:iam::…:role/…-terraform: operation error STS: AssumeRole, … get identity:
get credentials: failed to refresh cached credentials, no EC2 IMDS role found,
operation error ec2imds: GetMetadata, … dial tcp 169.254.169.254:80: connect: no route to host
```

The same configuration worked before the move to Atmos auth (e.g. under Leapp), and `atmos auth
whoami` shows a valid, current identity. Adding an explicit `identity:` to every store works around
it, but that is verbose and easy to miss.

## Root Cause

Store credential resolution and the terraform lifecycle hooks resolve auth on two different code
paths:

- **Main terraform path** (`internal/exec.injectTerraformStoreAuthResolver`): after authenticating,
  it persists the auto-detected identity and injects the store resolver with that identity as the
  default, via `StoreRegistry.SetAuthContextResolverWithDefaultIdentity`. So `!store` reads during a
  run, and stores without their own `identity`, inherit the run's identity.

- **Hook path** (`cmd/terraform.prepareHookContext` → `injectHookStoreAuthResolver`): hooks run in a
  **freshly loaded** Atmos config, so the apply-phase store registry (and its injected default
  identity) is gone. The hook re-injected the resolver with `SetAuthContextResolver` — resolver
  **only**, no default identity. Identity-less stores therefore fell through to the default AWS SDK
  credential chain, which is empty under Atmos auth (credentials live in the keyring, not the
  environment), so the SDK tried EC2 IMDS and failed.

This asymmetry was intentional and called out in #2625 as a deferred compatibility decision
("Component-identity inheritance for identity-less stores is intentionally left for a follow-up
design decision"). This change makes that decision: the hook path should inherit the run's identity
just like the main path.

## Fix

### What

- The after-apply hook path now injects the store resolver **with the run's default identity** so
  identity-less stores invoked by hooks inherit the same identity the component applied as.
- A store that declares its own `identity` keeps it; a run without Atmos auth (no resolved identity)
  keeps the prior ambient/default-SDK behavior.

### Why

- It removes a surprising asymmetry: `!store` reads during a run already inherit the run identity,
  but hook writes did not. Users had to duplicate `identity:` onto every store solely to satisfy the
  hook path.
- It is the minimal, backward-compatible completion of the store-auth work started in #2625.

### How

- New helper `internal/exec.HookStoreDefaultIdentity(authManager, info)` mirrors the main path:
  it calls `storeAutoDetectedIdentity` (populating `info.Identity` from the auth manager's chain leaf
  when no explicit identity is set), then normalizes the value through `storeDefaultIdentity`
  (empty / `--identity=select` / `--identity=disabled` → `""`, i.e. no inheritance). It returns `""`
  when no auth manager is present.
- `cmd/terraform.injectHookStoreAuthResolver` now calls
  `atmosConfig.Stores.SetAuthContextResolverWithDefaultIdentity(resolver, e.HookStoreDefaultIdentity(authManager, info))`
  instead of `SetAuthContextResolver(resolver)`.
- `StoreRegistry.SetAuthContextResolverWithDefaultIdentity` applies the default only to identity-aware
  stores that have no `identity` of their own.
- Fixed an adjacent omission surfaced by the end-to-end test: `defaultIdentityForStore` (the helper
  that decides which stores receive the default) handled `*SSMStore`, `*AzureKeyVaultStore`, and
  `*GSMStore`, but **not** `*SecretsManagerStore` (`aws/asm`). AWS Secrets Manager stores without an
  `identity` were therefore never given the default identity on any path. `*SecretsManagerStore` is
  now included, so `aws/asm` behaves like `aws/ssm`.

## Backward compatibility

- `HookStoreDefaultIdentity` returns `""` whenever no identity is resolved (no auth manager, empty /
  `select` / `disabled` identity). `SetAuthContextResolverWithDefaultIdentity("")` is a no-op for the
  default, so runs without Atmos auth keep using ambient/default SDK credentials exactly as before.
- Stores with an explicit `identity` are never overridden.

## Tests

- `internal/exec.TestHookStoreDefaultIdentity` — table test of the resolution/normalization logic:
  nil manager, chain-leaf auto-detection, empty chain, explicit identity preserved (chain not
  consulted), `select` sentinel resolves to chain leaf, `disabled` sentinel yields no inheritance.
- `cmd/terraform.TestInjectHookStoreAuthResolver_InheritsDefaultIdentity` — replaces the previous
  `..._ResolverOnly` test; asserts the hook now auto-detects the active identity (`GetChain`),
  populates `info.Identity`, and still wires the resolver into stores.
- `pkg/store.TestSetAuthContextResolverWithDefaultIdentity_DefaultsOnlyEmptyStores` — proves
  identity-less concrete stores inherit the default while stores with their own identity keep it.
  Updated so the identity-less `aws/asm` store now asserts inheritance (it previously asserted the
  buggy empty-identity behavior).
- `tests.TestAWSStoreHooks_InheritedIdentity_FlociE2E` — opt-in Floci E2E with fixture
  `tests/fixtures/scenarios/aws-store-hooks-floci-inherit`: stores declare **no** `identity`, a
  `default: true` identity is configured, ambient AWS credentials are cleared, and the after-apply
  hook must still write to SSM and Secrets Manager. Fails on the old behavior, passes with the fix.

Run the focused suite:

```shell
go test ./internal/exec ./cmd/terraform ./pkg/store -run 'HookStoreDefaultIdentity|InjectHookStoreAuthResolver|SetAuthContextResolverWithDefaultIdentity' -count=1
```

Run the Floci E2E against a running emulator:

```shell
ATMOS_TEST_FLOCI=true FLOCI_ENDPOINT_URL=http://localhost:4566 \
  go test ./tests -run 'TestAWSStoreHooks(_InheritedIdentity_)?FlociE2E' -count=1 -v
```

## Expected Behavior

- A component's after-apply `store-outputs` hook can write outputs to AWS SSM / Secrets Manager (and
  the Azure/GCP equivalents) **without** a per-store `identity`, by inheriting the identity the run
  authenticated as.
- A store with an explicit `identity` continues to use it.
- A run without Atmos auth keeps the prior ambient/default SDK credential behavior.
