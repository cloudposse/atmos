# Native CI Integration - Build Cache

> Related: [Artifact Storage](./artifact-storage.md) | [Configuration](./configuration.md) | [Interfaces](./interfaces.md) | [Implementation Status](./implementation-status.md)

## Executive Summary

Atmos installs a toolchain (via `atmos toolchain`) and downloads other regenerable
artifacts into XDG cache/data directories. In CI, those artifacts are re-fetched on
every run, wasting time and bandwidth. The **CI build cache** lets Atmos restore a
well-known cache directory at startup and save it at exit, using the *same store that
`actions/cache` uses* when running inside a CI provider. A single configured path (the
toolchain and anything else under the cache root) is cached as one archive.

The full lifecycle can run **in a single Atmos invocation** (auto restore-on-start +
auto save-on-end) or be **spread across steps** (`atmos ci cache restore` in one step,
do work, `atmos ci cache save` in another) via explicit CRUD commands. Both styles share
one implementation; "automatic" is just the same idempotent operations invoked by the
process lifecycle.

This is a CI-provider capability, layered onto the existing CI provider abstraction
(`pkg/ci`) exactly like the artifact subsystem. GitHub Actions is the first (and, for
now, only) implementation; a generic/S3 backend for non-GitHub providers is future work.

## Problem Statement

### Current State

- The toolchain re-downloads tools on every CI job. There is no first-class mechanism to
  persist `~/.cache/atmos` across jobs.
- Teams work around this by hand-wiring `actions/cache` steps in their workflows, which
  duplicates key/path logic Atmos already understands (e.g. the toolchain lockfile).

### Desired State

- Atmos natively restores and saves its cache directory through the active CI provider.
- The toolchain lives under the well-known cache root, so a single cache captures it.
- Users either turn on automatic behavior (`ci.cache.auto`) or drive the cache explicitly
  with `atmos ci cache {restore,save,list,delete}` — with no double-work between the two.

## Hard Constraint (drives the design)

The real Actions cache store is only reachable from inside a runner via the **Cache
Service v2**: Twirp over `ACTIONS_RESULTS_URL`, content uploaded/downloaded through Azure
Blob SAS URLs, authenticated with `ACTIONS_RUNTIME_TOKEN`. There is **no PAT path** for
save/restore. Therefore all cache operations are provider-determined and become no-ops
(with a debug log) when no cache-capable CI provider is detected — i.e. outside CI.

## What This Enables

- Fast toolchain warm-starts in CI: install once, reuse across jobs and workflow runs.
- A general path-cache that other regenerable Atmos data under the cache root inherits for
  free (vendoring caches, remote stack-import clones, plugin caches).
- A seam for a future generic CI cache (e.g. S3) usable outside GitHub Actions.

## Functional Requirements

**Capability model**: The cache is an optional capability on the CI `Provider` (like
`DebugModeDetector`). `pkg/ci.DetectCache()` returns the active provider's
`cache.Backend`, or `ErrCacheUnavailable` when no cache-capable provider is detected.

**Backend operations (CRUD)**: `Save` (write-once), `Restore` (exact key, then
restore-key prefix fallback), `List` (optionally filtered by key prefix), `Delete` (by
exact key; missing key is a no-op).

**Write-once semantics**: Cache entries are immutable. Saving an existing key returns
`ErrCacheAlreadyExists`, which the orchestration treats as success.

**Key derivation**: Keys support Go templates with `{{.OS}}`, `{{.Arch}}` and a
`hashFiles` function. The default key is derived from a SHA-256 of `toolchain.lock.yaml`
plus OS/arch; the default restore-key is the same prefix without the hash, mirroring
`actions/cache`.

**Well-known cache root**: Defaults to the Atmos XDG cache directory (`~/.cache/atmos`),
overridable via `ci.cache.root` and `ATMOS_XDG_CACHE_HOME`/`XDG_CACHE_HOME`. The toolchain
install path defaults to a sub-path of this root (`<root>/toolchain`).

**Archive format**: A single `tar.gz` of the cache root (or configured relative subpaths).
Extraction rejects entries that escape the root; symlinks/special files are skipped.

**Lifecycle**: With `ci.cache.auto`, Atmos restores in `PersistentPreRun` and saves in
`Cleanup()` (which runs on normal exit and on SIGINT/SIGTERM). The explicit subcommands
are always available regardless of `auto`.

**Enablement**: Gated by `ci.cache.enabled` (env `ATMOS_CI_CACHE_ENABLED`). Off by
default. The CRUD subcommands also require the cache to be enabled.

## Key Design Decisions

**Extend the CI abstraction, don't fork it.** The cache mirrors the artifact subsystem:
a `cache.Backend` interface + registry (`pkg/ci/cache`), a provider capability interface
(`provider.CacheProvider`), and a GitHub implementation (`pkg/ci/cache/github`). The
GitHub *provider* (`pkg/ci/providers/github`) implements `Cache()` by constructing the
GitHub backend.

**Auto ⇆ manual reconciliation via a single source of truth + state marker.** "Automatic"
is not separate code — it is the same `Manager.Restore`/`Manager.Save` operations invoked
by the lifecycle. A per-root state marker (`<root>/.atmos-cache/state.json`, excluded from
the archive) records, per key, how it was restored (`exact`/`prefix`/`miss`) and whether
it was saved. This makes both paths idempotent:

- **Restore** is a no-op once a key has been restored in the lifecycle.
- **Save** is skipped when the exact key was a hit at restore time (content unchanged) or
  was already saved — exactly `actions/cache`'s `cache-hit ⇒ skip save` behavior.

Consequently `auto: both` plus a manual `atmos ci cache save` cannot double-upload, and
`auto: off` gives fully manual control across steps. Both styles coexist with no
special-casing. The marker persists on disk so the lifecycle works across separate
invocations as well as within one.

**Constant content version.** GitHub's Cache Service v2 salts entries with a `version`.
A single deterministic version (SHA-256 of a namespace constant) is used for all Atmos
caches so restore-key prefix matching works across entries. A format change bumps the
namespace to invalidate old caches without key collisions.

**Provider-determined availability — split by operation.** Saving and restoring cache
*content* go through the runtime cache API (Twirp) and require `ACTIONS_RUNTIME_TOKEN` +
`ACTIONS_RESULTS_URL`, which exist only inside a runner; outside a runner `save`/`restore`
and the automatic lifecycle hooks degrade to clear `ErrCacheUnavailable` no-ops. Cache
*administration* (`list`/`delete`) uses the public REST caches API with an ordinary GitHub
token and so works from a workstation too: `NewBackend` builds an admin-capable backend
whenever a token + repository (from `GITHUB_REPOSITORY` or the local `git` remote) are
resolvable, and the runtime client is built lazily — only `Save`/`Restore` require it.
`ci.ResolveAdminCache` resolves the cache-capable provider for admin commands without
requiring the provider to be the actively-detected one (the github provider registers
unconditionally; `Detect()` still gates the in-runner lifecycle on `GITHUB_ACTIONS`).

**Reuse over duplication.** Archive uses `compress/gzip` + `archive/tar`; blob transfer
mirrors the artifact `runtimeUploader` (single-PUT BlockBlob); REST list/delete use the
existing `pkg/github` token resolution; hashing reuses the toolchain checksum approach.

## Configuration

```yaml
ci:
  cache:
    enabled: false          # master switch (env: ATMOS_CI_CACHE_ENABLED)
    auto: off               # off | restore | save | both (env: ATMOS_CI_CACHE_AUTO)
    root: ""                # override the well-known cache root (default ~/.cache/atmos)
    paths: []               # root-relative subpaths to cache (default: the whole root,
                            # minus the default-excluded auth paths below)
    key: ""                 # template; default derived from toolchain.lock.yaml + os/arch
    restore_keys: []        # prefix fallbacks
    compression: gzip       # gzip (default)
```

CLI overrides on the subcommands: `--key`, `--restore-key`, `--path`, `--root`,
`--format` (list). Env precedence follows the standard flag handling
(flag > `ATMOS_CI_CACHE_*` env > config > default).

## Commands

```text
atmos ci cache restore   # restore the cache into the well-known root
atmos ci cache save      # archive the root and upload under the key
atmos ci cache list      # list cache entries (uses pkg/list rendering)
atmos ci cache delete    # delete a cache entry by key
```

## Implementation

| Area | Location |
| --- | --- |
| Backend interface + registry | `pkg/ci/cache/backend.go`, `registry.go` |
| Orchestration + lifecycle helpers | `pkg/ci/cache/manager.go`, `state.go`, `config.go`, `key.go`, `archive.go` |
| GitHub backend (Cache Service v2 + REST) | `pkg/ci/cache/github/backend.go` |
| Provider capability | `pkg/ci/internal/provider/types.go` (`CacheProvider`), `pkg/ci/registry_provider.go` (`DetectCache`) |
| GitHub provider wiring | `pkg/ci/providers/github/cache.go` |
| CLI commands | `cmd/ci/cache/*.go` (mounted under `cmd/ci/ci.go`) |
| Lifecycle hooks | `cmd/ci/cache/lifecycle.go`, call sites in `cmd/root.go` (PreRun restore, `Cleanup()` save) |
| Schema | `pkg/schema/schema.go` (`CICacheConfig`), env binding in `pkg/config/load.go` |
| Toolchain consolidation | `pkg/toolchain/setup.go` (`GetInstallPath` → XDG cache sub-path) |
| Errors | `errors/errors.go` (`ErrCache*`) |

## Default-Excluded Auth Paths

Caching the whole XDG cache root by default is intentional (§ What This Enables) — it lets
vendoring caches, remote stack-import clones, and plugin caches inherit CI caching for free
without every new regenerable-data subdirectory needing a config change. But that same root
is where several Atmos auth flows persist session credentials to disk, so a whole-root default
needs an unconditional carve-out rather than relying on operators to hand-configure
`ci.cache.paths` correctly. This is defensive hardening, not a fix for an incident or
demonstrated exploit — a repo's Actions cache is already scoped to that repo, and the
carve-out keeps auth material out of the archive rather than relying on convention.

`pkg/ci/cache.DefaultExcludedPaths()` (`pkg/ci/cache/exclusions.go`) is pruned unconditionally
in `archive.go`'s `archiveSkipDecision` — regardless of `ci.cache.paths`, with no opt-out. The
same list is rendered as `!`-prefixed glob exclusions by `atmos ci cache paths` for the
native-`actions/cache` integration path.

| Root-relative path | Owning file | Contains |
| --- | --- | --- |
| `aws-sso` | `pkg/auth/providers/aws/sso_cache.go` (`ssoTokenCacheSubdir`) | AWS SSO access token, refresh token, OAuth client secret |
| `azure-device-code` | `pkg/auth/providers/azure/device_code_cache.go` (`deviceCodeTokenCacheSubdir`) | Azure access token, Graph API token |
| `aws-webflow` | `pkg/auth/identities/aws/webflow.go` (`webflowCacheSubdir`) | Refresh token for Atmos's browser-based AWS identity flow |
| `auth` | `pkg/auth/provisioning/writer.go` (`authSubDir`) | Provisioned identity/role metadata (not raw secrets; excluded defensively anyway) |

Each owning package carries a drift-guard test (e.g.
`TestSSOTokenCacheSubdir_MatchesCICacheDefaultExcludes`) asserting its subdir constant is
present in `DefaultExcludedPaths()`, so a rename there without updating the exclusion list
fails a test immediately rather than silently reopening the gap.

Deliberately **not** on this list, verified directly rather than assumed:

- **OIDC-based auth** (`pkg/auth/providers/github/oidc.go`, `gcp_wif/provider.go`,
  `azure/oidc.go`) writes nothing to any cache directory — credentials are minted fresh
  in-memory from the CI-native OIDC token every run.
- **`saml2aws`** browser storage lives at `~/.aws/saml2aws/` — outside the Atmos XDG cache
  root entirely.
- **GCP ADC/config** (`pkg/auth/cloud/gcp/files.go`) resolves via the XDG **config** dir
  (`~/.config/atmos`), a different XDG base than `ci.cache`'s root.
- **`auth/github-sts`** (`pkg/auth/integrations/github/sts.go`, `stsDataSubdir`) resolves via
  the XDG **data** dir, not the XDG cache dir — already outside `ci.cache`'s scan root today,
  though incidentally so, not as a designed boundary. If a future refactor moves it under XDG
  cache, it will need to be added to `DefaultExcludedPaths()`.

## Out of Scope (future work)

- Generic/S3 CI cache backend for non-GitHub providers (`ci.cache` outside Actions).
- TTL/eviction surfaced via config once basic CRUD has shipped.
- Multi-archive (per-`paths` entry) caching and content-defined chunking for very large
  caches.

## Open Questions

- Should auto-save be further gated on "cache was actually dirtied" (e.g. a toolchain
  install occurred) rather than relying solely on the exact-hit skip? The current
  write-once + state-marker behavior already prevents redundant uploads, so this is a
  potential optimization, not a correctness gap.
