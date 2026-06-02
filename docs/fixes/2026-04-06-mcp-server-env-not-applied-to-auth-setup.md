# Fix: MCP server `env:` block not applied during auth setup (ATMOS_PROFILE / ATMOS_CLI_CONFIG_PATH ignored)

**Date:** 2026-04-07
**Issue:** [#2283](https://github.com/cloudposse/atmos/issues/2283)
**PR:** [#2291](https://github.com/cloudposse/atmos/pull/2291)

## Problem

When configuring an external MCP server in `.atmos.d/mcp.yaml` with `ATMOS_PROFILE`
(or any other `ATMOS_*` variable) under the `env:` block, the value is not picked up
during the auth setup phase. Identity resolution happens against the parent process's
default profile and the server fails to start with `identity not found`.

The same identity resolves correctly when `ATMOS_PROFILE` is exported in the shell
**before** running `atmos mcp test ...`.

### Reproduction

```yaml
# .atmos.d/mcp.yaml
mcp:
  enabled: true
  servers:
    atmos:
      description: Atmos MCP server
      command: atmos
      args: ["mcp", "start"]
      env:
        ATMOS_PROFILE: managers
      identity: core-root/terraform
```

```bash
$ atmos mcp test atmos
✗ Server failed to start
   Error: MCP server failed to start: atmos:
     auth setup failed for "atmos": identity not found: core-root/terraform
```

Workaround that should not be necessary:

```bash
$ ATMOS_PROFILE=managers atmos mcp test atmos     # works
```

## Root Cause

The auth manager is constructed **once, in the parent process, before any MCP server
is spawned**. It uses `cfg.InitCliConfig`, which reads `ATMOS_PROFILE`,
`ATMOS_CLI_CONFIG_PATH`, `ATMOS_BASE_PATH`, etc. from `os.Environ()`. The server's
`env:` block is only applied to the **subprocess** environment in `connectAndDiscover`
(via `cmd.Env = env`), so the parent's auth manager never sees it.

```text
parent atmos                                 subprocess
─────────────                                ───────────
1. cfg.InitCliConfig()    ← parent ATMOS_PROFILE (empty)
2. CreateAndAuthenticateManagerWithAtmosConfig
   → identities loaded from default profile
   → "core-root/terraform" NOT in this set
3. session.Start(opts...)
   ├─ WithAuthManager runs PrepareShellEnvironment
   │  → identity lookup fails ✗
   └─ (subprocess never spawned)
```

The two affected entry points were `cmd/mcp/client/start_options.go` (used by
`atmos mcp test/tools/status/restart`) and `cmd/ai/init.go` (used by `atmos ai
ask/chat/exec`).

## Fix

Auth manager construction is now **deferred and per-server**: each server's `env:`
block is applied to `ATMOS_*` variables in the parent process for the duration of
manager construction, then immediately restored.

### 1. New `PerServerAuthProvider` interface

`pkg/mcp/client/session.go`:

```go
// PerServerAuthProvider is an optional extension of AuthEnvProvider that
// returns a new AuthEnvProvider scoped to a specific server's configuration.
type PerServerAuthProvider interface {
    ForServer(ctx context.Context, config *ParsedConfig) (AuthEnvProvider, error)
}
```

`WithAuthManager` checks for this interface and prefers the per-server path when
present:

```go
provider := authMgr
if perServer, ok := authMgr.(PerServerAuthProvider); ok {
    scoped, err := perServer.ForServer(ctx, config)
    if err != nil { ... }
    provider = scoped
}
return provider.PrepareShellEnvironment(ctx, config.Identity, env)
```

The non-per-server callers continue to work unchanged.

### 2. New `env.SetWithRestore` foundational primitive (in `pkg/env`)

`pkg/env/restore.go`:

```go
// SetWithRestore sets the given environment variables on the parent process
// and returns a cleanup closure that reverts every variable to its original
// state (including unsetting variables that did not exist before the call).
func SetWithRestore(vars map[string]string) (func(), error) { ... }
```

- **Generic** — does *not* filter by key prefix. Any caller-supplied map is
  honored. Filter policy belongs at the caller, not the primitive.
- Returns a restore closure that reverts each variable to its previous state
  (set to original value, or unset if it didn't exist).
- Idempotent — calling cleanup twice is a no-op after the first call.
- Returns `(cleanup, err)` even on partial failure so callers can
  `defer cleanup()` safely in error paths.
- NOT goroutine-safe — mutates `os.Environ()`. Callers must serialize.

This lives in `pkg/env` because Atmos already has a dedicated environment
package, and "set env vars with save/restore" is a generic env utility — not
an auth primitive. Atmos has at least four pre-existing local variants of
this same idea (see "Follow-up" below); `env.SetWithRestore` is the
foundation they should all consolidate to.

### 3. New `auth.CreateAndAuthenticateManagerWithEnvOverrides` primitive

`pkg/auth/manager_env_overrides.go`:

```go
// CreateAndAuthenticateManagerWithEnvOverrides builds and authenticates an
// AuthManager "as if" the given ATMOS_* environment variables were set.
// Composition of env.SetWithRestore (filtered to ATMOS_*) +
// cfg.InitCliConfig + CreateAndAuthenticateManagerWithAtmosConfig.
func CreateAndAuthenticateManagerWithEnvOverrides(envOverrides map[string]string) (AuthManager, error) { ... }
```

- Calls `env.SetWithRestore` for the actual env mutation — no save/restore
  logic lives in `pkg/auth`.
- Filters the input map to `ATMOS_*` keys via a small `filterAtmosOverrides`
  helper. The filter policy is intentionally caller-specific: this primitive
  is scoped to atmos config/identity resolution, so other ATMOS-unrelated
  keys are silently ignored. Callers wanting to mutate arbitrary env should
  use `env.SetWithRestore` directly.
- The auth manager's identity map is populated during construction, so the
  `defer restore()` running before the function returns does not affect
  subsequent use of the manager.

Placing the high-level primitive in `pkg/auth` follows Atmos's existing
precedent of co-locating auth operations and avoids each subsystem growing
its own factory.

### 4. Thin MCP adapter: `ScopedAuthProvider`

`pkg/mcp/client/scoped_auth.go` is now a ~70-line adapter. Its only job is to:

1. Satisfy `AuthEnvProvider` and `PerServerAuthProvider` (interfaces local to
   `pkg/mcp/client`).
2. Plumb `ParsedConfig.Env` into `auth.CreateAndAuthenticateManagerWithEnvOverrides`.
3. Convert `(nil, nil)` into `errUtils.ErrMCPServerAuthUnavailable` with
   server + identity context so callers can `errors.Is`-match and display
   actionable info.

```go
func (p *ScopedAuthProvider) ForServer(_ context.Context, config *ParsedConfig) (AuthEnvProvider, error) {
    mgr, err := p.buildManagerFn(config.Env) // defaults to auth.CreateAndAuthenticateManagerWithEnvOverrides
    if err != nil { return nil, fmt.Errorf("failed to build auth manager for MCP server %q: %w", config.Name, err) }
    if mgr == nil {
        return nil, fmt.Errorf("%w: server %q, identity %q", errUtils.ErrMCPServerAuthUnavailable, config.Name, config.Identity)
    }
    return mgr, nil
}
```

Both call sites — `cmd/mcp/client/start_options.go` (management commands)
and `cmd/ai/init.go` (AI commands) — now do a single line:

```go
return []mcpclient.StartOption{mcpclient.WithAuthManager(mcpclient.NewScopedAuthProvider(atmosConfig))}
```

No auth-factory code lives in `cmd/`, and `pkg/mcp/client` does not
reimplement auth primitives — it consumes them from `pkg/auth`. This
addresses review feedback that each subcommand should not grow its own
per-command auth factory.

### 5. Canonical Atmos env-var namespace in `pkg/config`

Reviewer feedback flagged that defining `const AtmosEnvPrefix = "ATMOS_"`
inside any specific subsystem (originally `pkg/mcp/client`, then
`pkg/auth`) was the wrong layering — the namespace is a global Atmos
identity concept. A grep also found **five** independent hardcoded copies
of `"ATMOS"` / `"ATMOS_"` across the codebase with no shared definition.

The canonical constants now live in `pkg/config/const.go` next to the
other foundational Atmos identity constants:

```go
AtmosEnvVarNamespace = "ATMOS"                       // viper.SetEnvPrefix
AtmosEnvVarPrefix    = AtmosEnvVarNamespace + "_"    // HasPrefix checks
```

The prefix is **derived** from the namespace so they cannot drift; a
build-time test (`TestAtmosEnvVarPrefixMatchesNamespace`) enforces the
invariant. A second test (`TestAtmosEnvVarConstants_Values`) locks in
the literal values because the Atmos env namespace is part of the public
contract — silently changing it would break every user with `ATMOS_*`
env vars set.

All five known call sites were migrated to use the canonical constants:
`cmd/root.go`, `cmd/auth_validate.go`, `pkg/auth/manager_env_overrides.go`
(deleted its local `AtmosEnvPrefix`), `pkg/ai/agent/codexcli/client.go`,
and `pkg/ai/agent/codexcli/client_test.go`.

### Fixed Flow

```text
parent atmos                                 subprocess
─────────────                                ───────────
1. lazy: per-server provider constructed
2. session.Start(opts...)
   └─ WithAuthManager (via PerServerAuthProvider)
      ├─ ApplyAtmosEnvOverrides({ATMOS_PROFILE: managers})
      ├─ cfg.InitCliConfig()         ← sees managers profile
      ├─ CreateAndAuthenticateManager()
      │   → identities for managers profile loaded
      │   → "core-root/terraform" found ✓
      ├─ restore()                    ← parent env reverts
      └─ PrepareShellEnvironment(...) ← uses constructed manager
3. cmd.Env = env (server env block also passed to subprocess)
4. atmos mcp start (subprocess) reads ATMOS_PROFILE from its own env ✓
```

## Files Changed

### `pkg/config/` — canonical Atmos env-var namespace constants

- `pkg/config/const.go` — added `AtmosEnvVarNamespace = "ATMOS"` and
  `AtmosEnvVarPrefix = AtmosEnvVarNamespace + "_"`. Single source of truth
  for "what counts as an Atmos environment variable" anywhere in the
  codebase. This is the namespace passed to `viper.SetEnvPrefix(...)`.
- `pkg/config/const_test.go` *(new)* — build-time invariants:
  - `AtmosEnvVarPrefix == AtmosEnvVarNamespace + "_"`
  - Both literal values are locked in to catch accidental renames (the Atmos
    env namespace is part of the public contract — changing it breaks every
    user with `ATMOS_*` env vars).

### `pkg/env/` — foundational env primitive (reusable everywhere)

- `pkg/env/restore.go` *(new)* — `env.SetWithRestore` save/set/restore primitive.

### `pkg/auth/` — high-level auth-specific primitive

- `pkg/auth/manager_env_overrides.go` *(new)* — the
  `CreateAndAuthenticateManagerWithEnvOverrides` high-level primitive plus
  the small `filterAtmosOverrides` helper. Uses `cfg.AtmosEnvVarPrefix`.

### Migrations to the canonical constants (no behavior change)

These call sites previously hardcoded `"ATMOS"` / `"ATMOS_"` literals:

- `cmd/root.go` — `viper.SetEnvPrefix(cfg.AtmosEnvVarNamespace)`
- `cmd/auth_validate.go` — `viper.SetEnvPrefix(cfg.AtmosEnvVarNamespace)`
- `pkg/ai/agent/codexcli/client.go` — `strings.HasPrefix(env, cfg.AtmosEnvVarPrefix)`
- `pkg/ai/agent/codexcli/client_test.go` — same.

### `pkg/mcp/client/` — interface and thin MCP adapter

- `pkg/mcp/client/session.go` — `PerServerAuthProvider` interface and
  `WithAuthManager` per-server dispatch path.
- `pkg/mcp/client/scoped_auth.go` *(new)* — `ScopedAuthProvider`: thin
  ~70-line adapter over the `pkg/auth` primitive. Implements both
  `AuthEnvProvider` and `PerServerAuthProvider`.

### `cmd/` — no auth-factory code

- `cmd/mcp/client/start_options.go` — `buildAuthOption` now calls
  `mcpclient.NewScopedAuthProvider`; dead `auth`/`cfg`/`fmt`/`ui` imports
  removed; helper `mcpServersNeedAuth` extracted.
- `cmd/ai/init.go` — `resolveAuthProvider` now calls
  `mcpclient.NewScopedAuthProvider`; the previously-duplicated
  `perServerAuthProvider` struct was removed.

## Tests

### `pkg/config/const_test.go` (new)

- `TestAtmosEnvVarPrefixMatchesNamespace` — build-time invariant: prefix
  must equal namespace + `"_"`. A future drift fails the build.
- `TestAtmosEnvVarConstants_Values` — locks in `"ATMOS"` and `"ATMOS_"`
  literals so an accidental rename trips a test instead of breaking
  every user's `ATMOS_*` env vars at runtime.

### `pkg/env/restore_test.go` (new)

- `TestSetWithRestore_SetsAndRestores_NoPreviousValue` — keys are applied;
  restore unsets keys that had no prior value.
- `TestSetWithRestore_RestoresPreviousValue` — restore reverts to the original
  value when one existed.
- `TestSetWithRestore_MixedPreviousState` — handles mixed set/unset prior state.
- `TestSetWithRestore_EmptyAndNilMap_NoOp`.
- `TestSetWithRestore_RestoreIsIdempotent` — calling cleanup twice is a no-op
  after the first call.
- `TestSetWithRestore_EmptyValue_IsSet` — `key=""` is distinct from "unset".
- `TestSetWithRestore_SetenvError_ReturnsCleanup` — verifies `(cleanup, err)`
  contract when the underlying setenv fails (uses an injectable `setenvFn`
  hook).
- `TestSetWithRestore_SetenvError_RestoresPreviouslySetVarsOnDefer` — even
  on partial failure, the cleanup closure correctly reverts variables that
  were already set.

### `pkg/auth/manager_env_overrides_test.go` (new)

- `TestCreateAndAuthenticateManagerWithEnvOverrides_AppliesEnvBeforeInit` —
  verifies that `initCliConfigFn` runs with the override in `os.Environ`.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_RestoresPreviousProfile` —
  parent `ATMOS_PROFILE` is restored after the primitive returns.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_NilOverrides` /
  `TestCreateAndAuthenticateManagerWithEnvOverrides_EmptyOverrides` — no-op.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_FiltersNonAtmosKeys` —
  non-`ATMOS_*` keys in the input map must NOT mutate the parent env.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_OnlyNonAtmosKeys_NoMutation` —
  filter producing an empty result is a clean no-op.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_InitConfigError` — error
  path; env still restored.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_NilManagerContract` —
  `(nil, nil)` propagates up untouched; callers decide how to react.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_SetEnvError_ReturnsAndCleansUp` —
  when `env.SetWithRestore` returns an error, the wrapper invokes the cleanup
  closure exactly once and returns the wrapped error.
- `TestCreateAndAuthenticateManagerWithEnvOverrides_SetEnvError_NilCleanup_NoPanic` —
  defensive guard for `(nil, err)` returns from the env hook.
- `TestFilterAtmosOverrides` — table-driven (nil/empty/all-atmos/no-atmos/mixed/empty-value).

### `pkg/mcp/client/auth_test.go` (extended)

- `TestWithAuthManager_PerServerProvider_UsesScopedProvider` — when the auth
  provider implements `PerServerAuthProvider`, `ForServer` is called and the
  scoped provider (not the root) handles `PrepareShellEnvironment`.
- `TestWithAuthManager_PerServerProvider_ForServerError` — error from `ForServer`
  is wrapped and returned; scoped provider is never called.
- `TestWithAuthManager_PerServerProvider_NilScoped_ReturnsAuthUnavailable` —
  defensive `nil` return surfaces `ErrMCPServerAuthUnavailable`.
- `TestWithAuthManager_PerServerProvider_NoIdentity_SkipsForServer` — pass-through
  for servers without `identity:`.

### `pkg/mcp/client/scoped_auth_test.go` (new)

Only the adapter behavior is covered here — env-override, config-reload, and
manager-construction semantics are tested in `pkg/auth` and not duplicated.

- `TestScopedAuthProvider_ForServer_PlumbsServerEnvToBuilder` — the adapter
  passes `ParsedConfig.Env` to the builder unchanged.
- `TestScopedAuthProvider_ForServer_NilManager_ReturnsSentinel` —
  `(nil, nil)` surfaces `ErrMCPServerAuthUnavailable` with server+identity
  context.
- `TestScopedAuthProvider_ForServer_BuilderError_Wrapped` — errors from the
  builder are wrapped with the server name and remain `errors.Is`-matchable.
- `TestScopedAuthProvider_PrepareShellEnvironment_FallbackPath` — the
  `AuthEnvProvider` interface is satisfied for callers that bypass `ForServer`;
  fallback path calls the builder with nil overrides.
- `TestScopedAuthProvider_PrepareShellEnvironment_NilManager_ReturnsSentinel` —
  fallback path guards the nil-pointer dereference and returns the sentinel.
- `TestScopedAuthProvider_PrepareShellEnvironment_BuilderError` — error
  propagation from the fallback path.
- `TestScopedAuthProvider_ImplementsBothInterfaces` — compile-time check.
- `TestNewScopedAuthProvider_DefaultBuilderWiredToAuthPackage` — smoke test
  that the default constructor wires `buildManagerFn` to the pkg/auth
  primitive.

### `cmd/mcp/client/start_options_auth_test.go` (new)

- `TestMcpServersNeedAuth` — table-driven.
- `TestBuildAuthOption_NoServersNeedingAuth`.
- `TestBuildAuthOption_ReturnsScopedProvider` — also verifies the shared
  provider satisfies both `AuthEnvProvider` and `PerServerAuthProvider`.

### `cmd/ai/init_resolve_auth_test.go` (new)

- `TestResolveAuthProvider_NoIdentity_ReturnsNil`.
- `TestResolveAuthProvider_WithIdentity_ReturnsScopedProvider` — verifies the
  returned provider implements `PerServerAuthProvider`.

### Coverage

All new code is at **100%** coverage:

- `pkg/env/restore.go` → `SetWithRestore` **100%** (including the
  setenv-error branch via the `setenvFn` test hook).
- `pkg/auth/manager_env_overrides.go` →
  `CreateAndAuthenticateManagerWithEnvOverrides` **100%** (including the
  env-hook error branch via the `setEnvWithRestoreFn` test hook);
  `filterAtmosOverrides` **100%**.
- `pkg/mcp/client/scoped_auth.go` → `NewScopedAuthProvider`, `ForServer`,
  `PrepareShellEnvironment` all **100%**.
- `pkg/mcp/client/session.go` → `WithAuthManager` (including the per-server
  dispatch branch) **100%**; `prepareEnv` **100%**.
- `cmd/mcp/client/start_options.go` → `buildAuthOption` and
  `mcpServersNeedAuth` both **100%**.

## Follow-up: consolidate existing save/set/restore env helpers

During this fix we discovered Atmos already has **four** local
implementations of "set env vars with save-restore", each in a different
package, none of which share code:

| Location | Function | Notes |
|---|---|---|
| `internal/exec/template_utils.go` | `setEnvVarsWithRestore` | Generic, returns `(func(), error)` — semantically identical to the new `env.SetWithRestore`. Doc: `docs/fixes/2026-02-16-gomplate-datasource-env-vars.md`. |
| `pkg/auth/cloud/gcp/env.go` | `PreserveEnvironment` + `RestoreEnvironment` | GCP-specific Preserve/Restore pair. |
| `pkg/telemetry/ci.go` | `PreserveCIEnvVars` + `RestoreCIEnvVars` | CI-detection specific. |
| `pkg/auth/identities/aws/credentials_loader.go` | `setupAWSEnv` | AWS-specific, returns `func()`. |

All four are slight specializations of the same idea. None of them used a
shared primitive because none existed. Now that `env.SetWithRestore` exists,
each of these can be migrated to call it (or composed via small helpers).

That migration is **out of scope for this PR** because it touches four
unrelated subsystems and would balloon the diff. A follow-up issue should
track:

1. Migrate `internal/exec.setEnvVarsWithRestore` → `env.SetWithRestore` (smallest, lowest risk).
2. Migrate `pkg/telemetry` Preserve/Restore CI vars → `env.SetWithRestore`.
3. Migrate `pkg/auth/identities/aws.setupAWSEnv` → `env.SetWithRestore`.
4. Migrate `pkg/auth/cloud/gcp` Preserve/Restore → `env.SetWithRestore`.

The new primitive is intentionally generic (no key-prefix filter,
caller-supplied map) so all four migrations are possible without further
changes to `pkg/env`.

## Notes

- Tests use injectable `initConfig` and `createAuthManager` fields on the
  per-server providers to avoid touching the real Atmos config loader.
  No `os.Chdir`, no fixture stacks needed.
- `ApplyAtmosEnvOverrides` mutates `os.Environ()` and is intentionally
  serial — the current MCP startup loop in `pkg/mcp/client/register.go`
  starts servers sequentially. Adding parallelism would require either
  process isolation or a config-loader that accepts an explicit env map.
- The fix also benefits `ATMOS_CLI_CONFIG_PATH`, `ATMOS_BASE_PATH`,
  `ATMOS_LOGS_LEVEL`, and any other `ATMOS_*` variable a user puts in the
  server's `env:` block.

## Related

- `docs/fixes/2026-03-25-describe-affected-auth-identity-not-used.md` —
  another auth-context propagation fix in a different code path.
- `docs/prd/atmos-mcp-integrations.md` — overall MCP client architecture.
