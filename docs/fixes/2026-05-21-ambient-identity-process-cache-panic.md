# Fix: SIGSEGV from process credential cache when authenticating standalone `ambient` identity twice

**Date:** 2026-05-21

**Issue:**

- `pkg/auth/identities/ambient/ambient.go:66-71` —
  `ambientIdentity.Authenticate()` returns `(nil, nil)` by design: the
  generic `ambient` kind is a pure passthrough and does not manage
  credentials. The earlier fix in
  [`docs/fixes/2026-04-17-ambient-identity-nil-credentials.md`](2026-04-17-ambient-identity-nil-credentials.md)
  taught `manager_whoami.buildWhoamiInfo` to honor this contract.
- `pkg/auth/manager_chain.go:85-87` (pre-fix) — after a successful
  standalone-ambient authentication, `authenticateChain` stored the
  resulting `nil` credentials in the process-level credential cache
  (`processCredentialCache`):

  ```go
  processCredentialCache.Store(cacheKey, &processCachedCreds{
      credentials: creds, // creds == nil for standalone ambient
  })
  ```
- `pkg/auth/manager_chain.go:56-61` (pre-fix) — on the next call for
  the same chain, the fast-path cache hit invoked
  `m.isCredentialValid("process-cache", cached.credentials)` with a
  nil `cached.credentials`.
- `pkg/auth/manager_chain.go:164` (pre-fix) — `isCredentialValid`
  immediately called `cachedCreds.GetExpiration()` on the nil
  `types.ICredentials` interface, producing a `SIGSEGV` on a nil
  interface value.

The AWS-specific `aws/ambient` identity is unaffected because its
`Authenticate()` returns real `*types.AWSCredentials` resolved via the
AWS SDK default chain — never nil — so the cache hit always validates
a real credential object. Only the generic `ambient` kind triggers
this panic, and only on the second (or later) authentication of the
same chain in the same process.

## Status

**Fixed.** Two-layer defense:

1. `authenticateChain` no longer stores nil credentials in
   `processCredentialCache`. Re-authenticating a standalone ambient
   identity is a no-op, so skipping the cache costs nothing and
   preserves the cache's behavioral guarantee that every entry is a
   usable credential object.
2. `isCredentialValid` short-circuits on a nil
   `types.ICredentials` input and returns `(false, nil)`, matching
   the same defense-in-depth pattern `buildWhoamiInfo` adopted in the
   2026-04-17 fix. If any future caller stores nil in the cache (or
   another code path passes nil into the validator), the worst case
   is a redundant re-authentication, never a panic.

### Progress checklist

- [x] Reproduce: added
  `TestManager_isCredentialValid_NilCreds` — before the fix, this
  test panicked at `manager_chain.go:164` with `runtime error:
  invalid memory address or nil pointer dereference` while calling
  `cachedCreds.GetExpiration()`. Asserts `(false, nil)` on nil
  credentials.
- [x] Reproduce end-to-end: added
  `TestManager_Authenticate_AmbientStandalone_RepeatedCallsNoPanic` —
  before the fix, this test panicked on the second `Authenticate()`
  call against the same standalone `ambient` identity in the same
  process, via the real `manager.Authenticate()` → `authenticateChain`
  process-cache fast-path. Asserts both calls return without panic
  and both yield `WhoamiInfo.Credentials == nil`.
- [x] Reproduce cache contract: added
  `TestAuthenticateChain_AmbientStandalone_DoesNotCacheNil` — asserts
  that after a standalone ambient authentication, the process
  credential cache holds no entry for the chain. This locks in the
  authenticateChain-side fix and prevents a regression where caching
  nil silently returns.
- [x] Fix (cache write): nil-check `creds` in `authenticateChain`
  before writing to `processCredentialCache`. Documented in a
  comment why the skip is safe for ambient and references the
  isCredentialValid defense.
- [x] Fix (cache read): nil-check `cachedCreds` at the top of
  `isCredentialValid`. Mirrors the comment style used by the
  2026-04-17 fix in `buildWhoamiInfo`.
- [x] Confirm: all three new tests pass; the original
  `TestManager_buildWhoamiInfo_NilCredentials` and
  `TestManager_Authenticate_Ambient_Standalone` (from the
  2026-04-17 fix) still pass.
- [x] Regression: `go test ./pkg/auth/... -count=1` — all packages
  green. `go build ./...` succeeds. `go vet ./pkg/auth/...` clean.

---

## Problem

### Reproducer

```yaml
auth:
  identities:
    passthrough:
      kind: ambient
```

Any command that authenticates the same standalone ambient identity
twice in the same process — most notably `atmos describe affected
--upload`, which resolves per-component auth across many components —
panics on the second authentication:

```bash
atmos describe affected --upload --skip terraform.state --base refs/heads/main
# ... first ambient auth succeeds, returns (nil, nil) and is cached as nil ...
# → SIGSEGV: segmentation violation
#   pkg/auth/manager_chain.go:164 cachedCreds.GetExpiration()
```

A minimal reproducer in code is two back-to-back `manager.Authenticate(ctx, "passthrough")`
calls on a manager configured with a single `kind: ambient` identity
— see `TestManager_Authenticate_AmbientStandalone_RepeatedCallsNoPanic`.

### Why it surfaced now

Two unrelated upstream changes converged to make this fatal in
production:

- `#2411` (shipped in v1.219.0) made per-component
  auth resolver failures fatal in
  `internal/exec/describe_stacks_component_processor.go:166-168`.
  Before #2411, the panic was caught and the parent auth manager was
  silently substituted; after #2411, the panic propagates and the
  whole command aborts with exit code 1.
- The 2026-04-17 ambient fix (`#2334`) only addressed
  the panic in `buildWhoamiInfo`. It did not touch the process-level
  credential cache path in `authenticateChain` /
  `isCredentialValid`, which is dormant for `atmos auth login` /
  `atmos auth whoami` (single authentication per process) but hot
  for `atmos describe affected` (many components, repeated chain
  resolution).

The panic was therefore latent in v1.217.0 / v1.218.0 (when the
2026-04-17 fix shipped) and became user-visible in v1.219.0 when
per-component auth errors became fatal.

### Code path

1. `atmos describe affected --upload ...` iterates components in
   `internal/exec/describe_stacks_component_processor.processComponentEntry`
   and resolves per-component auth via
   `resolveComponentAuthManager` → `createComponentAuthManager` →
   `CreateAndAuthenticateManagerWithAtmosConfig`.
2. For each component whose default identity is the standalone
   ambient identity, `manager.Authenticate("passthrough")` is called.
3. **First call:**
   - `authenticateChain` cache miss.
   - `authenticateFromIndex` recognizes a standalone ambient chain and
     calls `ambient.AuthenticateStandaloneAmbient` →
     `identity.Authenticate(ctx, nil)` → returns `(nil, nil)`.
   - `authenticateChain` stores `&processCachedCreds{credentials: nil}`
     in `processCredentialCache`.
   - `manager.Authenticate` calls `buildWhoamiInfo(name, nil)` which
     correctly short-circuits (2026-04-17 fix) and returns a valid
     `*WhoamiInfo`. No panic.
4. **Second call (next component sharing the same identity):**
   - `authenticateChain` cache hit at line 56 — `cached.credentials`
     is nil.
   - `m.isCredentialValid("process-cache", nil)` invoked.
   - `nil.GetExpiration()` on the nil `ICredentials` interface
     → **SIGSEGV**.

### Why `aws/ambient` does not trip this

`aws/ambient.Authenticate()` calls `config.LoadDefaultConfig(ctx)` and
returns a populated `*types.AWSCredentials` built from whatever the
AWS SDK default chain resolves. The credential object is always
non-nil, so the process cache stores a real credential and the
subsequent `cachedCreds.GetExpiration()` dispatches to a valid method.

The generic `ambient` kind has no analogous object to return — it is
cloud-agnostic and intentionally does not materialize credentials.
Fixing the manager to handle nil cached credentials is the correct
layer for this contract; forcing the identity to fabricate a
credential stub would leak implementation details into every
cloud-agnostic passthrough (and contradict the PRD).

## Fix

### Where

- `pkg/auth/manager_chain.go:isCredentialValid` — nil-check input.
- `pkg/auth/manager_chain.go:authenticateChain` — skip caching nil.

### What

**Cache read (`isCredentialValid`):** short-circuit on nil before any
method call on `cachedCreds`.

```go
func (m *manager) isCredentialValid(identityName string, cachedCreds types.ICredentials) (bool, *time.Time) {
    // Guard against a nil ICredentials interface. Generic ambient identities
    // (`kind: ambient`) return (nil, nil) from Authenticate by design — they
    // do not manage credentials. Calling GetExpiration on a nil interface
    // would panic, so treat nil as "not valid for cache reuse" and let the
    // caller fall through to re-authentication. The ambient re-auth path is
    // a no-op, so this is cheap.
    if cachedCreds == nil {
        log.Debug("Cached credentials are nil; treating as invalid", logKeyIdentity, identityName)
        return false, nil
    }
    // ... existing expiration check ...
}
```

**Cache write (`authenticateChain`):** don't store nil in the
process cache.

```go
// Cache the successfully authenticated credentials for this process.
//
// Skip caching for nil credentials. Generic ambient identities
// (`kind: ambient`) return (nil, nil) from Authenticate by design — they
// do not manage credentials. Storing nil here would cause the fast-path
// cache hit on the next call to invoke isCredentialValid(_, nil), which
// historically dereferenced a nil ICredentials interface and panicked.
// Defense-in-depth nil check also lives in isCredentialValid; this skip
// avoids the redundant cache lookup entirely. Re-authentication for
// ambient is a no-op, so the savings the cache provides are immaterial
// for this kind.
if creds != nil {
    processCredentialCache.Store(cacheKey, &processCachedCreds{
        credentials: creds,
    })
}
```

Either guard alone closes the panic. Both together make the
contract explicit at both the read and write sites and prevent a
future caller (or a future identity kind that also legitimately
returns nil credentials) from silently reintroducing the bug.

### Why not handle this in the identity

- The generic `ambient` kind is cloud-agnostic. Synthesizing a
  credential stub at the identity layer would require picking a
  concrete cloud type and would misrepresent the semantics — there
  are no credentials, by design.
- The `AuthenticateStandaloneAmbient` function already documents that
  ambient returns nil credentials
  (`pkg/auth/identities/ambient/ambient.go:154`). The contract is
  intentional; the bug was the manager's credential cache failing to
  honor it.
- The 2026-04-17 fix established the precedent that nil-credential
  handling belongs in the manager layer. This fix extends that same
  principle to the credential cache.

## Tests

### Unit — `isCredentialValid` directly

`pkg/auth/manager_chain_ambient_test.go::TestManager_isCredentialValid_NilCreds`

1. Construct a bare `manager` (no config needed — the nil-check is
   the first statement of `isCredentialValid`).
2. Call `m.isCredentialValid("process-cache", nil)`.
3. Assert no panic; `valid == false`; `expTime == nil`.

### Integration — repeated `Authenticate()` flow

`pkg/auth/manager_chain_ambient_test.go::TestManager_Authenticate_AmbientStandalone_RepeatedCallsNoPanic`

1. Build a real `manager` via `NewAuthManager` with a single
   `kind: ambient` identity.
2. Call `m.Authenticate(ctx, "passthrough")` twice, asserting no
   panic and `WhoamiInfo.Credentials == nil` each time.
3. Resets `processCredentialCache` at the start and via `t.Cleanup`
   to keep the test hermetic.

### Cache contract — no nil written

`pkg/auth/manager_chain_ambient_test.go::TestAuthenticateChain_AmbientStandalone_DoesNotCacheNil`

1. Same setup as the integration test.
2. After one `Authenticate()` call, inspect `processCredentialCache`
   for the chain's cache key.
3. Assert the key is absent — locks in the
   `authenticateChain`-side fix.

---

## Related

- [`docs/fixes/2026-04-17-ambient-identity-nil-credentials.md`](2026-04-17-ambient-identity-nil-credentials.md)
  — the predecessor fix. Added the same nil-check pattern in
  `buildWhoamiInfo`. This fix extends the same contract to the
  process credential cache.
- `docs/prd/ambient-identity.md` — feature PRD. Specifies that
  `ambient.Authenticate()` returns `(nil, nil)` and `ambient`
  identities do not store credentials.
- `pkg/auth/identities/ambient/ambient.go:66-71` — the intentional
  `return nil, nil` in `ambientIdentity.Authenticate()`.
- `pkg/auth/identities/ambient/ambient.go:144-162` —
  `AuthenticateStandaloneAmbient` already documents and propagates
  the nil-credentials contract.
- `pkg/auth/identities/aws/ambient.go:Authenticate` — the
  AWS-specific counterpart that returns real `*AWSCredentials` and
  therefore never triggers this bug.
- `pkg/auth/manager_chain.go:authenticateChain` — the cache-write
  site fixed by this change.
- `pkg/auth/manager_chain.go:isCredentialValid` — the cache-read
  site fixed by this change.
- `internal/exec/describe_stacks_component_processor.go:150-174` —
  the per-component auth resolver whose 1.219.0 change
  (`#2411`) made this latent panic fatal in
  `atmos describe affected --upload` workflows.
- `#2334` — the 2026-04-17 fix PR for the
  `buildWhoamiInfo` panic.
- `#2411` — the 1.219.0 PR that made
  per-component auth resolver failures fatal, surfacing this latent
  panic in `atmos describe affected --upload`.
