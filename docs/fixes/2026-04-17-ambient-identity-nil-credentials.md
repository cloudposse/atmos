# Fix: SIGSEGV authenticating standalone `ambient` identity

**Date:** 2026-04-17

**Issue:**

- `pkg/auth/identities/ambient/ambient.go:66-71` —
  `ambientIdentity.Authenticate()` returns `(nil, nil)` by design: the
  generic `ambient` kind is a pure passthrough and does not manage
  credentials.
- `pkg/auth/manager.go:294` — `manager.Authenticate()` forwards those
  `nil` credentials to `buildWhoamiInfo(identityName, finalCreds)`
  without a nil check.
- `pkg/auth/manager_whoami.go:24-26` — `buildWhoamiInfo` dereferences
  the credential interface unconditionally (`creds.BuildWhoamiInfo(info)`
  and `creds.GetExpiration()`), producing a `SIGSEGV` on a nil
  interface value.

The AWS-specific `aws/ambient` identity is unaffected because its
`Authenticate()` returns real `*types.AWSCredentials` resolved via the
AWS SDK default chain; only the generic `ambient` kind panics.

## Status

**Fixed.** `buildWhoamiInfo` now short-circuits safely when credentials
are nil — it still returns a populated `WhoamiInfo` (realm, provider,
identity, environment, timestamp) but skips the credential-dependent
branches (`BuildWhoamiInfo`, `GetExpiration`, keystore cache, reference
handle). This matches the contract the generic `ambient` kind advertises
in the PRD (`docs/prd/ambient-identity.md`): credentials are resolved by
the cloud SDK at subprocess runtime, and Atmos has nothing to cache or
expose on the whoami surface.

### Progress checklist

- [x] Reproduce: added `TestManager_buildWhoamiInfo_NilCredentials` —
  before the fix, this test panicked at
  `manager_whoami.go:25` with `runtime error: invalid memory address
  or nil pointer dereference` while calling
  `creds.BuildWhoamiInfo(info)`. Asserts no panic and a populated
  identity/provider/realm/environment on nil credentials.
- [x] Reproduce end-to-end: added `TestManager_Authenticate_Ambient_Standalone` —
  before the fix, this test panicked in the same location reached
  through the real `manager.Authenticate()` entry point
  (`manager.go:294 → manager_whoami.go:25`). Asserts no panic and a
  valid `*WhoamiInfo` with `Provider == "ambient"`,
  `Identity == "passthrough"`, `Credentials == nil`.
- [x] Fix: nil-check `creds` in `buildWhoamiInfo` before any method
  call, keystore store, or reference-handle assignment. Environment
  is populated unconditionally (it does not depend on credentials).
- [x] Confirm: both new tests pass; `TestManager_buildWhoamiInfo_SetsRefAndEnv`
  and all subtests of `TestManager_buildWhoamiInfoFromEnvironment`
  still pass (non-nil credentials path unchanged).
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

```bash
atmos auth login --identity passthrough
# → SIGSEGV: segmentation violation
#   pkg/auth/manager_whoami.go:25 creds.BuildWhoamiInfo(info)
```

### Code path

1. `manager.Authenticate(ctx, "passthrough")` builds chain `[passthrough]`.
2. `authenticateChain` → `authenticateFromIndex` → detects standalone
   `ambient` and calls `ambient.AuthenticateStandaloneAmbient(...)`.
3. `AuthenticateStandaloneAmbient` invokes
   `identity.Authenticate(ctx, nil)`. The ambient identity is a
   no-op and returns `(nil, nil)` by design — the generic `ambient`
   kind does not own or resolve credentials.
4. Back in `manager.Authenticate()` at line 294:
   `return m.buildWhoamiInfo(identityName, finalCreds), nil`.
5. `buildWhoamiInfo` at `manager_whoami.go:25` calls
   `creds.BuildWhoamiInfo(info)` on the nil interface → panic.

### Why `aws/ambient` does not panic

`aws/ambient.Authenticate()` calls `config.LoadDefaultConfig(ctx)` and
returns a populated `*types.AWSCredentials` built from whatever the
AWS SDK default chain resolves (IRSA, IMDS, ECS task role, env vars).
The credential object is always non-nil, so the subsequent
`creds.BuildWhoamiInfo(info)` call dispatches to a valid method.

The generic `ambient` kind has no analogous object to return — it is
cloud-agnostic and intentionally does not materialize credentials.
Fixing the manager to handle nil credentials is the correct layer for
this contract; forcing the identity to fabricate a credential stub
would leak implementation details into every cloud-agnostic
passthrough.

## Fix

### Where

`pkg/auth/manager_whoami.go:buildWhoamiInfo`

### What

Short-circuit before any method is invoked on `creds`:

```go
// Populate environment from identity regardless of creds.
if identity, exists := m.identities[identityName]; exists {
    if env, err := identity.Environment(); err == nil {
        info.Environment = env
    }
}

// Ambient identities (generic "ambient" kind) return nil credentials
// — credentials are resolved by the cloud SDK at subprocess runtime,
// not by Atmos. There is nothing to populate on WhoamiInfo or cache
// in the keystore.
if creds == nil {
    return info
}

// From here on, creds is non-nil: populate high-level fields, cache
// in the keystore, wire up the reference handle.
info.Credentials = creds
creds.BuildWhoamiInfo(info)
// ...
```

The order matters: environment is still populated for the nil-creds
path because `identity.Environment()` does not depend on credentials
and callers (e.g. `atmos auth whoami`) expect an identity+environment
surface even when Atmos is a pure passthrough.

### Why not handle this in the identity

- The generic `ambient` kind is cloud-agnostic. Synthesizing a
  credential stub would require picking a concrete cloud type
  (`*AWSCredentials`? `*AzureCredentials`?) and would misrepresent
  the semantics — there are no credentials, by design.
- The `AuthenticateStandaloneAmbient` function already documents that
  ambient returns nil credentials (`pkg/auth/identities/ambient/ambient.go:154`).
  The contract is intentional; the bug is the manager failing to
  honor it.

## Tests

### Unit — `buildWhoamiInfo` directly

`pkg/auth/manager_whoami_test.go::TestManager_buildWhoamiInfo_NilCredentials`

1. Construct `manager` with a registered ambient-kind identity and a
   test credential store.
2. Call `m.buildWhoamiInfo("passthrough", nil)`.
3. Assert no panic; `info.Provider`, `info.Identity`, `info.Realm`,
   `info.Environment` are populated; `info.Credentials == nil`;
   `info.CredentialsRef == ""`; nothing was written to the keystore.

### Integration — full `Authenticate()` flow

`pkg/auth/manager_whoami_test.go::TestManager_Authenticate_Ambient_Standalone`

1. Build a real `manager` via `NewAuthManager` with an ambient identity.
2. Call `m.Authenticate(ctx, "passthrough")`.
3. Assert no panic; returned `*WhoamiInfo` has
   `Identity == "passthrough"`, `Provider == "ambient"`,
   `Credentials == nil`.

---

## Related

- `docs/prd/ambient-identity.md` — feature PRD. Specifies that
  `ambient.Authenticate()` returns `nil, nil` and `ambient` identities
  do not store credentials.
- `pkg/auth/identities/ambient/ambient.go:66-71` — the intentional
  `return nil, nil` in `ambientIdentity.Authenticate()`.
- `pkg/auth/identities/ambient/ambient.go:144-162` —
  `AuthenticateStandaloneAmbient` already documents and propagates the
  nil-credentials contract.
- `pkg/auth/identities/aws/ambient.go:Authenticate` — the AWS-specific
  counterpart that returns real `*AWSCredentials` and therefore never
  triggers this bug.
- `pkg/auth/manager.go:294` — the `Authenticate()` return site that
  forwards creds to `buildWhoamiInfo` without filtering.
- `pkg/auth/manager_whoami.go:buildWhoamiInfo` — the function fixed
  by this change.
