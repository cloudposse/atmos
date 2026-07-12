# Fix: `atmos.Component()` / `!terraform.output` nested auth could reuse the wrong identity, and never resolved a target's own identity

**Date**: 2026-07-09

**Related**: [#2652](https://github.com/cloudposse/atmos/pull/2652) (per-component auth scoping/caching
in `describe stacks`), [#2656](https://github.com/cloudposse/atmos/pull/2656) (dedupe per-identity auth
in nested `!terraform.state` resolution), and the sibling fix
`docs/fixes/2026-06-27-store-hook-inherit-default-identity.md` (same "no EC2 IMDS role found" symptom
in the terraform-hooks path).

## Problem

A user reported `atmos list instances` (GitHub Actions, GitHub-OIDC auth, no ambient AWS credentials)
regressing between v1.221.0 and v1.222.0. A Go template calling `atmos.Component(a, stack-a)` — while
fetching a **different** nested component's terraform output via the templated `outputs` field — failed:

```text
Error: No valid credential sources found
Error: failed to refresh cached credentials, no EC2 IMDS role found,
operation error ec2imds: GetMetadata, expect HTTP transport, got <nil>
```

v1.221.0 logs for the same run show a fresh GitHub OIDC authentication before every single terraform
output fetch; v1.222.0 shows far fewer, reusing cached `AuthManager`s introduced by #2652/#2656.

## Root cause

Two independent, confirmed defects in the per-component/nested auth-caching work added in v1.222.0:

1. **Cache-key collision for wrapper-rooted parents.** `atmos.Component()` and `!terraform.state` /
   `!terraform.output` propagate the enclosing component's `AuthContext` into a nested lookup by
   wrapping it in `authContextWrapper` (`internal/exec/terraform_output_utils.go`).
   `authContextWrapper.GetChain()` deliberately always returns `[]string{}` (a non-empty chain would
   incorrectly make a nested target with its own default-identity `auth:` section inherit the *caller's*
   identity instead of its own — see `createComponentAuthManager`). But #2656's
   `buildComponentAuthCacheKey` (`internal/exec/terraform_nested_auth_helper.go`) uses `parent.GetChain()`
   as part of the cache key meant to disambiguate "inherited vs. auto-detected" identity. Because that
   chain is always empty for every `authContextWrapper`, two *different* real identities propagated this
   way collapse onto the same cache key whenever the nested targets they reference declare
   structurally-identical `auth:` sections — silently reusing one identity's `AuthManager` (and
   credentials) for a different identity's nested lookup.

2. **`atmos.Component()` and `!terraform.output` never consulted the target's own auth section.**
   Unlike `!terraform.state` (which routes through `resolveAuthManagerForNestedComponent` in
   `internal/exec/terraform_state_utils.go`), both `atmos.Component()`
   (`internal/exec/template_funcs_component.go`) and the `!terraform.output` YAML function
   (`internal/exec/yaml_func_terraform_output.go` → `pkg/terraform/output.Executor.GetOutput`) always
   fetched the target's terraform output using the *enclosing* component's `AuthContext` verbatim,
   even when the target declares its own default-identity `auth:` section. (For `!terraform.output`
   the target-aware resolution was lost when output fetching moved into `pkg/terraform/output`, which
   cannot call back into `internal/exec`.) This is a pre-existing asymmetry (not new in v1.222.0)
   that the caching work exposed more sharply.

## Fix

- `buildComponentAuthCacheKey` now treats any `*authContextWrapper` parent as **non-cacheable** — such
  lookups always resolve fresh instead of risking a false-positive cache hit. `GetChain()`'s existing
  (intentional) empty-chain behavior is untouched.
- `atmos.Component()` now resolves the nested target's own `auth:` section via
  `resolveAuthManagerForNestedComponent` (extracted as `resolveComponentFuncAuthManager` for testing),
  falling back to the enclosing component's `AuthContext` exactly as before when the target has no
  default identity of its own or auth is disabled — bringing it in line with `!terraform.state`.
- `!terraform.output` now does the same: `defaultOutputGetter.GetOutput`
  (`internal/exec/terraform_output_getter.go`) resolves the target's own auth via
  `resolveNestedOutputAuth` (same fallback semantics) before delegating to
  `pkg/terraform/output.GetOutput`, and the YAML entry propagates `AuthDisabled` via
  `authContextWrapper` when no AuthManager exists, mirroring `!terraform.state`.

Tests: `TestBuildComponentAuthCacheKey_AuthContextWrapperNeverCaches`
(`internal/exec/terraform_nested_auth_cache_test.go`), `TestResolveComponentFuncAuthManager`
(`internal/exec/template_funcs_component_test.go`), `TestResolveNestedOutputAuth`
(`internal/exec/terraform_output_getter_test.go`).

## Open question — not fully resolved

The user confirmed the specific nested component in their report declares **no** `auth:` override of
its own. Neither fix above changes behavior for that exact case (both only engage when the *nested
target* has its own default-identity auth section). The credentials for that fetch come entirely from
the *enclosing* component's inherited `AuthContext` (propagated from the top-level run identity via
`propagateAuth` in `internal/exec/describe_stacks.go`), and we could not reproduce, via static analysis
or unit tests alone, why that `AuthContext` would be empty specifically in v1.222.0 for a long,
many-stack `--all-sections` pass. The on-disk AWS credentials file mechanism
(`pkg/auth/cloud/aws/files.go`) was inspected and found to be additive/lock-protected, ruling out a
simple file-clobbering explanation.

If this recurs, capture `atmos list instances --logs-level Debug` output around the failing
`atmos.Component(...)` call and check whether the *enclosing* component (not the target) has its own
`auth:` override that is being cache-reused — that would point at the top-level
`describeStacksProcessor.authManagerCache` (`internal/exec/describe_stacks_component_processor.go`)
instead of the nested-path cache fixed here. (That cache was audited as part of this fix and shares
`buildComponentAuthCacheKey`, so it inherits the wrapper guard; its keys are safe for real managers.)

## Known same-class issues deliberately deferred (follow-ups)

A codebase audit for this bug class found two more systemic variants, out of scope here because each
needs a design decision:

1. **Credential-fetched result caches are keyed by stack+component only, ignoring identity.**
   `terraformOutputsCache` (`pkg/terraform/output/executor_utils.go`), `terraformStateCache`
   (`internal/exec/terraform_state_utils.go`), and `componentFuncSyncMap`
   (`internal/exec/template_funcs_component.go`) all cache data fetched *using credentials* without
   any identity discriminator in the key. Benign when two identities read the same backend (same
   content), but wrong when the `AuthContext` changes the endpoint (emulator identities vs. real
   cloud — the exact aliasing the S3/GCS/Azure backend *client* caches in
   `internal/terraform_backend/` are explicitly keyed to prevent). It also means a freshly resolved
   per-identity AuthManager can still be handed another identity's cached *result*. Fix direction:
   fold an identity/endpoint discriminator into these keys, mirroring the backend client caches, or
   make wrapper-context fetches non-cacheable like `buildComponentAuthCacheKey` does.

2. **Store default-identity injection is inconsistent across command paths.** Only the terraform
   execute path (`internal/exec/terraform_execute_helpers.go`,
   `SetAuthContextResolverWithDefaultIdentity`) and the after-terraform hook path
   (`cmd/terraform/utils.go`) inject the run's default identity into the store registry.
   `describe component` (`internal/exec/describe_component.go`,
   `injectDescribeComponentStoreAuthResolver`) and the `secret` commands (`cmd/secret/shared.go`)
   inject a resolver *without* the default identity, and paths like describe stacks / list
   instances / validate / packer / helmfile / workflows inject nothing — so identity-less `!store`
   reads there fall back to ambient SDK credentials and fail under Atmos auth with the same
   "no EC2 IMDS role found" symptom as `docs/fixes/2026-06-27-store-hook-inherit-default-identity.md`.
   Fix direction: reuse `HookStoreDefaultIdentity` on those paths.

Smaller related notes from the same audit: Azure credentials are keyed by provider name, not
identity (`pkg/auth/hooks.go` TODO — two identities sharing a provider overwrite each other), and
`gomplateDatasourceFuncSyncMap` (`internal/exec/template_funcs_gomplate_datasource.go`) keys by
alias only, ignoring call args (caching correctness, not auth).
