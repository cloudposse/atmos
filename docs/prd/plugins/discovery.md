# Plugin discovery: repositories and optional catalogs

**Status:** Native pilot implementation in the working branch. [All plugin PRDs](README.md).

## Product decision

Support two discovery inputs through one search interface and local cache:

| Input | Appropriate use | Publisher responsibility |
|---|---|---|
| Explicit OCI repository list | A small private collection | Publish each plugin release independently. |
| OCI catalog snapshot | A public registry or curated product collection | Maintain and publish a complete discovery snapshot through a single serialized publisher. |

Catalogs are optional. Direct installation by OCI repository never requires discovery configuration. A project can combine multiple catalogs and repository lists, including one catalog per product. Registry authentication is independent of discovery and can be configured without either input.

Atmos includes the `cloudposse` alias for `oci://ghcr.io/cloudposse/atmos/plugins/catalog:stable` by default. Set `plugins.discovery.default_registry: false` to opt out. An explicit alias replaces the complete default entry. See [official public packages](public-registry.md) for the manifest-driven release and serialized catalog workflows.

## Configuration

```yaml
plugins:
  max_concurrency: 4
  discovery:
    cache_ttl: 15m
  registries:
    team:
      host: registry.example.com
      identity: registry-reader
      repositories:
        - oci://registry.example.com/team/oidc
        - oci://registry.example.com/team/credentials
    infrastructure:
      catalog: oci://ghcr.io/acme/infrastructure/catalog:stable
    developer-tools:
      catalog: oci://ghcr.io/acme/developer-tools/catalog:stable
    publisher:
      host: publishing.example.com
      identity: publisher
```

`host` is a registry authority, including a port when present. All discovery inputs on an entry must match that host. When omitted, it is inferred from the configured inputs, preserving existing catalog-only configuration. `identity` references `auth.identities`; conflicting identities for the same normalized host fail closed. An entry with a host and no discovery inputs supplies authentication only and produces no search rows.

Repository lists contain OCI repositories without version tags or digests. Atmos does not enumerate an entire registry or depend on a vendor's repository search API. A catalog may reference releases on another host, but those release destinations receive their own source-policy and authentication checks.

## Search and cache behavior

`atmos plugin search` searches all configured discovery inputs; `--registry <alias>` restricts the set. Name/description, capability, and runtime filters use the same result model for both inputs. Results retain their registry alias, and alias-qualified installation resolves through the same discovery service before ordinary installation. Common Atmos list formatting and machine formats remain unchanged.

Direct indexing lists repository tags and reads release descriptors and static plugin metadata. It never runs a plugin or downloads its executable payload. Metadata is reused by immutable release digest: refreshing a repository rechecks its tags but does not repeatedly fetch unchanged release metadata. Removed tags disappear from a successfully refreshed index. Concurrent source requests are bounded by `plugins.max_concurrency`.

The cache lives under XDG cache at `atmos/plugins/discovery`; `plugins.cache_dir` overrides this with `<cache_dir>/discovery`. Entries include freshness information and source identity. Each cache key includes its source reference and kind, auth configuration, effective realm, and effective host-to-identity bindings. Adding another repository does not invalidate the existing repositories' metadata; projects with equivalent effective source/auth scopes may share it. Different auth configurations or realms do not reuse the same private discovery scope. Cache files contain no credential values or tokens. Logout does not purge advisory discovery metadata; cached search may remain available afterward, while installation and registry access independently enforce current policy and authentication. This scope is not an authorization boundary against another process running as the same operating-system user.

`plugins.discovery.cache_ttl` accepts a nonnegative Go duration, defaults to `15m`, and accepts `0` to revalidate on every online search. `search --refresh` bypasses freshness. Offline policy permits cached discovery even when stale; missing or corrupt caches fail explicitly. `--refresh` under offline policy fails rather than silently ignoring the flag. An online refresh failure fails the search rather than silently falling back to stale or partial results. Cache refreshes use an exclusive per-source file lock and atomically replace completed snapshots, preventing parallel local processes from overwriting each other during refresh.

Source policy is re-evaluated for configured inputs and result destinations on every search, including cache hits. Offline discovery does not authorize installation from unavailable bytes. Installation still enforces current source and package policy, signatures, compatibility, and digest integrity; a cached catalog or index never grants trust.

## Independent publication and catalog ownership

`plugin publish` publishes the plugin release, with optional signing. It does not accept `--catalog` and does not mutate discovery metadata. A small private collection becomes searchable after the consumer refreshes its configured repository index.

Catalog maintainers retain explicit `catalog build`, `catalog publish`, `catalog add`, and `catalog remove` operations. A public or product registry may keep its accepted release inventory in Git, validate changes in CI, and publish the resulting snapshot. The current CLI exposes the snapshot operations; it does not supply a hosted indexing service or a cross-system CI scheduler.

Each catalog has one externally serialized publishing workflow. Its lock must cover reading the latest accepted inputs, constructing the update, and replacing the catalog tag. A queue alone is insufficient if an older prebuilt snapshot can overwrite a newer one. Independent plugin release jobs must not all run `catalog add` against the same catalog.

Revision and digest rechecks catch an already-stale update; they are not atomic compare-and-swap and cannot eliminate simultaneous-writer races. Atmos supplies no portable registry lock. Restrict catalog write credentials to the catalog workflow and use its CI concurrency controls. Different product catalogs may publish independently.

A failed catalog update leaves previously published plugin artifacts available by direct reference. Catalog consumers see the previous snapshot until the maintainer publishes the update. Removing a catalog entry only unlists it; it neither deletes the artifact nor rewrites consumer locks.

## Acceptance

- A project with only explicit repositories can publish, search, select by source alias, and install without creating a catalog.
- Multiple catalogs, repository lists, and auth-only entries coexist; filtering retains the selected source alias.
- Fresh-cache search makes no registry requests; explicit or expired refresh rechecks sources and reuses unchanged digest metadata.
- Offline discovery returns cached results without network requests and rejects missing/corrupt cache or explicit refresh.
- Changed auth configuration, effective host identity bindings, or realm does not reuse another scope's private discovery index; cached data contains no tokens. Adding a repository retains the existing references' cache, and logout does not promise to purge discovery metadata.
- Policy changes apply to cached results, and install cannot use discovery cache as a substitute for verification.
- Release publishing leaves catalogs untouched. Documentation and help state the external serialization requirement for all catalog mutations.
