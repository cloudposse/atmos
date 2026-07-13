# Dedupe per-identity auth in nested `!terraform.state` resolution

## Summary

Follow-up to the `describe stacks` per-component auth fix
(`2026-06-22-describe-stacks-scope-and-cache-per-component-auth.md`). That change
scoped and memoized per-component authentication at the **top level** of a
`describe stacks` / `list` / `terraform --all` pass. This change extends the same
per-identity memoization to the **nested** resolution path that runs while
templates and YAML functions are evaluated.

## Problem

When a component references another component via `!terraform.state` /
`!terraform.output`, resolution flows through:

```text
GetTerraformState (terraform_state_utils.go)
  └─ resolveAuthManagerForNestedComponent (terraform_nested_auth_helper.go)
       └─ createComponentAuthManager
            └─ auth.CreateAndAuthenticateManagerWithAtmosConfig   // expensive: credential writes, file locks, keyring
```

`terraformStateCache` (keyed by `stack-component`) already short-circuits a
**repeat read of the same target** before auth runs. The gap: **distinct target
components that share one identity** each run a full auth cycle. On a large stack
whose components fan out to many shared-identity targets, this reproduces the same
N-auth blowup the top-level fix removed — just relocated into template/YAML
resolution.

`atmos.Component(...)` and `!terraform.output` only trigger auth cycles
transitively (when the component they describe itself contains `!terraform.state`);
they inherit the parent auth context directly and are not independent sources.

## Fix

Memoize `resolveAuthManagerForNestedComponent` results in a process-scoped cache
(`nestedAuthManagerCache`), mirroring the existing `terraformStateCache` /
`componentFuncSyncMap` pattern in this package.

- **Key:** parent auth chain + a deterministic JSON fingerprint of the component's
  auth section (`buildComponentAuthCacheKey`). Identities are defined globally and
  only *referenced* by components, so "same auth section" is a provable proxy for
  "same identity"; it never merges components whose auth differs. The parent chain
  disambiguates an inherited identity from an auto-detected one. A section that
  cannot be serialized is reported non-cacheable and resolves without caching.
- **Shared key function:** `buildComponentAuthCacheKey` is now used by **both** the
  `describe stacks` processor and the nested path, so the two keying strategies
  cannot drift.
- **Only successful, non-nil results are cached.** A resolver error is never
  memoized, so a transient auth failure does not poison the cache.
- **Reset:** `ResetNestedAuthManagerCache()` clears it; `ResetStateCache()` clears
  it alongside the state cache, because the managers in `nestedAuthManagerCache`
  are what read the state in `terraformStateCache` — clearing one while reusing the
  other would be inconsistent. Neither is reset in production.

## Safety

Sharing one authenticated manager across components is safe **iff** the auth
section (and therefore the resolved identity) is identical — the same assumption
the top-level processor cache already relies on. Lifetime is process-scoped
(command-per-process), identical to the sibling state/component caches; credentials
do not expire within a single seconds-long pass.

## Example

For a stack `acme-use1-tools` whose components cross-reference shared-identity
targets, the count of `CreateAndAuthenticateManager called` debug lines during
`atmos describe stacks -s <stack>` drops from roughly one-per-distinct-target to
roughly one-per-identity, with identical output.
