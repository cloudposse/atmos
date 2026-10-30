# Atmos Artifacts: Repositories, Publishing, Discovery, and Automation

**Status:** Proposed. This document specifies future capabilities; its configuration and command
examples are not a claim that those capabilities are implemented. This change is documentation only.

## Problem and outcome

Atmos needs one way to publish, retrieve, inspect, and discover the assets its users produce and
consume: components, scaffolds, backups, verified tool packages, CI outputs, and OCI content.
Users should configure one destination and publish many named assets into it. Switching between
OCI, S3, directories, and archives should change the selected repository, not the publishing workflow.

The same operations must work interactively, in CI, in workflows, and as lifecycle hooks. CI runs
must leave useful job summaries rather than requiring users to reconstruct publication results
from logs. Package README and CHANGELOG documents must be available without downloading a package.

This restores and revises the earlier `docs/prd/artifacts.md` draft preserved at commit
`2f81e11338d28a74c2a7fdf18a52d3cd5136ac4a`. That draft focused on container images and
`containers/image/v5`. This proposal broadens V1 to general artifacts and selects `oras-go/v2`
for shared OCI transport. Deferred requirements from that draft are retained below.

The scaffold documentation requirements also reflect the Erik/Judith conversation supplied during
planning: package documentation is distinct from generated repository documentation, and release
notes should eventually support template-update PRs. Changelog aggregation is deferred.

## Use-case inventory

Artifacts are the distribution and restoration layer for the following use cases. Offline and
air-gapped operation is a cross-cutting goal: content must remain usable after its upstream is unavailable.

| Use case | Content to store | Intended consumption or restoration |
| --- | --- | --- |
| Scaffolds and templates | Template trees, scaffold definitions, package README/CHANGELOG, and supporting files | Generate or update projects from a pinned template release |
| Components | Terraform, Helmfile, and other component source trees, scripts, and associated files | Provision components from a controlled repository |
| Terraform modules | Root or child module source, origin/version metadata, and checksums | Resolve pinned module dependencies without the original Git/registry source |
| Stack configuration | Stack manifests, catalogs, mixins, imports, and their dependencies | Load and compose stacks from published configuration |
| Atmos configuration | `atmos.yaml`, imported configuration, custom commands, workflows, policies, and supporting scripts | Bootstrap a consistent Atmos environment and reuse organizational automation |
| Toolchain packages | Verified original tool releases, platform information, lock metadata, and verification evidence | Install the required tools without contacting upstream release servers |
| Toolchain caches | Selected cached downloads and, eventually, installed tool trees with platform/version metadata | Warm or restore a compatible toolchain cache for offline execution |
| Terraform provider cache | Provider packages, origin addresses, versions, platforms, checksums, and mirror index metadata | Export Atmos-managed provider caches and restore a usable offline provider mirror |
| Terraform module cache | Cached module sources, registry resolution metadata, versions, and integrity records | Export Atmos-managed module caches and restore offline module resolution |
| OCI images and artifacts | Complete manifests, indexes, blobs, platforms, and required related artifacts | Run or consume pinned OCI content from an internal registry or local layout |
| Vendored dependencies | Git/HTTP source snapshots and dependency provenance | Hydrate the same source trees without contacting upstream sources |
| CI outputs | Planfiles, build outputs, generated configuration, reports, and run provenance | Exchange verified results between jobs and retain inspectable outputs |
| Backups | Selected files/directories and restoration metadata | Restore a copied directory or a versioned archive from local storage or S3 |
| Atmos plugins | Existing plugin release packages and associated verification material | Distribute/install existing plugin formats while preserving immutable-release policy |
| SBOMs and attestations | Inventory documents, provenance, signatures, and subject relationships | Inspect and verify artifacts and their dependencies in disconnected environments |
| Air-gap distribution | A selected closure of configuration, components, modules, providers, tools, images, and metadata | Transfer into a disconnected environment and restore everything needed for a defined workflow |

This inventory defines the product vision; delivery phases are specified below. Cache restoration must
preserve indexes, locks, checksums, and dependency relationships and prove consumption with upstream disabled.

## Model and V1 boundaries

- **Repository:** A configured collection root holding many named artifacts, possibly of different
  kinds. It is a destination and discovery source, not a declaration of one package.
- **Artifact:** Versioned content with portable metadata, labels, document descriptors, and digests.
- **Artifact kind:** Meaning of the content, such as `component/terraform`, `scaffold`,
  `toolchain/package`, or generic `files`. It is independent of the repository kind.
- **Packaging:** Representation of a file payload: individual files or an archive.
- **Reference:** Repository alias plus artifact name and version, or name and immutable digest.
- **Catalog:** An optional maintained discovery snapshot. It is not necessary for direct reads.

V1 includes publishing, pulling, copying, inspection, listing, search, deletion, package documents,
label filtering, optional catalogs, hooks, and CI reporting. OCI manifests/indexes are transferred
as content graphs, not interpreted as ordinary directory archives.

Artifacts are not components. Existing top-level `stores` remain key-value and secret stores.
An archive in this proposal packages one artifact; it is not a snapshot of an entire repository.

## Public configuration

`artifacts.repositories` is the single list of destinations to use and discover. Do not add another
`discovery.repositories` list or require one configuration entry per scaffold or component.

```yaml
artifacts:
  repositories:
    shared:
      kind: oci/registry
      identity: registry-publisher
      options:
        registry: ghcr.io
        prefix: acme/atmos
      defaults:
        metadata:
          labels:
            team: platform

    backups:
      kind: aws/s3
      identity: backup-writer
      packaging: tgz
      options:
        bucket: acme-artifacts
        prefix: atmos
        region: us-east-2
        tags:
          purpose: backup

    local:
      kind: local/dir
      options:
        path: .atmos/artifacts

    archives:
      kind: local/archive
      packaging: tgz
      permissions: "0600"
      options:
        path: .atmos/backups
        key_pattern: '{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}.tgz'

    offline:
      kind: oci/layout
      options:
        path: .atmos/oci
```

`identity` references Atmos auth identities; credentials do not belong in repository options.
`prefix` is an Atmos collection-root convention, not an ORAS namespace field. ORAS addresses a
registry, an OCI repository, and a tag or digest. Atmos maps a collection and artifact name onto
that OCI repository. Local backends use `path` for their destination root.

### Uniform addressing

```shell
atmos artifacts publish ./components/terraform/vpc \
  --repository shared \
  --name components/terraform/vpc \
  --version 1.2.0 \
  --kind component/terraform
```

Changing only `--repository` selects another destination:

| Repository | Resolved payload location with the configuration above |
| --- | --- |
| `shared` | `ghcr.io/acme/atmos/components/terraform/vpc:1.2.0` |
| `backups` | `s3://acme-artifacts/atmos/components/terraform/vpc/1.2.0.tgz` |
| `local` | `.atmos/artifacts/components/terraform/vpc/1.2.0/` |
| `archives` | `.atmos/backups/2026/10/30/components/terraform/vpc/1.2.0.tgz` for that creation date |
| `offline` | A named reference in the OCI layout at `.atmos/oci` |

Metadata and document sidecars accompany payloads where required; the table shows payload locations.
Names may contain relative slash-separated segments. Reject absolute paths, traversal, empty
segments, and backend-invalid names or versions before uploading; do not silently rename them.
Versions are explicit labels, including CI build identifiers, rather than universally requiring
SemVer. Kind-specific rules, including plugin release rules, still apply.

Publishing takes a local source argument. Explicit `--name` wins; otherwise use available package
metadata, then the source basename. Explicit `--version` wins over package metadata; fail when
neither supplies a version. Do not invent `latest`. Reads use `name:version` or `name@sha256:...`.
Require `--repository` for single-repository operations; list/search may span configured repositories.
Resolve mutable references once per operation and pin subsequent reads to that digest. Digest
references select OCI manifest/index digests on OCI backends and immutable record digests on
local/S3 backends; portable content digests are reported separately.

### Packaging, destinations, and permissions

| Repository kind | File payload packaging | Default |
| --- | --- | --- |
| `local/dir` | `none` | `none` |
| `local/archive` | `tar`, `tgz`, `zip` | `tgz` |
| `aws/s3` | `none`, `tar`, `tgz`, `zip` | `tgz` |
| `oci/registry`, `oci/layout` | `tar`, `tgz`, `zip` for newly packaged files | `tgz` |

With S3 `packaging: none`, publish separate objects below `<prefix>/<name>/<version>/`.
With archive packaging, publish one payload object plus metadata/documents. Existing OCI content
keeps its original media types and graph; repository packaging settings do not repackage it.

`options.path` is always the repository destination, never the publish source. Relative configured
paths resolve against the configuration file declaring them; CLI source/output paths resolve
against the invocation working directory. Workflow/hook source/output paths follow their existing
working-directory rules. Config loading must retain path provenance through imports.

Local/S3 `key_pattern` overrides the path below the stable root. Default paths follow the addressing
table. Expose `.Name`, `.Version`, `.Kind`, and `.CreatedAt`; use the existing Go template functions.
Capture `.CreatedAt` once in UTC for a publication and reuse it across retries. Reject root escapes
and conflicting resolved paths; validate archive extensions against packaging. There is no implicit
strftime interpretation of `%Y` or `%M`.

Persist the resolved location with the artifact's name/version record. Reads, deletes, and retries
use that record, not today's date. Keep discoverable records under the stable root even when
payloads use dated paths. An interrupted publication must not expose a complete-looking record.

`permissions: "0600"` controls the resulting local archive file, not its members, S3 access, or OCI
access. Accept quoted octal permission bits; reject `preserve` and unsupported backend usage.
Default local archive permissions are `"0600"`. Preserve ordinary payload permission bits,
including executable bits, where the target filesystem supports them. Full owner, ACL, xattr,
and privileged-bit backup fidelity is outside V1; report unsupported permission requirements.

## Command surface and mutation semantics

All examples in this section are proposed interfaces. The public command space is plural:
`atmos artifacts`.

```shell
atmos artifacts publish ./dist --repository backups --name builds/app --version build-123
atmos artifacts publish ./dist --repository backups --name builds/app --version build-123 --force
atmos artifacts publish ./dist --repository backups --name builds/app --version build-124 --dry-run
atmos artifacts pull components/terraform/vpc:1.2.0 --repository shared --output ./vendor/vpc
atmos artifacts copy components/terraform/vpc:1.2.0 --from shared --to backups
atmos artifacts inspect components/terraform/vpc:1.2.0 --repository shared --format json
atmos artifacts list --repository shared
atmos artifacts list components/terraform/vpc --repository shared
atmos artifacts search vpc --labels 'team=platform,purpose=backup'
atmos artifacts show readme components/terraform/vpc:1.2.0 --repository shared
atmos artifacts show changelog components/terraform/vpc:1.2.0 --repository shared
atmos artifacts delete builds/app:build-123 --repository backups
```

- `publish` creates a versioned artifact from a file or directory. Update means republishing that
  reference with `--force`; there is no separate in-place editing command or `--replace` flag.
- `pull` verifies and materializes content at `--output`. Existing different output requires
  `--force`; validate/extract into staging before replacing it. Do not overlay stale files.
- `copy` preserves artifact identity, metadata, documents, and verified content. Destination
  packaging may change physical representation; destination label defaults do not restamp copies.
  Use backend-native transfer where available and a bounded streaming/staging fallback elsewhere.
- `inspect` reads metadata; `show` reads a selected document. Neither downloads the main payload.
- `list` enumerates artifacts/versions; `search` filters name/description and labels. Both identify
  the source alias, support structured output, and accept `--refresh` for discovery-cache refresh.
- `delete` removes the selected published reference and its exclusively owned content where the
  backend supports it. It never means delete the collection, purge all versions, or remove a
  catalog entry. Shared OCI blobs are subject to registry garbage collection.

Compare content, documents, and effective user metadata before publishing. An identical publication
is a no-op; generated timestamps must not turn an identical retry into different content. Reuse
the existing record on a no-op. Different content at the same name/version fails unless `--force`
is supplied. Forced publication creates a replacement record and updates the reference; it does
not mutate bytes addressed by an old digest. Old digests remain readable only while retained.

Archive assembly must be deterministic for the same input bytes, paths, modes, and declared
metadata. Normalize incidental archive timestamps and ordering. Separate content identity from
generated operation metadata and backend representation digests. `inspect` and machine output
must clearly label content/record digests versus OCI manifest or archive checksums.

OCI tag updates are not portable compare-and-swap. Require external serialization for writers to
the same mutable reference and reconcile interrupted writes before retrying. Use conditional
writes/locks where supported; never claim universal atomic publication across multiple objects.
Publish completion metadata last. Unsupported operations fail explicitly before mutation.

`--force` does not bypass authentication, verification, or immutable-release policy. In particular,
plugin releases remain immutable. Generic artifact operations must not provide an overwrite path
around plugin-format immutability when they encounter plugin releases.

Dry runs display selected files, exclusions, metadata, and resolved destination without mutation.
Commands are noninteractive in CI, return nonzero for operation failures, and keep structured
results on stdout and human progress on stderr. Authentication uses existing Atmos auth/profile
flags and identities; no new artifact login/logout subsystem is introduced.

## Metadata and package documentation

The portable artifact record contains name, version, kind, description, creation/provenance data,
labels, content inventory, document descriptors, and integrity information. Inventory describes
relative paths, sizes, digests, and file modes. CI provenance may include commit, run, and source
repository identifiers; generic artifacts do not require a Terraform stack/component/SHA tuple.

Repository `defaults.metadata.labels` supplies defaults for newly published artifacts. Explicit
publication metadata overrides matching default keys. These labels belong to each published
artifact version, not to the repository itself. Metadata changes affect artifact identity and
follow the same conflict/force rules as content changes.
Use `publish --labels 'team=platform,purpose=backup'` for CLI overrides and `metadata.labels` in
artifact steps. Inspection and machine output return the effective labels after defaults merge.

| Backend | Authoritative artifact metadata | Efficient document access |
| --- | --- | --- |
| OCI | Versioned Atmos config blob referenced by the manifest; standard annotations for standard fields | Separate document blobs referenced by descriptors |
| S3 | JSON sidecar record, not only object headers or tags | Separate document objects |
| Local directory | Managed metadata alongside payload content | Direct document files |
| Local archive | Adjacent record plus embedded portable metadata | Adjacent document sidecars; embedded copies for transport |

For archives, embed portable metadata and documents under `.atmos/artifact/`; keep operation and
storage-location data in the adjacent record to avoid self-referential archive checksums. Readers
verify document descriptors and the payload checksum from that record. If only a standalone archive
is available, recover embedded metadata/documents by scanning it; do not promise random access into
gzip. Sidecars are part of the managed publication and must be consistent with its record.

S3 `options.tags` specifies backend object tags for lifecycle/access integration. Apply them to
payload, metadata, and document objects of the publication. They do not replace portable labels,
and labels are not automatically copied into tags. Validate the S3 limit of ten tags before writes;
tag changes outside Atmos do not change portable artifact labels.
See [S3 object tagging](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-tagging.html).

### README and CHANGELOG conventions

```text
components/terraform/vpc/
  main.tf
  README.md
  .atmos/artifact/
    README.md
    CHANGELOG.md
```

Default package documents are `.atmos/artifact/README.md` and `.atmos/artifact/CHANGELOG.md`.
Both are optional. `--readme` and `--changelog` override their source paths, resolved relative to
the publish source directory (or the parent for a single-file source); explicit missing files fail.
Step equivalents are `readme` and `changelog`. No changelog format/parser is required in V1.

Root README/CHANGELOG files remain payload unless explicitly selected as package documents.
Existing scaffold root README handling, generated files, and post-generation instructions retain
their behavior. Reserve `.atmos/artifact/` for distribution metadata and exclude it from scaffold
generated output. Explicit document paths do not implicitly remove an ordinary payload file.

Store document media types, sizes, and digests in metadata. `show readme` and `show changelog` fetch
only the selected blob/object/file. Missing optional documents produce a clear not-found result.
Catalogs carry descriptors, not copies of full Markdown documents.

### OCI media types and Helm lessons

Use OCI image manifests with artifact metadata, not fabricated runnable image configurations.
ORAS is the transport/assembly library; it does not dictate one media type for all payloads.

| Role | Proposed type |
| --- | --- |
| OCI manifest | `application/vnd.oci.image.manifest.v1+json` |
| Generic artifact type | `application/vnd.atmos.artifact.v1` |
| Scaffold artifact type | `application/vnd.atmos.scaffold.v1` |
| Component artifact type | `application/vnd.atmos.component.v1` |
| Toolchain package artifact type | `application/vnd.atmos.toolchain.package.v1` |
| Metadata config | `application/vnd.atmos.artifact.config.v1+json` |
| Tar payload | `application/vnd.atmos.artifact.content.v1.tar` |
| Gzip-compressed tar payload | `application/vnd.atmos.artifact.content.v1.tar+gzip` |
| Zip payload | `application/vnd.atmos.artifact.content.v1.zip` |
| README/CHANGELOG blob | `text/markdown` |

Record component subtype in portable metadata. Preserve upstream media types for tool release
assets and existing OCI graphs. Existing plugin media types, manifest bytes, signatures, and
lockfiles must remain compatible; this proposal does not rename plugin packages.

Follow the [OCI artifact manifest guidance](https://github.com/opencontainers/image-spec/blob/main/manifest.md#guidelines-for-artifact-usage).
Adopt Helm's useful conventions: package name/version drive publication beneath a collection root,
and metadata and payload have distinct media types. See [Helm OCI registries](https://helm.sh/docs/topics/registries/).
Atmos does not reuse Helm media types for non-chart content.

## Discovery, labels, and optional catalogs

`list` and `search` use `artifacts.repositories` directly. Without `--repository`, search all
configured sources. `--labels 'team=platform,purpose=backup'` matches both exact key/value pairs
on the published version. Filter portable metadata, not S3 tags or a registry-specific label API.
Bound concurrent reads and reuse metadata by immutable digest; do not fetch payloads to search.

Directory, archive-root, S3, and OCI-layout repositories enumerate their managed records locally
or by object listing. An OCI collection prefix is not itself a portable repository-enumeration API.
Use supported registry enumeration scoped to the prefix when available. Otherwise collection-wide
discovery requires a catalog; fail with an actionable capability diagnostic rather than silently
returning an empty or supposedly complete result. Explicit-name operations, including
`list components/terraform/vpc`, can list that OCI repository's tags without a catalog.

An optional `catalog` belongs directly to the configured repository:

```yaml
artifacts:
  repositories:
    shared:
      kind: oci/registry
      identity: registry-reader
      options:
        registry: ghcr.io
        prefix: acme/atmos
      catalog: oci://ghcr.io/acme/atmos/catalog:stable
```

Adapt the [plugin discovery and catalog design](./plugins/discovery.md): separate catalog
build/publish/add/remove/sync operations, version/digest-level labels, bounded metadata reads,
and a private local cache scoped by effective source/auth configuration. Catalog references
never grant credentials to another host or bypass current source/verification policy.

`publish` never updates a catalog implicitly. Catalog maintainers use one externally serialized
writer spanning read-latest, build, and publish. Revision/digest checks detect stale updates but
are not atomic registry locks. Removing an entry unlists it without deleting its artifact.

Use a 15-minute discovery-cache TTL initially. `--refresh` revalidates sources and reuses unchanged
digest metadata. Offline operation uses cached discovery only; missing/corrupt caches and offline
refresh requests fail. Online refresh failures must not silently return stale or incomplete search
results. Cached discovery does not prove payload availability or authorize installation.

## Producers, workflows, and hooks

### Components and scaffolds

```shell
atmos artifacts publish ./components/terraform/vpc \
  --repository shared --name components/terraform/vpc --version 1.2.0 --kind component/terraform
atmos scaffold publish ./scaffolds/landing-zone \
  --repository shared --name scaffolds/landing-zone --version 2.0.0
```

`atmos scaffold publish` delegates to the generic publisher with scaffold validation and kind.
Keep scaffold source resolution and component source provisioning as consumers of shared OCI
operations. Publishing a component does not run Terraform or require a stack selection.
The component producer excludes Terraform working directories, state, and generated plan files;
dry-run output makes those exclusions visible. Generic backup/file publishing does not inherit
component-specific exclusions. Preserve executable scripts and package documents.

### Verified toolchain packages

Publish original downloaded release archives/executables plus platform, upstream URL, checksum,
size, and verification metadata from toolchain lock data. Verification precedes publication.
Do not treat repackaged installed binaries as equivalent to verified upstream release bytes.
Toolchain remains responsible for resolution, verification, extraction, installation, and PATH;
artifacts provide storage/distribution and an explicit repository-backed package source.
Prove installation from published packages with upstream access disabled. Installed-tool snapshots,
automatic dependency crawling, and changing native CI cache semantics are deferred.

### Shared step handler

Add `type: artifact` to the existing step registry. Its `action` selects the same service operation
as the CLI. `artifact_name` and `artifact_kind` distinguish artifact identity from the step's own
`name` and hook `kind`; the CLI equivalents are `--name` and `--kind`.

```yaml
workflows:
  publish-vpc:
    steps:
      - name: publish-vpc
        type: artifact
        action: publish
        source: ./components/terraform/vpc
        repository: shared
        artifact_name: components/terraform/vpc
        artifact_kind: component/terraform
        version: "1.2.0"
```

```yaml
hooks:
  backup-output:
    events: [after-terraform-apply]
    kind: step
    type: artifact
    on_failure: fail
    with:
      action: publish
      source: ./dist
      repository: backups
      artifact_name: backups/app
      version: build-123
      force: false
```

Read/delete step actions use `ref`; pull uses `destination` for CLI `--output`; copy uses `from`
and `to`. Preserve existing templating, working-directory, retry, conditional, and failure behavior
through the [step hook bridge](./hooks-step-types.md). Return typed operation results and named
outputs, including reference, resolved location, digests, size, and outcome. Retry context must
retain publication identity and the captured timestamp.

Do not introduce a separate artifact hook implementation. Existing `type: publish` delivery behavior
and [archive steps](./archive-step.md) keep their semantics; reuse archive primitives internally.

## CI compatibility and job summaries

Artifact operations must produce polished Markdown job summaries when CI reporting is enabled.
Use the existing `ci.Reporter`, summary feature gates, provider routing, masking, and template
loader, following the [container reporting integration](./native-ci/providers/container.md).
Honor `ci.enabled`, `ci.summary.enabled`, and existing explicit CI-mode overrides. Do not write
directly to provider-specific environment files from artifact code.

```yaml
ci:
  enabled: true
  summary:
    enabled: true
  templates:
    base_path: .atmos/ci/templates
    artifacts:
      publish: artifacts/publish.md
      copy: artifacts/copy.md
```

`ci.templates.artifacts` is a proposed operation-to-template map. Resolution follows explicit
override, `<base_path>/artifacts/<operation>.md`, then embedded default. An invalid explicit
override produces a reporting diagnostic rather than silently choosing a different template.
Reporting remains best-effort and cannot turn a successful transfer into a failed operation.

The default presentation includes a heading and compact results table with operation, outcome,
artifact name/version, repository, destination, digest, human-readable size, and duration.
Use short display digests with full values in expandable details; identify their digest type.
Include useful links only where a backend can supply a valid viewer/download location; otherwise
show a copyable reference. Do not emit credentials or signed URLs into summaries.

Illustrative result rows (additional destination and integrity details appear below the table):

| Operation | Outcome | Artifact | Repository | Size | Duration |
| --- | --- | --- | --- | --- | --- |
| Publish | Published | `components/terraform/vpc:1.2.0` | `shared` | 48 KiB | 1.2 s |
| Publish | Unchanged | `scaffolds/landing-zone:2.0.0` | `shared` | 96 KiB | 0.3 s |
| Publish | Failed | `backups/app:build-123` | `backups` | — | 2.1 s |

Distinguish published, unchanged, replaced, downloaded, copied, deleted, and failed outcomes.
Read operations report inspected/listed results without claiming a payload transfer. Collapsible
sections show metadata, selected labels, file counts, document availability, and useful failure
diagnostics. Do not embed entire package files or changelogs by default.

One shared renderer consumes operation results for CLI, workflow, and hook entry points.
Assign reporting ownership at the outer execution boundary so hook bridging and nested steps
do not emit duplicate summaries. Aggregate concurrent work in deterministic order and serialize
writes; retain completed operations if a later operation fails. Retries produce one final outcome
per operation, with retry information in details.

Escape Markdown content, mask secrets, and truncate to provider limits with a visible omission
notice. Summary rendering/delivery failures log a diagnostic and preserve the operation's exit
status. Disabled summaries perform no additional metadata fetches. Preserve unrelated existing
job-summary content. Structured stdout remains clean; local presentation uses existing stderr/UI
behavior. PR comments and status checks are separate features and are not implicitly enabled.

## Shared implementation and compatibility

Build a common artifact service over registered repository adapters with explicit capabilities:
resolve/inspect, publish, pull, list, delete, document reads, and optional native copy. Keep backend
options and authentication separate from producer-specific validation and packaging.

Refactor [shared OCI operations](./plugins/platform/oci-reuse.md) around
[`oras-go/v2`](https://github.com/oras-project/oras-go). The plugin implementation selected
`go-containerregistry` to reuse the existing stack; this migration is deliberate convergence, not
a second independent OCI client in artifact commands. Replace library-specific public types with
shared descriptors/references and migrate callers behind compatibility entry points.

Preserve raw manifests/indexes, blobs, digests, media types, platforms, signature/referrer transport,
verification policy, and plugin immutability. Retain credential precedence, identity failure
behavior, private CA handling, cancellation, and independent source/destination credentials.
Existing plugin packages, catalogs, signatures, and locks must remain readable without rewriting.
Do not remove `go-containerregistry` until remaining image-specific consumers have been audited.
No Skopeo subprocess or new `containers/image/v5` transport is required by this proposal.

For generic artifacts copied across packaging backends, preserve verified logical content and
metadata while reporting any changed representation checksum. For existing OCI graphs transferred
through S3/local storage, preserve a standard OCI-layout representation, including referenced blobs;
restore it without rewriting manifests. Do not flatten container layers into an ordinary file tree.

Adapt [native CI artifact storage](./native-ci-artifact-storage.md) to the common substrate instead
of duplicating its S3/local clients. Preserve existing planfile store config, key patterns, identity
resolution, metadata, selection rules, and archive compatibility. General metadata must not require
CI-only fields. Fix mode loss when adapting file bundles so executable tools/scripts survive.
Existing GitHub artifact storage remains CI-scoped; it is not a durable general repository promise.

## Delivery and acceptance

Deliver in stages: shared OCI migration with conformance coverage; repository service and local/S3
adapters; command/step/hook surfaces; metadata/documents and discovery/catalogs; producer integrations
and CI reporting. Each stage must preserve existing consumers. Runtime implementation must add
schema definitions, CLI help, and website documentation; this PRD change adds no runtime behavior.

Acceptance scenarios:

1. Configure one repository, then publish multiple components, scaffolds, and generic files without
    adding repository entries. Repeat against OCI, S3, directories, and archives using the same CLI.
2. Round-trip payloads and documents across backends; compare content digests and file modes.
    Transfer an OCI graph through a layout and a second registry with identical manifest digests.
3. Read metadata and README/CHANGELOG with payload downloads forbidden by the test transport.
    Recover metadata from a standalone archive and explicitly account for the required scan.
4. Verify unchanged publication is a no-op, different bytes require force, and plugin releases
    reject replacement. Exercise interrupted writes, conflicting names, staging, and retries.
5. Read dated publications on a later day; confirm retry stability, source/destination anchoring,
    path containment, archive permission bits, and executable payload fidelity.
6. Filter version-specific labels across repositories. Test catalog-free discovery where supported,
    explicit-name OCI lookup, unsupported collection enumeration, and catalog-based discovery.
    Publication must leave catalogs unchanged. Verify offline cache and auth-scope behavior.
7. Publish a scaffold without changing generated README behavior; consume a component through
    existing provisioning; install a verified tool package with its upstream unavailable.
8. Run equivalent CLI, workflow, and hook operations with matching results, retry semantics, and
    exactly one CI report. Verify reporting gates, template overrides, secret masking, Markdown
    escaping, size limits, concurrent aggregation, no-op results, and partial failures.
9. Preserve existing CI planfile fixtures, plugin release/catalog/lock compatibility, signature
    verification, and source/destination auth isolation through the ORAS migration.

Use unit tests with injected transports and generated mocks, temporary local repositories, the
existing in-process OCI registry harness, and deterministic S3 fakes. Opt-in emulator round trips
must seed a source, publish to a destination, disable the source, and restore into fresh state.
Never substitute successful metadata lookup for proof that offline payload restoration works.

## Deferred requirements preserved from the earlier draft

These remain intended follow-up capabilities, not additional V1 command/configuration promises:

- Bulk image mirror/sync: standalone and component-driven selection, explicit host/all/selected
  platform policies, source-host-preserving names, tag preservation, promotion, and collision rules.
- Bundle integration: select an exact dependency closure, mirror locally, export/import for air-gap
  use, and verify restoration without upstream access. A bundle is distinct from one archive artifact.
- Vendoring convergence: compile existing vendor sources into source-artifact operations while
  preserving hydration targets, include/exclude filters, version resolution, and provenance.
- Additional Git-backed, HTTP archive, GitHub release, and CI-scoped repository adapters; retain
  native Git delivery semantics and document each backend's capability and retention boundaries.
- Terraform registry caches: export/restore individual provider/module records and required indexes
  by origin, version, platform, and digest; verify restored mirrors and module resolution offline.
- Toolchain installed-tree snapshots and resolver/index distribution for complete offline tool
  discovery. V1 verified upstream package storage must remain independently useful.
- SBOMs and attestations: subject-digest associations, preserve/require/ignore transfer policy,
  CycloneDX/SPDX documents, pluggable generation, and aggregate dependency-closure inventories.
  Existing plugin signature requirements are compatibility obligations now, not deferred work.
- Repository statistics, retention/prune/garbage-collection policies, advanced signing/encryption,
  and daemon/container-storage transports. V1 explicit reference deletion remains in scope.
- Structured changelog parsing, version-range release-note aggregation, and template-update PR
  enrichment using the package documents introduced here.

No historical singular `artifact.repositories`, per-image publish-target schema, or Skopeo-specific
flags in the old draft are introduced as compatibility aliases by this proposal. Future extensions
must build on the collection-root model and plural `atmos artifacts` command space defined here.
