# Fix: store identity no longer leaks into later `!terraform.state` reads

**Date:** 2026-10-26

## Summary

Resolving a store's `identity:` (for `!secret`, `!store`, and store hooks) authenticated that identity on the
component's shared auth manager. That overwrote the manager's authentication chain and auth context, so every
`!terraform.state` read evaluated afterwards ran as the store identity and failed with `AccessDenied` when the
state role trusted only the component identity. Store identities now authenticate on an isolated manager, and
the component's chain and credentials are never changed by a store read. Fixes issue #3544.

## Context

`pkg/store/authbridge.Resolver` called `Authenticate` on the component manager and then read the result back
from `GetStackInfo().AuthContext`. Two consumers depend on the state that call overwrote:

- Nested `!terraform.state` targets with their own default-identity auth section inherit the last element of
  the parent manager's chain (`createComponentAuthManager` in `internal/exec/terraform_nested_auth_helper.go`).
  The same chain feeds `storeAutoDetectedIdentity` and `HookStoreDefaultIdentity`.
- Backend reads use the manager's `AuthContext.AWS`, which is aliased into the component's stack info.

YAML function evaluation order is not fixed, so a different read failed on each run, and `--identity=deployer`
did not help because inheritance reads the chain rather than the identity flag. The deferred evaluation path
(`pkg/store/deferred`, `pkg/secrets/deferred`) already copies stack info and builds a separate manager per
effective auth config, so it was unaffected; only the non-deferred resolver used by `atmos terraform *` leaked.

## Changes

- `pkg/auth/types/interfaces.go`: `AuthManager` gains `AuthenticateIsolated(ctx, identityName)
  (*schema.AuthContext, error)`. The mock in `mock_interfaces.go` is regenerated.
- `pkg/auth/manager_isolated.go` (new): builds a separate manager from a copy of the effective auth config
  (`CopyGlobalAuthConfig`), the shared credential store and validator, the same `cliConfigPath` (so the realm
  matches), and a fresh stack info carrying only stack metadata. It authenticates there and returns that
  manager's auth context. The receiver's chain, stack info, and auth context are untouched. Sharing the
  credential store preserves keyring caching and refresh behavior. No result cache is introduced; stores
  already resolve credentials once per process when they initialize their SDK client.
- `pkg/store/authbridge/resolver.go`: all three `Resolve*AuthContext` methods call `AuthenticateIsolated` and
  map fields from the returned context through the existing `resolvedContext`. Error wrapping and the
  `ErrAuthContextNotAvailable` behavior for a missing cloud section are unchanged. The obsolete comment about
  reading back from the manager's stack info is replaced.
- `pkg/auth/deferred/manager.go`: delegates `AuthenticateIsolated` through `base()`, so a disabled manager
  returns `ErrAuthenticationUnavailable` like every other operation.
- `internal/exec/terraform_output_utils.go`: `authContextWrapper.AuthenticateIsolated` returns an
  `ErrNotImplemented` error rather than authenticating or panicking.
- Hand-rolled `AuthManager` test stubs in `pkg/auth` gained the method. The CloudFormation target auth test
  (`pkg/component/aws/cloudformation/target_auth_test.go`) now also expects `AuthenticateIsolated`, because
  artifact and store credentials there flow through the same bridge.
- `pkg/auth/manager_isolated_bench_test.go` (new): `BenchmarkStoreIdentityResolution` compares the old shared
  `Authenticate` path with `AuthenticateIsolated` on a warm memory keyring, so the per-call construction cost
  stays measurable. It is a local reproducer and is not part of the curated Mage benchmark inventory.

Store identity precedence, nested identity inheritance, and the deferred store resolver are unchanged by
design; they become correct once the chain stops being overwritten.

Known limit: isolation protects manager-owned state. It does not protect process environment. The GCP
project identity exports its environment with `os.Setenv` during post-authentication regardless of which
manager runs it; that is pre-existing and outside this fix.

## Validation

- `pkg/auth/manager_isolated_test.go`: parent chain, AWS auth context, config maps, stack info pointer, and
  auth context pointer are unchanged after isolated authentication succeeds, fails (unknown or empty identity),
  or runs with a canceled context. Returned contexts do not alias the parent or each other, and mutating one
  does not reach the other. A concurrent case runs isolated calls alongside the parent's own `Authenticate`
  under `-race`.
- `pkg/store/authbridge/resolver_test.go`: AWS, Azure, and GCP success, authentication failure, nil cloud
  section, and nil context cases against `AuthenticateIsolated`; a test asserts the resolver never calls
  `Authenticate` or `GetStackInfo` on the manager. `resolver_isolation_test.go` uses a real manager with the
  `mock/aws` kinds and asserts the chain and AWS context are unchanged after resolving a store identity.
- `internal/exec/terraform_store_identity_isolation_test.go`: real manager with `deployer` (default),
  `automation`, and `auditor`, wired through `injectTerraformStoreAuthResolver` into identity-aware stores.
  Covers state → secret → state, secret → state, explicit `--identity=deployer`, two pinned store identities,
  stores inheriting the component identity, and auth disabled. After each store read it asserts the shared
  chain, the manager and component auth contexts, `resolvedTargetAuthContext`, `storeAutoDetectedIdentity`,
  `HookStoreDefaultIdentity`, and the identity a nested target inherits via `createComponentAuthManager` are
  all still `deployer`. A companion test swaps in the old shared-manager behavior and proves the harness
  detects the leak. Against the pre-fix resolver (checked with `go test -overlay` on the HEAD version of
  `resolver.go`) five of six cases failed with chain `[mock automation]` and profile `automation`.
- `go build ./...`, `go vet` on the touched packages, `gofumpt -l` on every changed file: clean.
- `go test -race -count=1` on `pkg/auth`, `pkg/auth/deferred`, `pkg/store/authbridge`, `pkg/store/deferred`,
  and `pkg/secrets/...`: all pass.
- `go test -count=1 ./internal/exec/`: 2529 passed, 0 failed, 27 skipped.
- `go test -short -count=1` across every package except `./tests`: 485 packages passed. One failure was
  caused by this change and fixed (the CloudFormation target auth test mocked the old `Authenticate` call);
  that package passes after the mock update. The only other failing package was `pkg/runner/step`, whose cast
  playback tests timed out waiting for PTY output; see the note on it below.
- `atmos lint --changed`: 0 issues.
- Performance: `BenchmarkStoreIdentityResolution` (10 samples each, Apple M4 Max, machine otherwise idle)
  compared via `go tool benchstat`. Shared `Authenticate` 217.4µs/op versus `AuthenticateIsolated`
  217.2µs/op, p=0.796 (no detectable time change). Allocations 48.15 KiB → 52.91 KiB (+9.9%) and
  463 → 487 allocs/op (+5.2%), both p=0.000, which is the cost of constructing the separate manager. Call
  frequency is unchanged: identity-aware stores resolve once per process when they build their SDK client,
  and the SOPS cloud-KMS path resolves once per master key per decrypt exactly as before. The curated Mage
  CLI store workloads exercise the deferred authentication path, which this change does not touch, so a
  `bench:fieldTest` run would not discriminate the fix from its base and was not run.
- `pkg/runner/step`: seven cast playback tests fail locally with "timed out waiting for cast output" from the
  PTY session. They fail identically on a pristine checkout of the base commit in a separate worktree, the
  package's dependency on the changed code is only through the shared interface, and no test in it touches
  authentication. This is a pre-existing local environment issue, not a regression from this change.
- The CLI test package (`./tests`) was not run locally; on this machine it hangs on a pre-existing podman
  auto-start path unrelated to this change (documented in project notes). CI runs it with Docker present.

## Follow-ups

None.
