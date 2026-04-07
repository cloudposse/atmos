# Fix: MCP server `env:` block not applied during auth setup (ATMOS_PROFILE / ATMOS_CLI_CONFIG_PATH ignored)

**Date:** 2026-04-06
**Issue:** [#2283](https://github.com/cloudposse/atmos/issues/2283)

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

### 2. New `ApplyAtmosEnvOverrides` helper

`pkg/mcp/client/env_overrides.go`:

```go
// ApplyAtmosEnvOverrides applies ATMOS_* environment variables from the given
// map to the parent process environment, returning a restore function.
func ApplyAtmosEnvOverrides(env map[string]string) func() { ... }
```

- Only `ATMOS_*`-prefixed keys are touched (other keys are ignored to avoid
  surprising parent-process mutations).
- Returns a restore closure that reverts each variable to its previous state
  (set to original value, or unset if it didn't exist).
- Idempotent — calling `restore()` twice is safe.
- Sequential by design: MCP servers are started one at a time in current
  Atmos code paths, so we accept process-global mutation as the simplest
  solution. Concurrency would require a different mechanism.

### 3. Per-server providers

Two `perServerAuthProvider` types now wrap the parent's `auth.AuthManager`
construction with the server's env block:

- **`cmd/mcp/client/auth_factory.go`** — `PerServerAuthManager` for the
  management commands (`atmos mcp test/tools/status/restart`).
- **`cmd/ai/init.go`** — `perServerAuthProvider` for the AI commands
  (`atmos ai ask/chat/exec`).

Both implement `mcpclient.AuthEnvProvider` *and* `mcpclient.PerServerAuthProvider`,
sharing the same logic:

```go
func (p *perServerAuthProvider) ForServer(_ context.Context, c *mcpclient.ParsedConfig) (mcpclient.AuthEnvProvider, error) {
    return p.buildManager(c.Env)
}

func (p *perServerAuthProvider) buildManager(serverEnv map[string]string) (auth.AuthManager, error) {
    restore := mcpclient.ApplyAtmosEnvOverrides(serverEnv)
    defer restore()

    loadedConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
    if err != nil { return nil, err }

    return auth.CreateAndAuthenticateManagerWithAtmosConfig(
        "", &loadedConfig.Auth, cfg.IdentityFlagSelectValue, &loadedConfig,
    )
}
```

The `defer restore()` runs **before** `WithAuthManager` invokes
`PrepareShellEnvironment`, but that is fine: the auth manager's identity map
and credentials are already populated during construction.

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

- `pkg/mcp/client/session.go` — `PerServerAuthProvider` interface and
  `WithAuthManager` per-server path.
- `pkg/mcp/client/env_overrides.go` — `ApplyAtmosEnvOverrides` helper (new file).
- `cmd/mcp/client/auth_factory.go` — `PerServerAuthManager` (new file).
- `cmd/mcp/client/start_options.go` — `buildAuthOption` rewritten to use the
  per-server provider; dead `auth`/`cfg`/`fmt`/`ui` imports removed; helper
  `mcpServersNeedAuth` extracted.
- `cmd/ai/init.go` — `resolveAuthProvider` rewritten to use the per-server
  provider; new `perServerAuthProvider` struct.

## Tests

### `pkg/mcp/client/env_overrides_test.go` (new)

- `TestApplyAtmosEnvOverrides_AppliesAndRestores` — `ATMOS_*` keys are applied;
  non-ATMOS keys are ignored; restore unsets keys that had no prior value.
- `TestApplyAtmosEnvOverrides_RestoresPreviousValues` — restore reverts to the
  original value when one existed.
- `TestApplyAtmosEnvOverrides_EmptyMap_NoOp` / `TestApplyAtmosEnvOverrides_NoAtmosKeys_NoOp`.
- `TestApplyAtmosEnvOverrides_RestoreIsIdempotent` — calling `restore()` twice
  must not re-mutate state.
- `TestApplyAtmosEnvOverrides_AppliesAllAtmosPrefixedKeys` —
  `ATMOS_PROFILE`, `ATMOS_CLI_CONFIG_PATH`, `ATMOS_BASE_PATH`, `ATMOS_LOGS_LEVEL`.

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

### `cmd/mcp/client/auth_factory_test.go` (new)

- `TestPerServerAuthManager_ForServer_AppliesAtmosEnvBeforeInit` — verifies
  that `InitCliConfig` runs with the server's `ATMOS_PROFILE` in `os.Environ`,
  and that the env is restored after `ForServer` returns.
- `TestPerServerAuthManager_ForServer_RestoresPreviousProfile` — when the
  parent already had `ATMOS_PROFILE=outer`, the build sees `managers` and
  the parent is restored to `outer`.
- `TestPerServerAuthManager_ForServer_NoEnvOverride` — `nil` env is a no-op.
- `TestPerServerAuthManager_ForServer_InitConfigError` — error path; env still
  restored.
- `TestPerServerAuthManager_PrepareShellEnvironment_FallbackPath` — the
  `AuthEnvProvider` interface is satisfied for callers that bypass `ForServer`.
- `TestPerServerAuthManager_ImplementsBothInterfaces` — compile-time check.
- `TestMcpServersNeedAuth` — table-driven for the helper.
- `TestBuildAuthOption_NoServersNeedingAuth` /
  `TestBuildAuthOption_ReturnsOption`.

### `cmd/ai/init_perserver_auth_test.go` (new)

Same shape as the `cmd/mcp/client` tests, but for the AI command path:

- `TestPerServerAuthProvider_ForServer_AppliesAtmosEnv`
- `TestPerServerAuthProvider_ForServer_InitError`
- `TestPerServerAuthProvider_PrepareShellEnvironment`
- `TestPerServerAuthProvider_ImplementsBothInterfaces`
- `TestResolveAuthProvider_NoIdentity_ReturnsNil`
- `TestResolveAuthProvider_WithIdentity_ReturnsPerServerProvider`

### Coverage

After this change:

- `pkg/mcp/client/env_overrides.go` — **100%** (`ApplyAtmosEnvOverrides`).
- `pkg/mcp/client/session.go` — `WithAuthManager` is now **100%** including the
  per-server branch.
- `cmd/mcp/client/auth_factory.go` — `NewPerServerAuthManager`, `ForServer`,
  `buildManager` all **100%**; `PrepareShellEnvironment` fallback **80%**.
- `cmd/mcp/client/start_options.go` — `buildAuthOption` and `mcpServersNeedAuth`
  both **100%**.

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
