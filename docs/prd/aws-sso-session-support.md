# AWS SSO Token Provider Support in Atmos

**Status**: Draft
**Last Updated**: 2026-05-19
**Owners**: Atmos auth subsystem

**Upstream references** (verified via AWS docs):
- AWS CLI — [Configure IAM Identity Center authentication with the AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html) (the `[sso-session]` config block)
- AWS CLI — [Token provider configuration with automatic authentication refresh](https://docs.aws.amazon.com/cli/latest/userguide/sso-configure-profile-token.html) (cache location + auto-refresh behavior)
- AWS CLI — [Configuration and credential file settings](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html) (`aws configure sso-session` command)
- AWS SDK for Go v2 — [Single sign-on credentials](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)
- AWS SDK for Go v2 — `ssocreds` package: [pkg.go.dev/github.com/aws/aws-sdk-go-v2/credentials/ssocreds](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/credentials/ssocreds)

**Related Atmos PRDs**:
- [SSO Role Auto-Discovery](./sso-role-auto-discovery.md) (orthogonal — composes cleanly)
- [PRD-Atmos-Auth](../../pkg/auth/docs/PRD/PRD-Atmos-Auth.md) (umbrella)
- [Auth Realm Architecture](./auth-realm-architecture.md) (orthogonal axis)
- [AWS Auth File Isolation](./aws-auth-file-isolation.md) (downstream credentials layout)

---

## 1. Executive Summary

### Problem

When a user configures multiple `aws/iam-identity-center` providers in `atmos.yaml` that
point at the same AWS SSO portal, atmos runs a **separate browser device-authorization
flow for each provider**. Each provider also writes its own per-provider OIDC token cache,
so renaming a provider — or simply having two providers for the same portal — invalidates
the cache and forces another browser hop. This contradicts the experience AWS itself
delivers: one sign-in to the SSO portal, every assigned permission set available.

### Solution

Refactor the `aws/iam-identity-center` provider to adopt the **AWS SSO token provider
behavior** that AWS CLI v2 and modern AWS SDKs already use: a shared OIDC token cache
keyed per SSO portal, with automatic refresh via the `refreshToken` returned from
`ssooidc:CreateToken` (see [AWS CLI token provider docs](https://docs.aws.amazon.com/cli/latest/userguide/sso-configure-profile-token.html)).
The provider's user-facing config is **unchanged**; the behavioral change is invisible
to the user except for the fact that one browser flow now unlocks every provider/identity
that targets the same portal.

### Value Proposition

- **One browser flow per SSO portal**, regardless of how many providers reference it
- **Cache interop with the `aws` CLI** — `aws sso login` and `atmos auth login` no
  longer require two separate logins
- **Silent refresh-token renewal** — atmos no longer re-runs the full device flow when
  a session expires within the 8-hour SSO portal limit
- **Zero migration burden** — `atmos.yaml` does not change shape

### Success Criteria

- Two providers with the same `start_url`+`region` complete `atmos auth login` with
  **one** browser interaction (today: two)
- After `aws sso login`, running `atmos auth login` triggers **zero** browser
  interactions (today: one)
- Session refresh inside the 8-hour portal window does not prompt for re-auth (today:
  always prompts)

---

## 2. Background: AWS SSO Token Provider

AWS CLI v2.9 (Nov 2022) introduced the `[sso-session]` block in `~/.aws/config` and a
standardized OIDC token cache under `~/.aws/sso/cache/`. The cache filename is derived
from the **SSO session name** (the label inside `[sso-session <name>]`), as documented
in [Token provider configuration with automatic authentication refresh](https://docs.aws.amazon.com/cli/latest/userguide/sso-configure-profile-token.html):

> The authentication token is cached to disk under the `~/.aws/sso/cache` directory with
> a filename based on the session name.

A canonical user config (verified against AWS CLI docs) looks like:

```ini
[profile my-dev-profile]
sso_session = my-sso
sso_account_id = 123456789011
sso_role_name = readOnly
region = us-west-2

[sso-session my-sso]
sso_region = us-east-1
sso_start_url = https://my-sso-portal.awsapps.com/start
sso_registration_scopes = sso:account:access
```

Every modern AWS SDK (including `aws-sdk-go-v2`) ships a token provider that:

1. Checks the cache for a non-expired access token
2. If the access token is expired but `refreshToken` is present and unexpired, refreshes
   silently against `ssooidc:CreateToken` with `grant_type=refresh_token`
3. Only falls back to a full device-authorization flow when both fail

Multiple AWS profiles that reference the same `sso-session` block share the cache file,
so `aws sso login --sso-session my-sso` populates a token that all of them use. This is
the behavioral model atmos's provider should adopt internally.

The relevant Go SDK package is
[`github.com/aws/aws-sdk-go-v2/credentials/ssocreds`](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/credentials/ssocreds),
which exposes `SSOTokenProvider` and a cached-token-provider pattern that implements
this layout.

### 2.1 Cache key wrinkle (important for atmos)

The AWS CLI keys the cache file on **session name** (`sha1(session_name)`), not on the
portal URL. Atmos providers, as configured today, don't have a "session name" — only
`start_url` + `region`. This affects how much interop atmos can offer with the `aws`
CLI's cache. Three resolutions are evaluated in §5.3.

---

## 3. Current State

### 3.1 Implementation (today)

`pkg/auth/providers/aws/sso.go::Authenticate()` (lines 106–240):

1. Loads cached token from `~/.cache/atmos/aws-sso/<provider-name>/token.json`
2. On cache miss: `ssooidc.RegisterClient` → `StartDeviceAuthorization` → browser prompt → poll `CreateToken`
3. Writes new token back to the per-provider cache file

`pkg/auth/providers/aws/sso_cache.go::ssoTokenCache` struct:

```go
type ssoTokenCache struct {
    AccessToken string    `json:"accessToken"`
    ExpiresAt   time.Time `json:"expiresAt"`
    Region      string    `json:"region"`
    StartURL    string    `json:"startUrl"`
}
```

Cache path derivation (`sso_cache.go:69-82`):

```
~/.cache/atmos/aws-sso/<provider-name>/token.json
                       ^^^^^^^^^^^^^^^ keyed by provider name
```

### 3.2 User-facing config (today, unchanged by this PRD)

```yaml
auth:
  providers:
    aws-sso-dev:
      kind: aws/iam-identity-center
      start_url: https://acme.awsapps.com/start
      region: us-east-1
    aws-sso-prod:
      kind: aws/iam-identity-center
      start_url: https://acme.awsapps.com/start   # same portal
      region: us-east-1                           # same region
```

Today: two browser flows. After this PRD: one.

### 3.3 Gaps

| Gap | Consequence |
| --- | --- |
| Cache keyed by provider name | Renaming a provider invalidates a still-valid token; two providers = two device flows |
| No `refreshToken` handling | Every expiry forces a new browser interaction even within the 8h portal window |
| Cache disjoint from `~/.aws/sso/cache/` | `atmos auth login` and `aws sso login` are independent — users do both |
| Device-flow concurrency | If two `atmos terraform` calls fire concurrently for different identities backed by the same portal, both run device flows |

---

## 4. Goals & Non-Goals

### Goals

1. One device-authorization flow per `(start_url, region)` tuple per cache lifetime,
   regardless of how many atmos providers reference it.
2. Refresh-token support so a long-running developer session doesn't re-prompt until
   the portal-side session truly expires.
3. Interop with `aws` CLI's `~/.aws/sso/cache/` (default behavior, opt-out available).
4. **No changes to `pkg/schema/schema_auth.go` or user-facing config shape.**
5. **No new provider kinds**; the existing `aws/iam-identity-center` provider absorbs
   the change.

### Non-goals

- Replacing or modifying SAML (`pkg/auth/providers/aws/saml.go`) or webflow
  (`webflow*.go`) providers
- Changes to identity chaining (`via.provider` / `via.identity`)
- Changes to realm isolation (`auth.realm`) — sessions are per-portal, realms are
  per-repo, they're orthogonal axes
- A new `auth.sso_sessions` top-level config block (explicitly rejected; portal identity
  is derived from existing `start_url`+`region`)
- Schema additions of any kind

---

## 5. Design

### 5.1 Implicit session identity

A "session" is the tuple `(start_url, region)`. Two providers that share this tuple
share a token. Within atmos's *in-process* `sessionTokenStore`, the key is derived
deterministically:

```
sessionKey = sha1(start_url + "|" + region)
```

This collapses N atmos providers pointing at the same portal into one shared token —
which delivers the primary goal (one browser flow per portal). The on-disk cache
**filename** is a separate question (see §5.3) because AWS CLI uses a different keying
scheme (`sha1(session_name)`).

### 5.2 Token acquisition flow (proposed)

```
ssoProvider.Authenticate(ctx):
  sessionKey = sha1(p.startURL + "|" + p.region)

  // 1. Check shared session cache (in-memory + on-disk)
  if token := sessionStore.Get(sessionKey); token.valid():
      return token.toCredentials()

  // 2. Acquire per-session mutex (defeats concurrent device flows for same session)
  sessionStore.Lock(sessionKey)
  defer sessionStore.Unlock(sessionKey)

  // 3. Double-check after lock acquired
  if token := sessionStore.Get(sessionKey); token.valid():
      return token.toCredentials()

  // 4. Try refresh-token flow (silent, no browser)
  if cached := readCache(sessionKey); cached.hasRefreshToken() && cached.refreshTokenValid():
      if newToken, err := refreshOIDCToken(ctx, cached); err == nil:
          writeCache(sessionKey, newToken)
          return newToken.toCredentials()

  // 5. Fall back to device-authorization flow (browser)
  newToken := runDeviceAuthFlow(ctx)
  writeCache(sessionKey, newToken)
  return newToken.toCredentials()
```

### 5.3 Cache layout

There are three layout options, each with a different interop story. PRD recommends
**Option B** (XDG cache with file-format compatibility) as the default — it captures
~80% of the value with none of the "atmos writes to your `~/.aws/` dir" surprise — and
provides **Option C** (full AWS CLI cache sharing) as an opt-in for users who want
zero-touch interop with `aws sso login`.

**Option A — XDG isolation, atmos-native format (rejected)**:

```
~/.cache/atmos/aws-sso/sessions/<sha1(start_url|region)>.json
```

Cheapest to implement, but the on-disk file format is atmos-specific; if the user runs
`aws sso login` separately, atmos can't read it. Rejected because it leaves the
"two-login problem" half-solved.

**Option B — XDG location, AWS-compatible file format (recommended default)**:

```
~/.cache/atmos/aws-sso/sessions/<sha1(start_url|region)>.json
```

File contents match the schema the AWS SDK's `ssocreds` package uses:

```json
{
  "startUrl": "https://acme.awsapps.com/start",
  "region": "us-east-1",
  "accessToken": "...",
  "expiresAt": "2026-05-19T15:00:00Z",
  "refreshToken": "...",
  "clientId": "...",
  "clientSecret": "...",
  "registrationExpiresAt": "2026-08-19T15:00:00Z"
}
```

This lets atmos reuse the same parsing/refresh logic as the AWS SDK while keeping atmos
out of the user's `~/.aws/` directory. It does **not** automatically share with
`aws sso login` (different cache path, different key).

**Option C — share with AWS CLI cache (opt-in)**:

```
~/.aws/sso/cache/<sha1(session_name)>.json
```

Activated by environment variable `ATMOS_AWS_SSO_USE_AWS_CLI_CACHE=true` (or future
config field outside the auth schema — exact mechanism TBD; not via `auth.*`).

Because the AWS CLI keys the cache by **session name**, atmos needs to know that name.
The provider has none today. Two viable strategies:

1. **Discover-and-match** — on cache lookup, atmos walks `~/.aws/sso/cache/*.json`,
   parses each file, and matches on the `startUrl` JSON field. Slow (linear in #cache
   files), but zero-config interop.
2. **Convention-based** — atmos derives a stable session name from the start_url
   hostname (e.g., `acme-awsapps-com`) and uses `sha1(that_name)`. If the user picks
   the same name when running `aws configure sso-session`, the cache files coincide.
   Cheap, but fragile.

PRD recommends **strategy 1 (discover-and-match)** for Option C, because cache
directories are typically small (< 10 files) and the cost is paid only on a cache miss.

### 5.3.1 Summary

| Option | Cache path | Interop with `aws sso login` | Schema additions |
| --- | --- | --- | --- |
| A (rejected) | XDG, atmos format | none | none |
| **B (default)** | XDG, AWS format | none, but easy future migration | none |
| C (opt-in) | `~/.aws/sso/cache/` | full | none (env-var gated) |

### 5.4 Concurrency: `sessionTokenStore`

A new in-process registry (`pkg/auth/providers/aws/sso_session_store.go`):

```go
type sessionTokenStore struct {
    mu       sync.Mutex
    locks    map[string]*sync.Mutex // key: sessionKey
    inMemory map[string]ssoTokenCache
}

// Acquire returns a per-session mutex; callers Lock/Unlock to single-flight device auth.
func (s *sessionTokenStore) Acquire(sessionKey string) *sync.Mutex
func (s *sessionTokenStore) Get(sessionKey string) (ssoTokenCache, bool)
func (s *sessionTokenStore) Put(sessionKey string, token ssoTokenCache)
```

Process-level locking (e.g., flock on the cache file) is **out of scope** for v1; today's
`manager.go` serializes `authenticateWithProvider` calls (`pkg/auth/manager.go:320`) and
typical user workflows are single-process. A future iteration can add flock if telemetry
shows multi-process contention.

### 5.5 SDK helper vs roll-our-own

Two paths; PRD recommends **roll our own** thin wrapper for v1, with a clear migration
path to [`ssocreds`](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/credentials/ssocreds)
later.

**`ssocreds.SSOTokenProvider` (rejected for v1)**:

- Pros: AWS owns the spec compliance; refresh handled inside the SDK; minimal new code
- Cons: Tightly couples atmos token resolution to AWS SDK's filesystem-only loader;
  hard to inject the `CacheStorage` interface used by atmos tests (see
  `sso_cache.go:31-42`); error messages bypass atmos's `errUtils.Build()` chain; the
  SDK loader assumes the AWS CLI cache location which conflicts with the Option B
  default in §5.3

**Roll our own (recommended for v1)**:

- Reuse existing `ssooidc.Client` calls — no new SDK packages, no new go.sum churn
- Keep `CacheStorage` interface for testability
- Add ~150 lines: refresh-token call, session-keyed cache layout, in-process store
- File-format-compatible with `ssocreds`, so migrating later is mechanical (drop-in
  replacement, not a re-architecture)

### 5.6 Backwards compatibility

- **Config**: zero changes. Existing `atmos.yaml` files keep working.
- **Cache migration**: on first post-upgrade `Authenticate()`, the provider writes the
  new session-keyed cache file. The old `~/.cache/atmos/aws-sso/<provider-name>/token.json`
  files become orphans. They are *not* deleted automatically (could be in-flight from
  another atmos version); they are cleaned up by `atmos auth logout`.
- **Logout**: `Logout()` in `sso.go:666-690` is updated to remove the session-keyed
  cache file. If multiple providers reference the same session, logging out of one
  removes the shared token — this is intentional and matches `aws sso logout` semantics.
- **`aws/sso` kind alias** (`sso.go:60-83` rejects only non-`aws/iam-identity-center`
  kinds) — verify all existing kind aliases continue to work; no behavior change.

### 5.7 Headless / CI behavior

Unchanged. The existing branch at `sso.go:127-138` already returns a helpful error in
non-interactive environments. After this change, CI users gain an additional path:
populate `~/.aws/sso/cache/` via `aws sso login` once before running atmos, and atmos
will find the cached token (when Option 1 is in effect).

---

## 6. Files to modify

| File | Change |
| --- | --- |
| `pkg/auth/providers/aws/sso.go` | Refactor `Authenticate()` body (~lines 106-240) to delegate to `sessionTokenStore`; add refresh-token call; preserve all error-builder chains, TUI prompts, and CI-detection branches |
| `pkg/auth/providers/aws/sso_cache.go` | Re-key cache file path; expand `ssoTokenCache` struct with `RefreshToken`, `ClientID`, `ClientSecret`, `RegistrationExpiresAt`; keep `CacheStorage` interface intact |
| `pkg/auth/providers/aws/sso_session_store.go` (new, ~80 lines) | In-process session registry with per-session mutex |
| `pkg/auth/providers/aws/sso_refresh.go` (new, ~60 lines) | Wraps `ssooidc.CreateToken` with `grant_type=refresh_token`; isolated for testability |
| `pkg/auth/providers/aws/sso_test.go`, `sso_cache_test.go` | Update fixtures for new cache key; add tests for refresh path and session sharing |

**Untouched**:

- `pkg/schema/schema_auth.go` (no schema changes)
- `pkg/auth/identities/aws/permission_set.go` (consumes `AWSCredentials.AccessKeyID` —
  no contract change)
- `pkg/auth/manager.go` (no manager-level changes needed)
- `pkg/auth/factory/` (no new kinds registered)
- All example `auth.yaml` files (no config-shape changes)

---

## 7. Telemetry

Add two events to track adoption and verify success criteria:

- `auth.aws.sso.session_cache_hit` — fired when `Authenticate()` returns from the
  session cache (no device flow, no refresh)
- `auth.aws.sso.refresh_token_used` — fired when the refresh path succeeds
- `auth.aws.sso.device_flow_run` — fired when device auth is needed (counts the
  "browser hops" metric the PRD's success criteria target)

Existing telemetry calls in `sso.go` (CI detection, etc.) are preserved.

---

## 8. Verification

### 8.1 Unit tests

- Cache key derivation: `sha1("https://acme.awsapps.com/start|us-east-1")` matches
  expected SHA1 (golden value)
- Refresh-token flow: mock `ssooidc.Client.CreateToken` returning a new token on
  `grant_type=refresh_token` — verify cache is updated and no device flow triggered
- Session sharing: two `ssoProvider` instances with identical `(start_url, region)`
  share the same in-memory `sessionTokenStore` entry — only one device flow runs
- Distinct sessions: providers with differing `start_url` or `region` get independent
  cache entries and independent device flows
- CacheStorage interface still satisfied by `defaultCacheStorage`; tests inject mocks

### 8.2 Integration tests

- `tests/fixtures/scenarios/auth-realm-profile-bug/` — verify realm isolation is
  unaffected (sessions are per-portal, realms are per-repo)
- New scenario `tests/fixtures/scenarios/auth-sso-session-sharing/` with two providers
  sharing a portal, asserts only one device-flow prompt fires

### 8.3 Manual acceptance

1. Configure two `aws/iam-identity-center` providers with the same `start_url`+`region`
2. Run `atmos auth login` → exactly **one** browser interaction
3. Run `aws sso login --sso-session <any-named-session-pointing-at-same-portal>`
4. Run `atmos auth login` → **zero** browser interactions
5. Wait until `accessToken` expires (or shorten via debug flag), re-run `atmos auth
   login` → no browser; refresh-token path used

### 8.4 Non-regression

- `make testacc` clean
- Existing `sso_test.go` and `sso_extended_test.go` updated and green
- Snapshot tests (`tests/snapshots/`) for `atmos auth login` output unchanged in CI mode
- `atmos auth logout` clears the session cache (verified by next login triggering
  device flow)

---

## 9. Open Questions

1. **Default cache location** — PRD currently recommends **Option B** (XDG location,
   AWS-compatible file format). Needs sign-off; the alternative is Option C (write
   into `~/.aws/sso/cache/` for full `aws sso login` interop). Tradeoff is
   "predictable / atmos-owned" vs. "one login works in both tools."
2. **AWS CLI cache-key reconciliation (only relevant if Option C is chosen)** —
   `aws sso login` keys the cache by **session name** (`sha1(session_name)`); atmos has
   no session name. PRD §5.3 proposes "discover-and-match" (walk
   `~/.aws/sso/cache/*.json` and match the `startUrl` field). Alternative is a
   convention-based name derived from the start_url. Decide before implementation.
3. **`sso_extended_test.go` and `sso_provisioning.go` interaction** — the SSO
   auto-provisioning flow (see [sso-role-auto-discovery PRD](./sso-role-auto-discovery.md))
   calls `ListAccounts` with the access token. After this change, that token comes
   from the shared session cache; verify the auto-provisioning code path is unaffected
   by the cache key rename. Spike before implementation starts.
4. **Multi-process locking** — today's serial usage doesn't need `flock`, but a
   developer running `atmos terraform plan` in two terminals could race. Defer to a
   follow-up issue (open at PR-merge time per CLAUDE.md follow-up tracking rules), or
   include `flock` from day one? PRD recommends defer.

---

## 10. Migration & Rollout

- **Phase 1 (this PRD)**: implementation behind no flag; cache change is automatic
- **Phase 2 (next release)**: telemetry-driven check that `device_flow_run` event
  count dropped vs. previous release for users with multi-provider configs
- **Phase 3 (later)**: if Option 1 (AWS CLI interop) causes issues, revisit Option 2
  as the default

---

## 11. Out of Scope

- Implementation code (this PRD is design only; follow-up PR will implement)
- SAML provider changes
- Webflow / `aws/user` provider changes
- Schema additions to `pkg/schema/schema_auth.go`
- New CLI flags or commands
- Multi-process / cross-host token sharing
