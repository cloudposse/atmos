# Terraform Registry Cache

> Related: [Terraform RC Management](terraform-rc-management.md) (prerequisite) · [Native CI — Artifact Storage](native-ci/framework/artifact-storage.md) · [Native CI — Unify Artifact Stores](native-ci/phases/unify-artifact-stores.md) · atmos-bundles PRD

## Overview

Atmos repeatedly downloads the same Terraform/OpenTofu **providers** and **modules** across runs, components,
stacks, and CI jobs. This wastes bandwidth and time, makes runs fragile when upstream registries are slow or
unavailable, breaks in air-gapped environments, and offers no organizational control over which sources are
trusted.

To be fair, Atmos **already** addresses part of this for providers: it sets the Terraform plugin cache
(`TF_PLUGIN_CACHE_DIR`) today, which deduplicates provider *plugin* downloads across working directories. But
the plugin cache does nothing for **modules**, and module downloads — especially pulling modules from large
mono-repos — are a significant and growing cost. The plugin cache also does nothing for reproducibility,
offline operation, cross-machine/CI sharing, or supply-chain control.

This PRD introduces a transparent **registry cache** for Terraform/OpenTofu that goes beyond the plugin cache.
The user enables it with a single flag; Atmos then runs an ephemeral local registry proxy, generates the
Terraform CLI configuration to route through it (via [Terraform RC Management](terraform-rc-management.md)),
and caches registry content on disk. It covers **both providers (via the Provider Network Mirror Protocol) and
modules (via the module registry protocol)**. No Terraform code, provider declarations, or module sources
change.

For **registry-sourced modules** (`source = "cloudposse/label/null"`), the module mirror below caches the
download *whatever it resolves to* — including the common `git::` case. It rewrites `X-Terraform-Get` back
through the proxy, resolves the source with the same go-getter Terraform uses, and caches it as a single tar
artifact. The one thing explicitly **out of scope right now (but on our radar)** is a **git mirror** for
modules sourced **directly** from git (`source = "git::https://…"` or `source = "github.com/org/repo"`):
those bypass the registry protocol entirely, so the proxy never sees them. Caching that content reuses the
same `GIT_CONFIG_*` `insteadOf` injection Atmos already has, and is the future git mirror.

The design is intentionally broader than Terraform: a **generic caching proxy** is the core, with thin
**registry-mirror adapters** on top. Terraform registry caching is simply the first consumer of a generic
caching-proxy + artifact-store foundation that can later serve git mirrors, OCI artifacts, and CI caches.

### The deeper idea: dependencies are artifacts Atmos owns

The strongest theme here is not "caching." It is treating **infrastructure dependencies as artifacts** —
versioned, cached, and reproducible the way mature software projects treat their dependencies. Users own
their infrastructure and its dependencies; Atmos simply **facilitates capturing, storing, and reproducing
them**, the same way it already treats CI artifacts and (soon) bundles. The registry cache is the first
manifestation of that idea, and it is why the cache, RC
management, Native CI artifact storage, bundles, and git mirrors all share one artifact-storage + caching
substrate rather than each inventing its own. The immediate, concrete payoffs follow directly from that
framing: repeated and CI runs stop re-downloading the same providers and modules (less time, less
bandwidth); runs keep working through upstream outages and when artifacts disappear; the exact providers
and modules a deployment used are preserved so it can be replayed years later; and a warm cache is the
on-disk closure that an air-gapped "atmos bundle" is built from. One feature flag turns this on with no
changes to Terraform code, provider declarations, or module sources.

## Architectural Principles

1. **The cache key is authoritative; the backend implementing that keyspace is the source of truth — no
   separate database or index.** What is truly canonical is the cache key
   (`<host>/<namespace>/<type>/<version>/<platform>`), not any one storage medium. The backend that implements
   that keyspace *is* the authoritative store — presence of a key determines cache state — so there is no
   secondary index that can drift, corrupt, or require migrations. **In V1 the backend is the filesystem**
   (the path *is* the key); in a later version the backend may be object storage, with the local filesystem
   acting as a read-through cache. This mirrors the existing artifact-store abstraction where `local`, `s3`,
   and `github` are interchangeable implementations of the same backend interface. A future SQLite database may
   serve *only* metrics/reporting/eviction statistics — never correctness.
2. **A generic caching proxy is the core; registry mirrors are adapters.** The proxy knows nothing about
   Terraform; protocol-specific mirrors plug into it.
3. **Reuse existing Atmos infrastructure.** Storage backends, locking, atomic writes, XDG resolution, and the
   ephemeral-HTTP-server pattern already exist in the codebase and are reused rather than re-implemented.
4. **Native CLI-config passthrough.** The cache configures Terraform by injecting standard directives into the
   generated CLI config — it invents no new Terraform abstraction.

## The Four Pieces

```text
Terraform/OpenTofu
      │  (TF_CLI_CONFIG_FILE → provider_installation { network_mirror { url } })
      ▼
[1] Ephemeral caching proxy (127.0.0.1:<dynamic>)   ← pkg/http/proxy
      │   request → cache key → file lock → hit? serve : miss
      ▼
[2/3] Registry-mirror adapter (provider | module)   ← pkg/terraform/registry
      │   maps the protocol request to a cache key + upstream resolver
      ▼
[4] Cache backend (local | s3 | github)             ← pkg/ci/artifact
      │   miss → fetch upstream → verify (SHA-256) → atomic store → serve
      ▼
Upstream registry (registry.terraform.io / registry.opentofu.org)
```

1. **Caching proxy** — a generic, protocol-agnostic ephemeral HTTP proxy. For each request it computes a cache
   key, acquires a lock, and either serves a hit or fetches upstream, verifies, atomically stores, and serves.
   Dynamic localhost port, no persistent daemon; the cache persists on disk after the process exits.
2. **Provider registry mirror** — an adapter implementing the *Provider Network Mirror Protocol*. **V1.**
3. **Module registry mirror** — an adapter implementing Terraform's *module registry protocol*, redirected via
   a `host` service-discovery block in the generated CLI config. **V1** caches the registry protocol and the
   resolved module source for every download (`git::` or HTTP archive) as a tar artifact; only modules sourced
   directly from git (not via the registry) await the git mirror (see Non-Goals).
4. **Cache implementation** — pluggable backend adapters (filesystem in V1; object storage later) plus a lock
   manager, both reusing existing Atmos infrastructure. The backend implements the authoritative keyspace; the
   filesystem is V1's implementation of it.

## Goals

- **Performance** — reduce repeated provider downloads across runs by **>90%**, and cut module registry
  round-trips and module source downloads (registry-sourced modules, `git::` or HTTP archive; modules sourced
  directly from git follow with the git mirror).
- **Reliability** — Terraform runs succeed during upstream outages when artifacts are already cached.
- **Reproducibility** — previously resolved versions remain available even if upstream content disappears.
- **Supply-chain control** — restrict and audit approved provider sources (via native CLI config; see
  Governance).
- **Offline / air-gapped operation** — a warm cache can run disconnected and can be packaged for transport.
- **Visible value** — print a one-line **savings report** before exit when bytes were served from cache.

## Non-Goals (V1)

- **A git mirror for directly-declared git module sources** — *out of scope right now, but on our radar.*
  Modules sourced **directly** from git (`source = "git::https://…"`, `source = "github.com/org/repo"`)
  bypass the module registry protocol, so the HTTP registry proxy never sees them and cannot cache them.
  (Registry-sourced modules that *resolve* to a `git::` `X-Terraform-Get` **are** cached now — the module
  mirror resolves and packs them; see Scope.) Closing the direct-git gap is **not** a from-scratch
  git-protocol server: it is local **bare mirrors** (`git clone --mirror`) plus **`insteadOf`** URL rewriting,
  reusing the `GIT_CONFIG_*` `insteadOf` plumbing Atmos already injects
  (`pkg/auth/providers/atmospro/broker`) and the go-getter git downloader (`pkg/downloader`). Tractable,
  well-trodden, and the highest-value follow-up.
- **OCI artifacts** — future phase.
- **Object-storage backends** — **V1 is filesystem-only, end-to-end.** The backend adapter pattern is in place
  (and the `pkg/ci/artifact` `s3`/`github` backends already exist), but no object-storage backend is required
  or wired for V1; the complete solution is expected to merge before any object-storage backend is added.
  Object storage is a clean follow-up that needs only configuration + tests.
- **An Atmos-built enforcement engine** — provider-source trust is achieved natively via `provider_installation`
  include/exclude (see Governance).
- **An authoritative database** — the filesystem is the index.

## Scope: Providers and Registry-Sourced Modules

V1 caches **both** providers and modules through the proxy:

- **Providers** — the **Provider Network Mirror Protocol** is small, clean, and fully proxyable over HTTP. All
  provider downloads (metadata + zips) are cached.
- **Modules** — the **module registry protocol** is redirected via a `host` service-discovery override. Version
  listings and download resolution are cached, and the resolved source is cached **in full regardless of what
  `X-Terraform-Get` points to** — `git::` (the common case for the public registry and Cloud Posse modules) or
  HTTP archive. The module mirror rewrites `X-Terraform-Get` back through the proxy, resolves the source with
  go-getter, and stores it as a single `.tar.gz` artifact.
- **The boundary** — only modules sourced **directly** from git (`source = "git::…"` / `"github.com/org/repo"`,
  not via the registry) still bypass the proxy. Caching those is the **git mirror** on our radar (Non-Goals).

## User Experience

```yaml
components:
  terraform:
    cache:
      enabled: true
```

That's it. Atmos resolves the cache location, starts the proxy, generates the CLI config, manages locking and
population, and tears the proxy down on exit. The cache remains on disk for the next run.

## Order of Operations

1. **Startup** — load `components.terraform.rc` and `components.terraform.cache`; resolve cache root and
   backend.
2. **Cache init** — resolve the cache root via XDG (`$ATMOS_XDG_CACHE_HOME` → `XDG_CACHE_HOME` →
   `~/.cache/atmos`); ensure the layout `providers/ modules/ metadata/ objects/ locks/`.
3. **Proxy start** — bind an ephemeral listener on `127.0.0.1:0` (dynamic port, no collisions); serve in a
   background goroutine with graceful shutdown.
4. **RC generation** — inject `provider_installation { network_mirror { url = "http://127.0.0.1:<port>/" }
   direct {} }` and the module `host` service-discovery block into the user's `components.terraform.rc`; render
   and set `TF_CLI_CONFIG_FILE`.
5. **Terraform execution** — provider requests flow through the proxy.
6. **Cache lookup** — the mirror adapter computes a canonical cache key; the proxy acquires a file lock.
7. **Hit** — serve immediately. **Miss** — download upstream → temp file → verify integrity → atomic rename →
   release lock → serve.
8. **Concurrency** — multiple Atmos processes share the cache; locking guarantees one downloader, many
   readers.
9. **Shutdown** — Terraform exits, the proxy exits, the cache persists. If bytes-served-from-cache > 0, Atmos
   prints a one-line savings report.

## Caching Proxy Design

The proxy is **generic HTTP-proxy infrastructure** (`pkg/http/proxy`) with caching as injected behavior — it
is deliberately *not* named for caching, so a future **git mirror** can be a separate mirror on the same
infrastructure. It exposes a pluggable `Mirror` adapter interface (`Route(request) → key, upstream resolver,
artifact kind`) and rewrites artifact download URLs so that package downloads route back through the proxy and
get cached. The proxy tracks per-run statistics (hit count and bytes saved) for the savings report.

### No retries in the proxy (deliberate)

Atmos already owns retry behavior. The proxy must **not** add its own. On a cache miss it:

```text
Fetch once
Return result
Propagate failures
```

No exponential backoff, no retry loops, no hidden behavior. Retry/backoff is the caller's (Atmos's)
responsibility; the proxy is a thin, predictable cache-or-fetch layer. This keeps separation of
responsibilities clean and makes failures observable rather than silently masked.

### Credential, header, and User-Agent passthrough

The proxy is a transparent intermediary, so it must not strip the things a **private registry** depends
on. On every upstream fetch it:

- **Forwards all sensible request headers** from Terraform's inbound request — every header except
  hop-by-hop/connection-specific ones (`Connection`, `Transfer-Encoding`, `Host`, `Accept-Encoding`, …).
  So `Authorization` and any custom registry headers reach upstream unchanged.
- **Falls back to the native host-token env** when no `Authorization` is forwarded, honoring both
  `TF_TOKEN_<host>` (Terraform) and `TOFU_TOKEN_<host>` (OpenTofu).
- **Forwards Terraform's `User-Agent` verbatim.** Because Atmos already injects `TF_APPEND_USER_AGENT`
  into Terraform's own User-Agent, the Atmos identity rides along inside it and is never lost. We do not
  synthesize or override the User-Agent; whatever Terraform/OpenTofu presents to the proxy is what reaches
  the upstream registry.

### Terraform and OpenTofu parity

Both tools speak the **same** protocols and use the **same** headers, so the cache supports both with no
binary-specific branching:

- The provider network-mirror protocol, provider registry protocol, module registry protocol, the
  `.well-known/terraform.json` service-discovery document, and the `X-Terraform-Get` module header are
  identical across Terraform and OpenTofu (OpenTofu reuses them for compatibility).
- The mirrors are **host-keyed**, so `registry.terraform.io` and `registry.opentofu.org` (and any private
  registry host) coexist in the keyspace automatically.
- RC management (below) sets **both** `TF_CLI_CONFIG_FILE` and `TOFU_CLI_CONFIG_FILE`, and credential
  fallback checks both `TF_TOKEN_<host>` and `TOFU_TOKEN_<host>` — no heuristic guess about which binary
  is in use.

## Provider Registry Mirror

Implements the Provider Network Mirror Protocol endpoints:

- `GET /:host/:namespace/:type/index.json` → available versions.
- `GET /:host/:namespace/:type/:version.json` → platform packages with (proxy-rewritten) download URLs and
  hashes.
- `GET <package>` → the provider `.zip`.

Canonical cache key: `<host>/<namespace>/<type>/<version>/<os>_<arch>`. Integrity is verified with the
`zh:`/`h1:` hashes before an artifact is committed.

## Canonical Mirror Layout & Hydration

The filesystem backend stores providers in the **canonical network/filesystem-mirror layout** — the same
layout produced by `terraform providers mirror` / `tofu providers mirror` and consumed by `filesystem_mirror`.
One directory then serves three ways:

1. **Lazy (proxy):** our proxy serves it over HTTP as a `network_mirror`, populating on demand during normal
   `init`.
2. **Eager (pre-seed):** `terraform/tofu providers mirror <cache-dir>` (and a future `atmos terraform cache
   warm`) pre-fetch providers across platforms into the same directory.
3. **Offline (no proxy):** Terraform consumes the directory directly via `filesystem_mirror`.

`providers mirror` is therefore **complementary** (eager) to the proxy (lazy), not a separate mechanism — both
speak the same provider-mirror content model. The reproducible-build bundle (below) is simply this directory.
(The object-storage backend stores object keys and is served via the proxy as a `network_mirror`; the direct
`filesystem_mirror` / `providers mirror` interchange is specific to the filesystem backend.)

## Module Registry Mirror

The module mirror adapter implements the module registry protocol, redirected via a CLI-config `host`
service-discovery override (`services = { "modules.v1" = "http://127.0.0.1:<port>/..." }`):

- Caches module **version listings** and **download resolution** metadata, removing registry round-trips. The
  download resolution caches the *upstream* source string and rewrites it to the proxy at **serve time**, so a
  value cached in a prior run stays valid even though the proxy binds a different ephemeral port each run — and
  a fully warm cache resolves a download with no upstream call.
- Caches the **resolved source** for every download by rewriting `X-Terraform-Get` to a `_source` sub-route.
  On a miss the mirror resolves the source with go-getter (`pkg/downloader`, reusing its `insteadOf`/credential
  plumbing) and stores it as a single **`.tar.gz`** artifact (immutable, keyed by the resolved source minus
  its `//subdir` so a mono-repo referenced by several modules is fetched once). This covers `git::` sources
  (the common case) and HTTP archives uniformly. The rewritten `X-Terraform-Get`/`location` carries a `.tar.gz`
  extension so the Terraform-side go-getter detects and unpacks it — a bare `.tar` is **not** in Terraform's
  module decompressor set, so gzip is required, not just a size optimization.
- **Boundary:** only modules sourced **directly** from git (not via the registry protocol) bypass the proxy;
  caching those is the **git mirror** (Non-Goals; on the radar). The Terraform-facing contract is unchanged.

## Cache Implementation

- **Backend substrate** — reuses the shipped `pkg/ci/artifact` package: the `Backend` interface
  (`Upload`/`Download`/`Exists`/`Delete`/`List`/`GetMetadata` by name) and its `Register(type, factory)`
  registry, with existing `local`, `s3`, and `github` backends, SHA-256 integrity, identity-aware AWS auth,
  and tar bundling. The registry cache consumes raw blobs (bypassing the higher-level bundled store for
  per-object provider zips).
- **Lock manager** — reuses the shipped `pkg/cache.FileLock` (`NewFileLock(path)` → `WithLock`/`WithRLock`):
  `flock` on Unix, graceful degradation on Windows, a `.lock` sidecar that survives atomic renames, and
  bounded retries. Multi-architecture; no new locking code.
- **Freshness** — registry **metadata** (versions, download metadata) honors `metadata_ttl` with
  stale-while-revalidate; **artifacts** (provider zips) are immutable and effectively cached forever.

## Cache Key Design

The cache key is the authoritative, canonical identity of an artifact and the foundation for everything
downstream — lock-file paths, on-disk layout, object-store keys (future), metrics aggregation, and bundle
exports — so it is defined explicitly here. In V1 the filesystem backend represents the key directly as a path;
other backends (e.g. S3) map the same key string to their own addressing without translation:

```text
Providers:
  <host>/<namespace>/<type>/<version>/<os>_<arch>
  e.g. registry.terraform.io/hashicorp/aws/5.95.0/linux_amd64

Modules:
  <host>/<namespace>/<name>/<provider>/<version>
  e.g. registry.terraform.io/cloudposse/vpc/aws/2.1.0

Git mirrors (future):
  <host>/<org>/<repo>
  e.g. github.com/cloudposse/terraform-aws-vpc
```

Properties:

- **Deterministic and collision-free** — distinct artifacts never share a key; the same artifact always
  resolves to the same key across machines and backends.
- **Lock scope** — `pkg/cache.FileLock` is taken on `<key>.lock`, giving one-downloader/many-readers per
  artifact.
- **Backend-portable** — the same key string maps to a filesystem path (V1) or an object-store key (future S3)
  without translation, and provider keys align with the canonical mirror layout so `providers mirror` /
  `filesystem_mirror` interchange holds.
- **Bundle-addressable** — a bundle export is a selection of keys, so the key scheme is also the manifest
  vocabulary shared with the atmos-bundles PRD.

## Configuration Reference

Cache settings live under `components.terraform.cache`. Conceptually the cache is **execution-environment
(runner) configuration, not stack data** — it describes *how* a machine runs Terraform (where to cache, which
backend, TTLs), not *what* infrastructure a stack declares. It nests under `components.terraform` because that
is where Atmos already groups runner-level Terraform behavior (e.g. `command`, `plugin_cache`), and because it
merges cleanly through the stack hierarchy so an org can set it once and components inherit it.

<dl>
  <dt><code>components.terraform.cache.enabled</code></dt>
  <dd>Enable the registry cache. Defaults to <code>false</code>.</dd>

  <dt><code>components.terraform.cache.location</code></dt>
  <dd>Cache root. Defaults to the XDG cache directory (<code>~/.cache/atmos</code> or
  <code>$XDG_CACHE_HOME/atmos</code>).</dd>

  <dt><code>components.terraform.cache.backend.type</code></dt>
  <dd>Storage backend. <code>filesystem</code> (default, maps to artifact <code>local</code>);
  <code>s3</code> (maps to artifact <code>s3</code>) in a follow-up.</dd>

  <dt><code>components.terraform.cache.metadata_ttl</code></dt>
  <dd>Time-to-live for registry metadata. Defaults to <code>24h</code>.</dd>

  <dt><code>components.terraform.cache.stale_while_revalidate</code></dt>
  <dd>Window during which stale metadata may be served while revalidating. Defaults to <code>168h</code>.</dd>
</dl>

```yaml
components:
  terraform:
    cache:
      enabled: true
      location: ~/.cache/atmos
      backend:
        type: filesystem
      metadata_ttl: 24h
      stale_while_revalidate: 168h
```

There are no Atmos-specific `mode` or `trusted_sources` keys — see Governance.

## Storage Backends

The cache reuses the `pkg/ci/artifact` backend registry. **V1 implements and ships the `local` (filesystem)
backend only, working end-to-end** — the entire solution is expected to merge before any object-storage
backend is added. The adapter pattern keeps that door open: the `s3` and `github` backends already exist in
`pkg/ci/artifact` and become available with only configuration + tests. Object storage will enable shared CI,
team, and organizational caches; it becomes the authoritative backend with the local filesystem as an optional
tier.

## Governance (native, documented)

The cache makes the proxy the single egress point for providers, and Terraform's native
`provider_installation { network_mirror { include/exclude } direct { exclude } }` (set through
`components.terraform.rc`) already restricts which providers may load and from where. This delivers
supply-chain control, approved-dependency lists, and auditability **with no new Atmos abstraction**. This PRD
documents those patterns rather than building an enforcement mode. Module governance has no native equivalent
and is deferred with module caching.

## Cache Management Commands

```bash
atmos terraform cache list             # list cached artifacts
atmos terraform cache stats            # size, object count, provider/module breakdown
atmos terraform cache prune            # apply retention policy
atmos terraform cache delete <key>     # remove a specific artifact
```

`cache stats` reports only **filesystem-derivable facts** — total size, object count,
provider-vs-module breakdown, largest and oldest objects. It deliberately does **not** report a hit
rate: by Architectural Principle 1 the filesystem is the index and there is no persistent hit/miss
store. A hit is an event, not stored state. Per-run hit counts and bytes-saved are surfaced once, by
the end-of-run **savings report**; persistent hit-rate reporting would require the optional future
metrics database (out of scope) and is intentionally absent.

Future: `atmos terraform cache warm` (wraps `providers mirror` to pre-seed) and `export` / `import` (bundle
hooks).

## Air-Gapped Reproducible Builds (North Star)

The ultimate goal is **reproducible builds in air-gapped environments**: a self-contained on-disk closure of
everything a set of stacks needs — every provider, and every module's source — that can be packaged,
transported, and replayed offline with byte-for-byte fidelity.

What this PRD delivers gets us most of the way and is the foundation for the rest:

- **Providers** — fully captured (Provider Network Mirror Protocol) and replayable offline via the proxy or
  `filesystem_mirror`. ✅ in V1.
- **Registry-sourced modules** — registry metadata, download resolution, and the **resolved source captured in
  full** (whether `X-Terraform-Get` is `git::` or an HTTP archive — Cloud Posse modules and mono-repos
  included), stored as tar artifacts. ✅ in V1.
- **Directly-declared git modules** (`source = "git::…"` / `"github.com/org/repo"`, not via the registry) —
  **require the git mirror** (Non-Goals; on the radar). This is the remaining gap to a *complete* air-gapped
  closure. The good news: it reuses Atmos's existing `GIT_CONFIG_*` `insteadOf` injection and go-getter git
  downloader, so it is local bare mirrors + URL rewriting — not a git-protocol server.

In short: this PRD captures providers and all registry-sourced modules; the git mirror completes the closure
for modules sourced directly from git. The design is sequenced so the git mirror slots in without changing the
Terraform-facing contract.

## Reproducible-Build Bundles

Once warm, the cache directory is the build closure. That directory (providers in canonical mirror layout,
cached module source tars, and — once the git mirror lands — local bare git mirrors) is exportable as a
**bundle** — reusing `pkg/ci/artifact` tar bundling — for air-gapped or reproducible distribution. **The
bundle format and distribution are owned by the separate atmos-bundles PRD**; this PRD exposes the cache as
the bundle's source of truth and provides the future `atmos terraform cache export` / `import` hooks.

## Benefits

- **Performance** — fewer repeated downloads; faster local iteration and CI.
- **Reliability** — reduced dependence on `registry.terraform.io`, GitHub, and release servers; cached
  artifacts remain available during outages.
- **Reproducibility** — builds remain deployable even if upstream content disappears.
- **Supply-chain security** — approved sources enforced natively; a single auditable egress.
- **Air-gapped operation** — cached artifacts can be packaged and transported into disconnected environments.
- **Visible value** — a per-run savings report (bytes saved + hit count) printed before exit when > 0.

## Success Criteria

- **Performance** — >90% reduction in repeated provider downloads across runs.
- **Reliability** — Terraform runs succeed during upstream outages when artifacts are cached.
- **Developer experience** — caching requires no Terraform code changes and minimal Atmos config.
- **Visibility** — users regularly see measurable savings reported on real runs; this is arguably the most
  visible proof that the feature is working.

## Roadmap

- **Phase 1** — [Terraform RC Management](terraform-rc-management.md): generate CLI config; no caching.
- **Phase 2** — Caching proxy + **provider mirror and module registry mirror** + filesystem (`local`) backend
  in canonical layout + cache management commands.
- **Phase 3** — Shared object-storage backends (`s3`, `github`); shared CI/team caches; `cache warm` /
  `export` / `import`.
- **Phase 4** — **Git mirror** (modules sourced directly from git, not via the registry); OCI; Atmos CI integration; air-gapped
  packaging.

## Long-Term Vision

The goal is not merely a Terraform cache but a generic Atmos caching-proxy + artifact-cache framework. The
same primitives — cache backend, lock manager, TTL policy, stale-while-revalidate, local filesystem and object
storage — later serve git mirrors, OCI artifacts, CI caches, and reusable package downloads. Terraform
registry caching is the first frontend built on that foundation.

## Multi-platform lock files (`.terraform.lock.hcl`)

A network mirror — and the default provider plugin cache (`TF_PLUGIN_CACHE_DIR`, on by default) — is a
"customized provider installation method", so `terraform/tofu init` can no longer record the registry's signed
cross-platform checksums and writes a `.terraform.lock.hcl` with hashes for only the **current** platform. It
then prints *"Incomplete lock file information for providers … only includes checksums for `<host>`"* (see #2150),
which breaks any other platform in a fleet that shares the committed lock.

**Resolution.** Declare the platforms a project targets once, at `components.terraform.platforms` (a first-class
list of `<os>_<arch>`). A single list drives **both** eager `atmos terraform cache mirror` pre-seeding **and**
automatic lock completion. A built-in `after.terraform.init` provisioner (`pkg/provisioner/lock`, registered like
the backend/source/workdir hooks) runs `providers lock -platform=…` for the declared platforms once init succeeds,
reusing the same binary, env (incl. `TF_CLI_CONFIG_FILE` → the live proxy), and working dir. It is a silent no-op
unless platforms are declared (beyond the host) **and** a customized install method is active — so non-adopters
see no change, and projects with neither cache native-complete their locks already. Writes are serialized with
`pkg/cache.FileLock` (sidecar kept under the temp dir, never in a committed component directory).

**Per-instance locks for ephemeral/vendored components.** One Terraform root module is shared by N
`(stack, component)` instances whose `required_providers` can resolve to different versions; a single
`.terraform.lock.hcl` cannot hold two versions of one provider, and merging N locks could never *expunge* a
no-longer-required provider. So for **plain in-repo** components the canonical lock is completed in place and
committed as usual. For **ephemeral or vendored** components (`provision.workdir.enabled` or a `source:`) the
canonical lock has no committable home, so Atmos keeps the committed source of truth in a per-instance dotfile,
`.<stack>-<component>.terraform.lock.hcl` (the same disambiguation used for varfiles/planfiles), with this
lifecycle:

1. **Restore** (source/workdir provisioner, `before.terraform.init`): copy the per-instance lock → canonical
   `.terraform.lock.hcl` in the working dir, so init honors the instance's pinned providers.
2. **Complete** (`after.terraform.init`): `providers lock -platform=…` fills in every platform.
3. **Persist**: copy the completed canonical lock → the per-instance dotfile (whole-file, no merge → stale
   providers vanish), `FileLock`-guarded; the destination is the workdir's recorded source dir, else the vendored
   working dir.

The source→workdir sync preserves the workdir's managed lock and never drags a source lock in (`shouldSkipSyncFile`).
Gitignore convention: keep canonical `**/.terraform.lock.hcl` ignored (scratch) and **un-ignore**
`!**/.*-*.terraform.lock.hcl` so per-instance locks commit even inside vendored dirs.

## Open Questions / Risks

- Whether to promote `pkg/ci/artifact` → `pkg/artifact`, since it is now consumed by CI artifacts, the registry
  cache, *and* bundles — its `ci` namespacing no longer fits.
- Exact canonical-mirror-layout alignment (packed vs. unpacked; generated `index.json` / `<version>.json`) so
  that `providers mirror` / `filesystem_mirror` interchange holds.
- Precedence/merge with a user-supplied `TF_CLI_CONFIG_FILE`.
- Terraform vs. OpenTofu registry hosts and `providers mirror` command differences.
- Windows file-lock degradation (shared-cache concurrency guarantees are weaker on Windows).
