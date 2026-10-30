# Atmos Artifacts implementation plan

**Status:** Ready for implementation; all work packages below are unstarted.
**Product contract:** [Atmos Artifacts PRD](../artifacts.md).
**Companion specifications:** [Contracts and storage](./contracts.md), [validation and handoff](./validation.md).

## Instructions for the implementing agent

Implement the work packages in dependency order. Finish the acceptance checks for one package
before marking it complete. A package is not complete because its interfaces compile or its mocks
pass: demonstrate the described behavior through its real adapter or deterministic test harness.
Keep the public names and compatibility guarantees in the PRD. This plan fills in implementation
choices; it does not authorize implementing the PRD's deferred features as part of V1.

Start by reading this plan and both companions, then inspect the indicated production code.
Recheck current repository instructions and branch state. Preserve concurrent changes. Build the
workspace binary with `atmos build` before using repository hooks; run subsequent commands with
the workspace `build` directory first on PATH. Do not disable hooks, signing, or verification.

Use small changes that compile independently. Use registered backends, CommandProvider commands,
standard flag parsing, functional options, static error sentinels, context cancellation, and
generated `go.uber.org/mock` mocks. Keep configuration/dependencies in explicit structs, not context
values. Keep package files below the repository's size limit. Add no artifact business logic to
the CLI or a second implementation in hooks.

Keep a handoff record listing completed package IDs, commits, exact commands/results, remaining
failures, and the next package. Do not mark all of V1 complete while a producer, backend, document
read path, or CI surface listed here is missing.

## Current integration map

These are existing entry points to inspect, not a proposed file-by-file rewrite.

| Area | Current entry points | Constraint |
| --- | --- | --- |
| OCI transport | `pkg/oci/client.go`, `identity.go`, `credentials.go`, `referrers.go` | Registry methods expose go-containerregistry types; auth and referrers are already shared |
| OCI layouts/materialization | `pkg/oci/layout.go`, `pull.go`, `selection.go`, `verified.go` | Existing layout helpers expect one root release index; collections need an additional API |
| OCI consumers | `pkg/plugin/oci`, `pkg/plugin/catalog`, `pkg/plugin/discovery`, `pkg/oci/signature`, `pkg/version/resolver/oci` | Preserve release bytes, locks, policies, and immutable plugin publication |
| File consumers | `pkg/generator/source/resolver.go`, `pkg/vendoring/install/fetch.go`, `pkg/provisioner/source/vendor.go` | Preserve existing source resolution and extraction |
| CI artifacts | `pkg/ci/artifact/{backend,store,bundled_store,metadata,tar}.go` | Existing metadata validation requires stack/component/SHA; file entries omit modes |
| CI backends | `pkg/ci/artifact/local`, `s3`, `github` | Existing local/dir stores a bundled stream, not an ordinary copied directory |
| Planfiles | `pkg/ci/plugins/terraform/planfile/adapter/store.go` | Existing store selection, key patterns, metadata, and integrity remain compatible |
| Configuration | `pkg/schema/schema.go`, `pkg/config/imports.go`, `pkg/config/load.go` | Preserve declaring-file origins for destination paths through config merges |
| Schema assets | `pkg/datafetcher/schema/atmos/config/1.0.json`, `stacks/stack-config/1.0.json`, `scaffold/scaffold-config/1.0.json` | Update the surfaces that actually admit the new configuration/steps |
| Steps/hooks | `pkg/schema/workflow.go`, `pkg/schema/task.go`, `pkg/runner/step/{registry,types,variables}.go`, `pkg/hooks/step_engine.go` | Step name, existing Artifacts declarations, and output display settings already have meanings |
| Packaging | `pkg/archive/{archive,walk,tar,zip,mtime}.go` | Reuse archive primitives; do not change existing archive-step defaults |
| Scaffold | `cmd/scaffold`, `pkg/generator`, `pkg/generator/source` | Preserve root README and generated-file behavior |
| Toolchain | `cmd/toolchain/install.go`, `pkg/toolchain/installer/{installer,download,asset,extract}.go`, `pkg/toolchain/lockfile` | Cached filename alone is not identity; URL sidecars and verification matter |
| Reporting | `pkg/ci/reporter.go`, `mode.go`, `templates/loader.go`, `pkg/component/container/imagesummary` | Existing gates, providers, template conventions, and masking are authoritative |

## Dependency order and delivery units

| ID | Deliverable | Depends on | Completion evidence |
| --- | --- | --- | --- |
| A00 | Compatibility fixtures and contract types | None | Baseline fixtures and digest vectors |
| A01 | Config/schema/registry foundation | A00 | Import-origin and validation tests |
| A02 | Shared ORAS transport and legacy adapters | A00 | Existing consumers pass conformance |
| A03 | Preparation, packaging, operation service | A00, A01 | Determinism, streaming, and conflict tests |
| A04 | Local directory/archive repositories | A03 | Real filesystem round trips |
| A05 | Shared blob I/O and S3 repository | A03, A04 | S3 and legacy CI adapter suites |
| A06 | OCI repositories and signature/lock parity | A02, A03, A04, A05 | Signed graph round trips and shared contract tests |
| A07 | Public artifact commands | A04, A05, A06 | CLI contract tests and documentation |
| A08 | Discovery/cache/catalogs | A02, A04, A05, A06, A07 | Offline and catalog independence tests |
| A09 | Step and hook integration | A03, A07 | CLI/step/hook parity |
| A10 | CI summaries and outputs | A07, A09 | One report per operation, including failures |
| A11 | Scaffold/component producers | A03, A06, A07 | Existing consumer integration tests |
| A12 | Verified toolchain producer/consumer | A03, A05, A06, A07 | Cold offline installation with verification |
| A13 | Native CI repository bridge and release gates | A05, A09, A10, A11, A12 | Planfile compatibility and full acceptance matrix |

A02 and A01 can be developed independently after A00. Backend work can overlap after A03's
contracts are stable. No parallelism is required. Keep review units under the repository's file
limit; a work package can take several PRs. Behavior-preserving foundations use `no-release`;
each PR exposing new public behavior follows the repository's `minor` release documentation rules.

## A00 — Freeze compatibility and introduce domain contracts

- [ ] Inventory all direct go-containerregistry imports and network operations in current consumers.
  Record which are transport, image construction, extraction, or verification; do not remove the
  dependency while image-specific callers still need it.
- [ ] Add `pkg/artifact` for generic domain models, typed requests/results, repository registration,
  and service orchestration. Do not reuse `schema.Artifacts`, which describes step output files.
- [ ] Establish the record, reference, operation, and capability contracts in [contracts](./contracts.md).
  Add canonical serialization vectors before implementing any backend writer.
- [ ] Define shared Sign, Verify, Pin, and Lock contracts, evidence subjects, policy results, and
  lock guarantees. Signatures and locks must not be implemented only as OCI-specific options.
- [ ] Capture deterministic fixtures for a plugin release index, its manifests/blobs, a plugin
  catalog, signature material, signed native images, legacy CI metadata/tar, and single-root OCI layouts.
  Preserve captured bytes as testdata; do not regenerate them to hide incompatible changes.
- [ ] Add shared sentinels for invalid config/reference, conflict, not found, unavailable publication,
  unsupported capability, integrity failure, missing document, invalid packaging, and unsafe path.
  Preserve existing sentinel wrapping in compatibility adapters.

**Done when:** Every changed public contract has a test fixture, and a compatibility test can detect
a changed plugin manifest digest or planfile archive even before the implementation is replaced.

## A01 — Configuration and backend registry

- [ ] Add `ArtifactConfig` and `ArtifactRepositoryConfig` to `pkg/schema`, attached as
  `AtmosConfiguration.Artifacts`. Include repositories, kind, identity, options, packaging,
  permissions, optional catalog, and defaults.metadata; keep backend option decoding typed.
- [ ] Add global/repository verification policy and repository locking configuration from contracts.
  Validate trust restrictions, delegated lock identities, cycles, and required lock scopes.
- [ ] Register repository factories by kind in `pkg/artifact`. Return capability descriptors without
  network access. Reject duplicate kinds and unknown kinds; avoid a central backend switch.
- [ ] Normalize local destination paths while loading the declaring config, before imported values
  lose their source directory. Track the origin of `options.path` independently of unrelated merged
  repository fields. Do not expand key-pattern templates during general config rendering.
- [ ] Validate root/prefix containment, packaging matrix, octal strings, label values, catalog
  references, and required backend options. Reject `permissions` for non-local archives.
- [ ] Resolve identity bindings lazily when a real operation requires credentials; no credentials
  or destination directories should be created merely by describing config or running dry-run.
- [ ] Update embedded config JSON Schema and relevant stack/scaffold step schemas together with
  Go types. Verify YAML, JSON, mapstructure, imports, and config describe round trips.

**Done when:** The PRD's repository examples load unchanged, two same-named components can share one
collection under different logical paths, and imported local roots resolve to their declaring files.

## A02 — ORAS transport behind shared OCI interfaces

- [ ] Promote the existing pinned `oras.land/oras-go/v2` dependency to direct use; do not combine
  this migration with an unrelated version upgrade. Consult documentation for that pinned version.
- [ ] Add OCI-standard descriptor/reference/blob interfaces alongside the existing Registry API.
  Use `opencontainers/image-spec` descriptors and `go-digest`, not ORAS remote.Repository or
  go-containerregistry images in the new public transport boundary.
- [ ] Implement resolve, fetch, exists, push, tag, list tags, delete manifest, graph copy, layout
  storage, and referrer discovery using ORAS. Keep bounded reads/digest verification at the boundary.
- [ ] Adapt existing authentication resolution to ORAS credentials. Preserve explicit identity
  failure behavior, ambient precedence, host/realm scoping, token refresh, and TLS/proxy options.
  Source and destination clients must not share a credential cache across different identity scopes.
- [ ] Bridge old Registry methods onto the new transport while legacy callers still construct
  go-containerregistry images/indexes. Serialize their existing raw bytes; never rebuild an old
  release with ORAS PackManifest just to transfer it.
- [ ] Migrate plugin release/catalog reads and writes, discovery, version resolution, signatures,
  and shared source pulls in small groups. Preserve `ResolveImage`, `PinDigest`, and `ProcessImage`.
- [ ] Add multi-reference layout operations without weakening `LoadIndex`'s existing single-release
  validation. Preserve OCI 1.1 referrers and the existing fallback behavior required by signatures.
- [ ] Reconcile writes by resolving the destination after ambiguous failures; do not blindly retry
  mutations or downgrade to anonymous authentication. Cancellation must stop graph traversal.
- [ ] Extract reusable verifier/signer primitives into `pkg/signature`, retaining compatibility
  wrappers and existing plugin trust requirements. Keep attachment transport in the adapters.

**Done when:** All A00 OCI fixtures retain their digest and verification result through registry →
layout → second registry movement with distinct credentials and the source switched off.

## A03 — Artifact preparation and shared operation service

- [ ] Implement a producer registry. Start with generic files; kind-specific producers add validation
  and exclusions later. Preparation resolves name/version, labels, documents, inventory, and digests.
- [ ] Snapshot input into bounded disk staging before committing publication. Detect files changing
  during capture and fail instead of publishing an inventory that describes different bytes.
  Close readers and remove owned temporary data on success, error, or cancellation.
- [ ] Reuse archive traversal/writers with explicit deterministic options: sorted entries, normalized
  incidental timestamps and ownership, preserved ordinary modes, and safe symlink handling.
  Existing `type: archive` behavior remains unchanged.
- [ ] Recognize the reserved document paths and explicit overrides. Validate explicitly requested
  files, compute document descriptors, and attach generated metadata without checksum recursion.
- [ ] Build publish/pull/copy/inspect/list/search/delete/show orchestration over Repository interfaces.
  Pin mutable references once, check capabilities before mutation, and return partial operation
  results alongside errors so reporting can describe completed work.
- [ ] Implement intent comparison/no-op, force conflicts, plugin immutability admission, operation
  timestamps, publication journals, and retry reconciliation from the contracts companion.
- [ ] Call repository Sign/Verify/Pin/Lock methods through common orchestration. Enforce source and
  destination trust requirements before copy/consumption; preserve evidence for the actual subject.
  Reject insufficient lock guarantees and retain explicit ownership across retries.
- [ ] Implement extraction staging and replacement. Reject traversal and escaping symlinks; never
  merge a newly pulled tree over old files or delete files outside the explicit output destination.
- [ ] Expose dry-run preparation results without calling repository mutation/auth operations.

**Done when:** One service test suite drives all entry points; unchanged input yields the same
content identity, unchanged publication is a no-op, and failed reads/writes never report success.

## A04 — Local directory and archive repositories

- [ ] Implement separate `local/dir` and `local/archive` factories in `pkg/artifact/local`.
  Do not redirect the existing CI `local/dir` name to the new copied-directory behavior.
- [ ] Use stable-root reference records, immutable metadata records, and adjacent document files
  as specified in contracts. Directory payloads remain ordinary copied files; archives contain
  one artifact and have adjacent metadata/documents plus embedded portable copies.
- [ ] Hold per-reference and resolved-location locks over comparison, preparation, and commit.
  Publish completion last; use same-filesystem staging and atomic file replacement where possible.
- [ ] Persist dated resolved locations. A subsequent read/delete must not rerender the key pattern
  with a new timestamp. Detect two references attempting to own the same resolved path.
- [ ] Apply quoted octal archive-file permissions separately from payload modes. Verify behavior
  on Unix and document the platform capability error for explicitly unsupported requirements.
- [ ] Implement metadata-only inspect/show, record enumeration, verified digest reads, and scoped
  deletion. Keep private managed records out of the user's pulled payload.
- [ ] Implement detached signature storage, shared signer/verifier delegation, immutable pinning,
  and filesystem LockHandles. Signature additions must not mutate their signed Record.

**Done when:** Real temporary-directory tests cover every allowed packaging format, dated roots,
ordinary executable files, safe links, interrupted writes, output replacement, and missing sidecars.

## A05 — Share blob transport with CI and add S3

- [ ] Extract transport-only local/S3 object I/O from `pkg/ci/artifact` into `pkg/artifact/blob`.
  The low-level API handles streams, size, headers/tags, conditional writes, listing, and deletion;
  it must not require CI Metadata. Keep legacy metadata encoding in CI adapters.
- [ ] Preserve existing CI factories and wire formats using adapters over that transport. Keep
  GitHub artifact storage CI-specific and avoid a reverse dependency from artifact core to CI.
- [ ] Implement `aws/s3` with archive payloads or per-file objects, stable-root records, document
  objects, configured prefix/key_pattern, and explicit object tags. Copy the configured tags to
  all objects in the publication; portable labels remain independent.
- [ ] Use streaming/multipart transfers and bounded worker concurrency. Abort multipart uploads
  owned by a canceled operation. Handle pagination, zero-length files, and endpoint overrides.
- [ ] Use S3 conditional writes for completion/reference updates where supported. An endpoint that
  cannot enforce a needed condition must fail that operation clearly, not silently overwrite.
- [ ] Implement digest-addressed evidence objects, Pin, and persistent owner-checked S3 lock claims.
  Keep crash recovery explicit; no automatic lease expiry without fencing and no Object Lock confusion.
- [ ] Preserve lazy identity resolution and endpoint/TLS behavior. New generic repository identity
  failures must fail closed; do not copy the legacy missing-resolver ambient fallback into it.

**Done when:** An S3 fake proves request ordering, tags, pagination, and conditional conflicts;
an opt-in S3-compatible emulator proves archive/files round trips and existing planfile compatibility.

## A06 — OCI collection repositories and foreign graphs

- [ ] Implement registry and layout factories over A02. Map root/prefix/name/version consistently;
  expose capability differences without requiring users to configure each artifact name separately.
- [ ] Assemble new Atmos manifests with the PRD media types, config metadata, deterministic payload
  descriptors, and separate README/CHANGELOG blobs. Supply the captured creation time explicitly.
- [ ] Support selective config/document fetches and multi-reference layout enumeration. Never fetch
  payload layers merely to inspect labels or show a document.
- [ ] Implement OCI content-graph import/export and cross-backend transport through a standard OCI
  layout payload. Keep original manifests and related required signature material byte-identical.
- [ ] Distinguish generic metadata from foreign OCI image/plugin schemas. Inspect foreign objects
  without pretending they have an Atmos config blob; do not repackage them on copy.
- [ ] Implement the same Sign/Verify/Pin/Lock contract as local/S3, using OCI attachments and the
  shared verifier. Bind image signatures to the original manifest/index; bind portable Record
  signatures through the addressed config descriptor. Support delegated repository-scope locks.
- [ ] Add genuine keyless and configured-public-key verification tests. Missing, tampered,
  wrong-subject, wrong-issuer, untrusted-key, and expired/missing offline trust must fail policy.
- [ ] Enforce plugin-format immutability for generic publication/copy before a tag can be overwritten.
  Preserve caller policy/verification checks on every read and write path.
- [ ] Implement safe deletion semantics: no silent deletion of sibling tags that share a manifest.
  Report unsupported tag-only deletion when the registry cannot provide the required semantics.

**Done when:** Generic artifacts and legacy plugins coexist beneath one collection prefix, a
multi-platform image survives S3/layout transfer, and metadata/document-only request budgets pass.

## A07 — Command space

- [ ] Add a `cmd/artifacts` CommandProvider with publish, pull, copy, inspect, list, search, delete,
  sign, verify, and show readme/changelog subcommands through the existing command registry.
- [ ] Implement the exact flag/step mapping in contracts using StandardParser and normal Atmos
  configuration precedence. Reuse list/table formatting and the existing label parser.
- [ ] Keep data on stdout, progress on stderr, and errors in the established structured error path.
  Emit result envelopes on failure when structured output is requested; retain a nonzero exit code.
- [ ] Embed command usage Markdown and add website command/configuration pages. Label all new
  functionality accurately; the PRD is not a substitute for shipped command help.
- [ ] Test with `cmd.NewTestKit(t)` and injected service dependencies; add real temporary-local CLI
  tests so command validation, serialization, and service wiring are exercised together.

**Done when:** Every PRD command example has a tested counterpart and a documented error path;
no command invents a default repository, `latest`, catalog mutation, or interactive CI prompt.

## A08 — Discovery cache and optional catalogs

- [ ] Extract reusable source-scoped snapshot/cache mechanics from `pkg/plugin/discovery` into a
  neutral `pkg/discoverycache` package; retain plugin-specific schemas, keys, defaults, and policy.
  Prove plugin cache compatibility before switching its consumer to shared mechanics.
- [ ] Enumerate managed local/S3 records and OCI-layout refs. For OCI registries use supported
  repository enumeration scoped to prefix, or the configured catalog; explicit-name tag listing
  works without either. An unsupported collection scan fails, not an empty successful result.
- [ ] Apply exact-match AND label filtering to published-version metadata and substring matching
  to name/description. Sort results deterministically and retain repository alias and pinned digest.
- [ ] Implement the 15-minute cache, explicit refresh, offline semantics, credential-scope keys,
  policy reevaluation, bounded reads, and atomic local snapshot replacement from the PRD.
- [ ] Add a separate `AtmosArtifactCatalog` manifest using `pkg/manifest`. Carry artifact kind,
  source, versions, digests, labels, and document descriptors; do not impose plugin SemVer rules.
- [ ] Add catalog build/publish/add/remove/sync commands specified in contracts. Catalog writes
  require one externally serialized publisher; release publication never invokes these commands.
- [ ] Treat catalog payloads as discoverable data, never trust grants. Validate every referenced
  location against the destination repository and current auth/source policy before access.

**Done when:** Multiple configured repositories search together, cache hits fetch no payloads,
catalog changes never rewrite artifacts, and incomplete/unauthorized results are never presented as complete.

## A09 — Workflow and hook integration

- [ ] Add `ArtifactHandler` to `pkg/runner/step`; implement Validate, Execute, KnownFields, and
  RequiresTTY=false. Use flat step fields and the existing kind: step hook bridge.
- [ ] Add shared schema fields without repurposing `name`, `kind`, `output`, or `Artifacts`.
  Cover WorkflowStep, Task conversion, custom-command decoding, script step dispatch, and hook `with`.
- [ ] Resolve source/destination paths through the runner's existing working-directory helpers;
  resolve repository roots independently from their config origin.
- [ ] Return typed operation results in a documented StepResult.Metadata entry and strings in
  named Outputs; use the same payload as CLI JSON. Return partial results even on service error.
- [ ] Retain a preparation/operation identity across wrapper-level retries so dates, versions, and
  reports do not change on every attempt. Do not multiply the runner and transport retry budgets.
- [ ] Exercise before/after Terraform hooks, workflow execution, custom commands, and direct script
  dispatch. Preserve conditions, continue/on_failure, and cancellation semantics.

**Done when:** The same publication produces the same metadata and result across CLI, workflow,
and hook calls, including retry/failure cases; no shelling out to `atmos artifacts` is required.

## A10 — CI summaries and machine outputs

- [ ] Add `pkg/artifact/report` as a renderer over typed results, with no repository mutations or
  extra inspection requests. Add embedded templates and CITemplatesConfig.Artifacts overrides.
- [ ] Use the existing template loader's explicit/convention/default order, with an error for a
  missing explicit override. Route completed Markdown through `ci.Reporter.Summary`.
- [ ] Collect at execution boundaries: CLI owns standalone reports; the runner owns step reports
  after retries; hooks reuse that runner collector and retain an empty duplicate hook body.
  Pass collectors explicitly through runner services/options, never through context values.
- [ ] Deduplicate by logical operation ID and retain the final attempt. Buffer concurrent results,
  order by scheduled step/operation position, and flush from a deferred finalizer on error too.
- [ ] Render outcomes, destinations, digest types, sizes, duration, and collapsible details. Escape
  Markdown, mask secrets, avoid signed URLs, and enforce provider byte limits with visible truncation.
- [ ] Include signature subject, verified signer/key claims, verification status, and actual lock
  guarantee. Distinguish signed from verified and a content checksum from publisher authentication.
- [ ] Wire `ref`, `location`, digest fields, size, and outcome to existing CI output mechanisms when
  enabled. Keep JSON/YAML stdout valid and leave PR comments/checks opt-in through their own features.

**Done when:** Success, no-op, force replacement, partial failure, canceled siblings, disabled
reporting, template failure, and provider failure all have deterministic tests and correct exit status.

## A11 — Scaffold and component publishing

- [ ] Add a scaffold producer and `atmos scaffold publish` provider delegating to the generic service.
  Validate scaffold configuration without generating output or executing scaffold hooks.
- [ ] Add a component/terraform producer with visible default exclusions for `.terraform/**`,
  `*.tfstate`, `*.tfstate.*`, `*.tfplan`, and `*.tfplan.*`; retain `.terraform.lock.hcl` and scripts.
  Preserve explicit dry-run inventory and do not apply these exclusions to generic backups.
- [ ] Exclude reserved package metadata from generated scaffold output; preserve root README
  rendering and user-selected ordinary payload files. Cover existing generator source strategies.
- [ ] Teach existing OCI materializers to select the Atmos payload descriptor rather than trying
  to extract every layer, while keeping legacy component/scaffold layer heuristics intact.
- [ ] Prove published components work through existing source provisioning and scaffolds through
  existing generation/update flows, including documentation without pulling the payload.

**Done when:** A component with an executable helper and a scaffold with both README roles round-trip
without leaked distribution files or altered generated output. Other configuration/module trees
remain publishable as generic files; native cache/export adapters are not silently added here.

## A12 — Verified toolchain package integration

- [ ] Add `atmos toolchain publish` and explicit artifact-repository installation selection as
  specified in contracts. Reuse tool resolution and lockfile validation; do not introduce another
  tool registry format or automatic upstream fallback.
- [ ] Package the original upstream asset with its resolved install definition, platform identity,
  relevant lock entry, and available checksum/signature evidence. Retain original asset bytes.
- [ ] Verification metadata is evidence to recheck, not a boolean trust grant. Require pinned
  checksums and rerun applicable verification policy before publication and installation.
- [ ] Inject an asset source into the existing installer before it consults network registries.
  Feed the verified original package to normal extraction/install/PATH logic. Update URL cache
  sidecars transactionally and retain the upstream identity separately from artifact location.
- [ ] Keep adapter wiring outside artifact core and installer core to avoid cyclic imports.
  Fail explicitly when an offline verification dependency or install definition is absent.

**Done when:** With empty toolchain caches, blocked upstream HTTP, and a pinned local/OCI/S3
artifact source, install the fixture tool and execute it; a tampered asset or incompatible platform fails.

## A13 — CI compatibility bridge and release completion

- [ ] Add explicit repository selection to native CI planfile storage without changing legacy
  `planfiles.stores` selection. Follow the selector contract; never guess a repository from a
  missing legacy store or silently fall back after download-not-found.
- [ ] Adapt CI bundle metadata to generic records while preserving old metadata validation and
  serialized sidecars for existing callers. Add file-mode fields with a legacy-compatible default;
  preserve existing planfile tar bytes when no mode is supplied.
- [ ] Complete all backend/entry-point scenarios in [validation](./validation.md), including restore
  with upstream disabled, negative auth tests, and CI result parity.
- [ ] Update CLI/config/hook docs, examples, schema tests, and generated mocks. Record the exact
  semantics of unsupported capabilities and offline data requirements.
- [ ] Run repository checks, targeted suites, full regression checks, and website build; inspect
  generated changes. Follow the release skill for labels, blog, roadmap, and milestone requirements.

**Done when:** The validation matrix is green for new behavior and compatibility, required docs are
published with the feature, and no deferred capability is presented as implemented.

## Follow-on work queue

Keep these outside V1 and link future designs back to the use-case inventory: Terraform provider
and module cache export/restore with mirror indexes; native stack/Atmos-config dependency closure;
installed toolchain snapshots; bulk image mirroring/platform selection; complete air-gap bundles;
additional transports; SBOM generation and aggregate inventories; retention/prune; structured
changelog aggregation. Generic file upload alone is not completion evidence for these integrations.
