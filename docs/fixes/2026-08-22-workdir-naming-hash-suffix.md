# Fix: `.workdir` names mangled by hyphen-escaping, replaced with a prefix+hash formula

**Date:** 2026-08-22

## Summary

`workdir.BuildPath` built `.workdir/<componentType>/<stack>-<instanceName>`
directory names by running the component/instance name through
`escapeComponentNameForPath`, which injectively encoded the name into a
single filesystem path segment by escaping every literal `-` to the
two-character token `-h` (and `/` to `-s`, `\` to `-b`), using `-` itself as
the escape marker. Since `-` is the single most common separator in Atmos
component/stack/version names, this mangled ordinary names badly — e.g.
`vpc-flow-logs-1.226.1` became `vpc-hflow-hlogs-h1.226.1`, an `h` inserted
after every hyphen past the first segment. Reported directly by the user as
"another weird thing" seen in real `.workdir` output.

Two escape-marker swaps (`~`, and standard `%2F`-style percent-encoding)
were drafted and rejected during design discussion: both still required the
same fragile "prove every input maps to a unique output" character-escaping
reasoning the original `-h` scheme already had, just with a different
marker character. That's the wrong shape of fix for a problem that already
has a standard, much simpler solution.

## Root cause

`escapeComponentNameForPath` (`pkg/provisioner/workdir/types.go`) needed to
turn an arbitrary component/instance name into a single, collision-free
path segment without ever creating a real subdirectory for a `/`-bearing
name (see "why not real subfolders" reasoning below). Doing that with
reversible character-escaping requires picking a marker character and
escaping every occurrence of it in the input, or the marker becomes
ambiguous with real input. `-` was chosen as the marker, which is exactly
why every literal hyphen — the most common character in real Atmos names —
had to be escaped too.

Component name (and, when present, `atmos_component`) is deliberately kept
to a single path segment rather than a real nested directory even when it
contains `/`, because the workdir needs to sit at a **fixed depth** under
`basePath` regardless of a component's own naming. Some components use a
relative Terraform module `source` (the classic account-map pattern,
`source = "../../account-map/modules/..."`) that assumes a fixed number of
`..` hops from the component's own on-disk directory; a workdir whose depth
varied based on an unrelated component-naming choice would silently break
that reference for exactly the components whose name happens to contain
`/`. (Separately, JIT/workdir provisioning cannot resolve a *sibling*
component's workdir via a relative reference at all today, since every
workdir name is stack-prefixed — a distinct, unaddressed gap; see "Known
limitation" below.)

## Fix: prefix+hash instead of character-escaping

Replaced the whole injective-escaping design with the standard
human-readable-prefix + content-hash-suffix pattern (the same shape used by
Bazel, npm, and most build/package caches):

```text
prod-vpc-flow-logs-1.226.1-09b13626
dev-eks-ingress-1-c1ff2a35
dev-eks-ingress-controller-6382df14
```

- `sanitizeComponentNameForPath(name string) string` — a purely cosmetic,
  **lossy** display prefix: `/` and `\` both become `-`, nothing else
  changes. It does not need to be reversible or collision-proof, so no
  escape tokens, no injectivity proof.
- `workdirPathHash(stack, component string) string` — `hex(sha256(stack +
  "\x00" + component))[:8]` (32 bits). This is what actually guarantees two
  different `(stack, component)` pairs never collide, since it's computed
  from the *unsanitized* strings. Two names that sanitize to the identical
  prefix — e.g. `eks-ingress/1` and the unrelated literal `eks-ingress-1`,
  both sanitizing to `eks-ingress-1` — still land on distinct directories
  because their hashes differ.
- `BuildPath` now builds the name as `"<stack>-<sanitized>-<hash>"` instead
  of `"<stack>-<escaped>"`.

A hash is only *practically* collision-free, not mathematically so like the
scheme it replaced. Closed that gap with a new identity check on every
workdir *reuse*, not just legacy migration: `createWorkdirDirectory` now
checks whether a directory already exists at the computed hash-suffixed
path and, if so, reads its `metadata.json` and confirms it actually belongs
to the component/stack being provisioned (`checkWorkdirIdentityOnReuse`)
before treating it as a safe cache hit. A mismatch — a genuine (if
vanishingly unlikely) hash collision, or a foreign/corrupted directory —
fails closed with a clear `errUtils.ErrWorkdirCreation` rather than
silently reusing or corrupting another identity's workdir. Unlike the
existing legacy-migration identity check (`verifyLegacyWorkdirIdentity`),
missing/unreadable metadata here is *not* treated as suspicious: the
hash-suffixed path format is brand new, so a workdir found at it with no
readable metadata overwhelmingly means an earlier provision of this exact
identity was interrupted before `WriteMetadata` ran (e.g. a crash between
`MkdirAll` and metadata write), not a foreign directory — failing closed on
that common, benign case would permanently block re-provisioning the same
identity.

## Migration

This naming scheme has shipped before (see
`docs/fixes/2026-08-14-workdir-buildpath-collision-and-stack-traversal.md`
and `website/blog/2026-08-17-container-config-validation-and-workdir-path-encoding.mdx`),
so an orphan-and-forget approach isn't acceptable — same reasoning that
motivated the original `migrateLegacyWorkdir`. Generalized it to try an
ordered, **deduped** list of legacy-path candidates (dedup matters because
every workdir name now always gains a hash suffix, so a legacy candidate
essentially never coincides with the new path anymore — even for a plain
component with no special characters — unlike before, when the two schemes
happened to agree whenever the name had nothing to escape):

1. the original pre-escaping `"%s-%s"` formula (`legacyWorkdirName`,
    unchanged)
2. the now-retired hyphen-encoded formula (`legacyHyphenEncodedWorkdirName`,
    a frozen copy of the old `escapeComponentNameForPath` body, kept solely
    to locate old directories — never called from the new creation path)

Both candidates share the same exists-check → identity-verify → rename
logic (`migrateFromLegacyPath`), reusing the existing
`verifyLegacyWorkdirIdentity` fail-closed guard against blindly renaming an
ambiguous or unverifiable legacy directory.

**Cost note:** because a legacy-formula name now essentially never equals
the new hash-suffixed name, every single workdir provision permanently
pays for one extra `Exists` syscall for the legacy-migration check (deduped
to one call for the common no-special-character case) plus one for the new
reuse-identity check — a few cheap stat calls, not a measurable regression,
but worth noting since the original single-candidate design could skip this
entirely for the common case.

## Changes

- `pkg/provisioner/workdir/types.go`: removed `escapeComponentNameForPath`;
  added `sanitizeComponentNameForPath`, `workdirPathHash`,
  `workdirHashLength` (8); `BuildPath` and `resolveWorkdirComponentName`
  updated for the new formula; doc comments rewritten to describe the
  prefix+hash design. Also fixed a stale claim in
  `resolveWorkdirComponentName`'s doc comment that it "mirrors" sanitization
  in `internal/exec/terraform_generate_backends.go` — that file's
  `strings.Replace(componentName, "/", "-", -1)` is an unrelated, simpler,
  non-injective substitution for a Go-template context value, never a real
  mirror.
- `pkg/provisioner/workdir/workdir.go`: added
  `legacyHyphenEncodedWorkdirName`; generalized `migrateLegacyWorkdir` into
  a deduped loop over `migrateFromLegacyPath` candidates; added
  `checkWorkdirIdentityOnReuse` and wired it into `createWorkdirDirectory`
  before `MkdirAll`.
- Updated every test asserting on the old `-h`/`-s`/`-b` literal encoding
  across `pkg/provisioner/workdir/{types,workdir,integration,clean}_test.go`,
  `pkg/component/workdir_path_test.go`,
  `internal/terraform_backend/terraform_backend_local_test.go`,
  `pkg/provisioner/source/{source,provision_hook}_test.go`,
  `pkg/terraform/output/config_test.go`,
  `cmd/terraform/workdir/workdir_helpers_test.go`,
  `tests/cli_{jit_source_workdir,source_provisioner_workdir,jit_source_oci}_test.go`.
  Independent-oracle-style tests (`pkg/component/workdir_path_test.go`) got
  their expected hash suffixes recomputed externally (`shasum -a 256`), not
  by calling the production hash function, preserving their
  non-tautological guarantee.
- Added 3 new tests in `pkg/provisioner/workdir/workdir_test.go`:
  `TestServiceProvision_MigratesPreExistingHyphenEncodedLegacyWorkdir` (the
  second legacy candidate migrates end-to-end),
  `TestServiceProvision_ReusesExistingWorkdirWithMatchingIdentity` (happy
  path reuse), `TestServiceProvision_FailsClosedWhenExistingWorkdirBelongsToDifferentIdentity`
  (collision simulated by pre-seeding the real computed target path with a
  different identity's metadata, the same trick
  `TestServiceProvision_DoesNotMigrateLegacyWorkdirBelongingToDifferentIdentity`
  already used for the legacy-migration collision case).

## Known limitation (out of scope for this fix)

Separate investigation during design found that JIT/workdir provisioning
cannot currently support a component referencing a *sibling* component via
a relative Terraform module `source` (the account-map pattern,
`source = "../../account-map/modules/..."`): every workdir name is always
stack-prefixed, so a sibling component's own workdir never sits at the
plain, unprefixed relative location such a reference assumes, and nothing
in the provisioner symlinks or aliases it there. This is unaddressed today
regardless of naming scheme (character-escaping or hash-suffix) and isn't
something this fix attempts to solve — the only existing mitigation is
leaving `provision.workdir.enabled` off (the default) for components that
rely on this pattern. Flagged for the user to decide on a follow-up.

## Validation

- `go build ./...` — clean.
- `gofumpt -l` on every changed file — clean.
- `go test ./pkg/provisioner/workdir/... ./pkg/provisioner/source/...
  ./pkg/terraform/output/... ./internal/terraform_backend/...
  ./pkg/component/... ./cmd/terraform/workdir/...` — all pass (201 tests in
  `pkg/provisioner/workdir` alone, including the 3 new ones above).
- `go test ./tests/...` (full CLI-level integration suite, including
  `TestJITSource_*`, `TestSourceWorkdir_*`) — all pass; real hash suffixes
  produced by actually running the CLI (e.g.
  `.workdir/terraform/dev-oci-component-751f8f04`) cross-checked against
  independently computed `sha256` values.
