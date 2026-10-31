# Atmos Artifacts validation and agent handoff

**Status:** Required future implementation checks; no box is marked complete by writing this plan.
Read the [work packages](./implementation.md), [contracts](./contracts.md), and [PRD](../artifacts.md).

## Test strategy and fixtures

Use small real files, temporary repositories, deterministic clocks, injected failures, and bounded
fake transports. Generate interface mocks with the repository's mockgen workflow. Favor observable
behavior over tests that repeat implementation details. Do not manually edit existing golden files.

Create reusable fixtures in the owning package testdata, not under unrelated existing golden trees:

- File package: nested files, an executable helper, empty file/directory, confined symlink, Unicode
  filename, root README, and distinct package README/CHANGELOG.
- Invalid package: traversal, symlink escape, duplicate normalized paths, conflicting reserved
  metadata, input changing during capture, and unsupported special-file entries.
- Generic Record: canonical byte vectors and expected content/intent/record digests, including
  alternate map insertion order and fixed creation times.
- OCI graph: tiny single-platform image and two-platform index with distinguishable child bytes.
- Legacy plugin: existing format manifests/index, executable fixture, catalog, lock, and genuine
  Sigstore evidence. Keep expected digest values stable through transport migration.
- Signed generic artifact/native image: distinct subject types, genuine trusted and untrusted
  signer fixtures, public-key-only evidence, incorrect issuer, and mismatched subject digest.
- CI planfile: pre-change tar and metadata plus old store/default/priority configurations.
- Toolchain: tiny original release archive/raw executable, two platform definitions, lock entries,
  URL-cache sidecars, checksum/signature evidence, and an executable version-reporting fixture.

Reuse `pkg/oci/ocitest` and `pkg/oci/signature/signaturetest` rather than live public registries or
identity providers. Fake S3 must model pagination, conditional state, multipart cancellation, object
tags, and response ambiguity; a map that always succeeds does not test publication correctness.
Opt-in emulators supplement these tests and must not be required for ordinary unit suites.

## Required conformance matrix

Run the shared repository conformance suite against each registered backend, with capability-specific
assertions for supported operations and explicit errors for unsupported guarantees.

| Behavior | Local directory | Local archive | S3 | OCI layout | OCI registry |
| --- | --- | --- | --- | --- | --- |
| Multiple names/versions per configured root | Required | Required | Required | Required | Required |
| Publish, pull, inspect, show, delete | Required | Required | Required | Required | Required within declared delete capability |
| Packaging | Copied files | tar/tgz/zip | Files and tar/tgz/zip | Packaged files and native graphs | Packaged files and native graphs |
| Metadata/document reads without main payload | Required | With sidecars | Required | Required | Required |
| Collection enumeration | Managed refs | Managed refs | Paginated managed refs | Layout refs | Native enumeration or catalog; error otherwise |
| Generic Sign/Verify/Pin | Detached evidence | Detached/portable evidence | Evidence objects | Layout evidence | Referrer evidence |
| Lock | Host filesystem | Host filesystem | Repository owner claim | Host filesystem | Host guard; repository scope through configured delegate |
| Missing required lock/evidence capability | Error | Error | Error | Error | Error |

Test local directory ↔ archive, local ↔ S3 in both packaging modes, local ↔ OCI, registry ↔ layout,
registry ↔ second registry, and native OCI graph ↔ S3/layout carrier. Byte-identical native graphs
retain resolved digests; generic repackaging retains Record/content identity but may change the
representation/OCI envelope digest. Assert both rather than assuming every digest is interchangeable.

## Acceptance scenarios

### Configuration and interface parity

- [ ] Load every PRD YAML example; validate identical values across YAML/JSON/mapstructure.
- [ ] Load two imported configs with different source directories and merged repository fields;
  each destination path uses the declaration that supplied that path.
- [ ] A remote config must not accidentally make an ephemeral download directory a write root;
  until local anchoring is explicit, require a locally declared/absolute destination override.
- [ ] Reject unknown kinds, invalid option combinations, traversal, catalog/identity mismatch,
  unquoted numeric permissions, malformed labels, and delegated lock cycles before mutation.
- [ ] Configuration inspection and dry-run perform no credential requests, destination directory
  creation, signature publication, or remote object writes.
- [ ] Reuse one alias for components, scaffolds, configuration trees, and generic backups.

### Publication, integrity, and recovery

- [ ] Repeated preparation produces canonical records/inventory and deterministic payloads.
- [ ] An identical republish is unchanged even when invocation time/run metadata differ.
- [ ] Changed content, labels, documents, or modes conflict without force; plugin releases remain
  immutable with force. Existing/incoming plugin formats both trigger the guard.
- [ ] Resolve a tag, then move it during a read/copy; consume only the originally pinned subject.
- [ ] Inject interruption before payload, during payload, after metadata, and after server-side
  commit before its response; retry reconciles correctly and never advertises partial success.
- [ ] Inject two writers for one ref and two refs with a colliding key_pattern. Verify ownership
  and conditional guards; never silently bind one reference to another artifact's payload.
- [ ] Date rollover and wrapper-level retries reuse captured time and resolved locations.
- [ ] Pull replacement removes stale destination files only inside the requested output root.
- [ ] Tampered metadata, payload, document, or OCI blob fails its declared integrity check.
- [ ] Direct digest reads never return replacement bytes under an old digest; unavailable retained
  records report failure. Scope deletion to the selected artifact and its exclusively owned files.
- [ ] Archive/file member permissions remain distinct; executable bits survive. Oversize records,
  unsafe links, unsupported target modes, and extraction errors are explicit.

### Signatures and coordination

- [ ] Every backend's Sign/Verify/Pin/Lock implementation passes the same interface tests; backend
  differences are represented by capabilities, not omitted operations that claim success.
- [ ] Sign a portable Record, copy it through local/archive/S3/OCI packaging, and verify the same
  signed subject after restoring it. Attachments bind to the correct record/config.
- [ ] Copy a signed native image/index through an offline carrier and a second registry, disable
  the source, and verify the original root digest and preserved evidence at the destination.
- [ ] Reject wrong digest, wrong issuer, untrusted identity/key, corrupt signature, absent required
  evidence, unsupported format, expired trust, and missing required transparency/signing-time proof.
- [ ] Public-key verification cannot satisfy an OIDC identity policy. Existing plugin rejection of
  public-key-only bundles under identity policy remains unchanged.
- [ ] Signing alone reports signed, not policy-verified; plain hashing reports integrity, not signer
  authenticity. `verify` without a usable trust configuration does not return a green result.
- [ ] Independent source/destination policies both apply to copy; repository overrides and force
  cannot weaken governing requirements. Cached verification results are reevaluated under new policy.
- [ ] Preserve a signed multi-platform root when selecting a child; a reconstructed root must not
  inherit an original signature claim. Required evidence is fetched before materialization.
- [ ] Offline verification uses only cached/provisioned trust and evidence, with a transport that
  fails any network request. Missing trusted roots cannot be filled from the artifact itself.
- [ ] S3 owner claims reject competing owners and wrong-token release; they never expire into an
  unfenced takeover. Local guards exclude competing processes and release when their owner exits.
- [ ] OCI host guards are not reported as repository-scope locks; a required distributed guard
  fails without a capable delegate. Delegated lock access uses its own identity.
- [ ] Canceled lock acquisition does not leak ownership; pending publication recovery requires
  valid ownership and matching intent. Document operator recovery of abandoned S3 claims.
- [ ] Pins persist immutable digests/evidence descriptors without changing existing consumer lock
  schemas or granting trust merely because the lockfile says a prior run succeeded.

### Discovery and catalogs

- [ ] Label pairs use AND semantics on artifact-version labels, not S3 tags or repository defaults
  retroactively applied to older artifacts. Keep aliases visible and sorting deterministic.
- [ ] Named OCI tag listing works without a catalog; unsupported collection discovery returns a
  capability error. Add no second repository list in configuration.
- [ ] Fresh-cache search performs zero registry requests; refresh reuses metadata by digest and
  removes disappeared refs only after a complete refresh succeeds.
- [ ] Offline cache works without payload downloads, rejects corrupt/missing snapshots and refresh,
  and does not cross auth realms/host bindings or bypass current policy.
- [ ] Catalog build reads metadata only. Publish/sign/delete of an artifact never implicitly edit
  its catalog. Catalog removal unlists without deleting content or rewriting locks.
- [ ] Concurrent catalog updates expose stale revision/digest conflicts; serialized workflows
  read latest inside the guard. Do not claim a preflight check alone prevents races.
- [ ] Plugin discovery/cache behavior stays compatible after common mechanics are extracted.

### Producers and existing consumers

- [ ] Scaffold generation/update preserves root README behavior and excludes reserved distribution
  metadata. `scaffold publish` validates without executing generation hooks.
- [ ] Component source provisioning reads new Atmos payload descriptors and old formats; helpers
  remain executable, Terraform transient data is excluded, and generic backups retain requested files.
- [ ] Toolchain publication verifies the original package against lock data. Offline installation
  succeeds with empty caches and blocked upstreams, using the normal extractor/installer.
- [ ] Reject toolchain tampering, wrong platform, missing install metadata/evidence, and cache URL
  collisions. Recheck configured verification; a recorded "verified" string is not sufficient.
- [ ] Existing CI stores retain metadata validation, tar defaults, key patterns, selection order,
  and not-found behavior. Only explicit `artifact/repository` opts into the new repository bridge.
- [ ] The planfile adapter preserves original key/query information and does not overwrite unrelated
  generic or plugin artifacts. Existing CI fixtures continue reading through their old adapters.

### CLI, steps, hooks, and summaries

- [ ] Run matching operations through CLI, workflow, custom command, script dispatch, and kind: step
  hooks; verify identical typed results, named outputs, source paths, and repository roots.
- [ ] Retry, on_failure/continue, cancellation, and failure after some completed operations preserve
  operation identity and report the correct final outcome exactly once.
- [ ] Summary rendering covers published, unchanged, replaced, downloaded, copied, deleted, signed,
  verified, and failed outcomes; includes actual signature subjects and lock guarantees.
- [ ] Sequential/parallel/nested steps produce one deterministically ordered report per operation;
  the hook bridge does not render a second copy. Existing unrelated job summary content remains.
- [ ] CI disabled, summaries disabled, output variables disabled, and explicit CI-mode overrides
  honor existing gates. Disabled summaries perform no extra metadata/evidence fetches.
- [ ] Custom template precedence, missing override errors, secret masking, Markdown escaping,
  provider size limits, and safe links work without corrupting JSON/YAML stdout.
- [ ] Reporting errors preserve artifact exit status; operation/verification errors remain failures.

## Commands and validation gates for implementation

Run package-scoped checks after each change; the following package paths include proposed packages
and are executable once those packages exist. Do not run unimplemented paths and count them as skipped.

```shell
go test -short ./pkg/artifact/... ./pkg/signature/...
go test -short ./pkg/oci/... ./pkg/plugin/oci/... ./pkg/plugin/catalog/... ./pkg/plugin/discovery/...
go test -short ./pkg/ci/artifact/... ./pkg/ci/plugins/terraform/planfile/...
go test -short ./cmd/artifacts/... ./cmd/scaffold/... ./pkg/generator/...
go test -short ./pkg/runner/step/... ./pkg/hooks/... ./pkg/ci/...
go test -short ./pkg/toolchain/... ./cmd/toolchain/...
go test -race -short ./pkg/artifact/... ./pkg/discoverycache/...
atmos build
atmos lint --changed
atmos test
```

Also run the website build for shipped command/config docs and the opt-in OCI/S3 emulator round
trips. Use current repository instructions for regeneration and CI checks. Target the required
patch coverage with behavioral tests; do not add redundant tests solely to inflate coverage.

Known prior workspace observations, not acceptance waivers: a documentation-only baseline build
passed, while a broad test invocation encountered missing Packer, plugin/formatting failures and
a package timeout. Recheck the current baseline before implementation, record exact failures, and
distinguish environment/baseline failures from regressions. New artifact tests must still pass.

## Completion record for the next agent

For each A00–A13 package, record:

- [ ] Implementation commit(s), changed interfaces, and migration/compatibility notes.
- [ ] Exact successful test/check commands, relevant fixtures, and skipped emulator prerequisites.
- [ ] Failure reproduction, baseline comparison, and owner for any unresolved check.
- [ ] Public docs/schema/help changes and release-label implications where applicable.
- [ ] Remaining work package IDs and the next concrete action.

V1 is complete only when every required backend and public surface passes this matrix and all
claimed offline flows work with fresh local state and unavailable upstreams. An upload success,
metadata search result, detached signature file, or apparent lock object alone is not that proof.
