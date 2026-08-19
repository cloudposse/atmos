# PRD: `vendor.lock.yaml`

## Status

Implemented.

## Problem

`atmos vendor pull`/`atmos vendor update` fetch and materialize components, mixins, and
`vendor.yaml`/`component.yaml` sources onto disk, but before this feature there was no receipt of
what was actually fetched: no way to detect that a vendored file was manually edited after the
fact, no way to verify a checkout's vendored state matches what was last pulled without re-fetching
everything, and no auditable provenance trail for the SBOM generator (`docs/prd/sbom-provenance.md`)
to build on. `version:` (an exact pin, the sole source of truth for what gets fetched) already
answers "what should be here" — nothing answered "does what's actually on disk still match."

A lock file also enables a policy choice most Atmos projects don't take today but the format
doesn't foreclose: committing vendored output directories to Git gives an immutable historical
record and works fully offline, but at real cost in repo size and diff noise on every version bump
— a trade-off many package ecosystems (npm, Go modules, mise, aqua) resolve by *not* committing the
fetched artifacts at all, relying instead on a lock file's immutable hashes as the reproducibility
guarantee: re-fetch on a clean checkout, verify the bytes match the locked digest, done. Committing
vendored files and verifying them against a lock are not mutually exclusive — Atmos does both by
default — but `vendor.lock.yaml`'s digest/checksum guarantee is what would make the "don't commit
the fetched bytes, verify them instead" choice legitimate for a project that wants it, the same way
it already is for those other ecosystems.

## Goals

1. Record a verifiable receipt — declared source, resolved identity, and a per-file digest — for
  every artifact vendoring materializes on disk, across every discovery path.
2. Detect drift (a missing or modified lock-owned file) without any network access, and let a user
  choose how loudly that drift is reported before a `pull` silently re-fetches it
  (`vendor.lock.enforcement`).
3. Expose drift detection directly, as a CI-friendly command (`atmos vendor verify`), not just as a
  side effect of `pull`.
4. Support a semver-range `version:` as an alternative to an exact pin, without reintroducing an
  unconditional network dependency at every `pull`.

## Non-goals

- Selecting *what version* to fetch for an exact-pinned source. `version:`/`component.yaml`'s
  `source.version` remain the sole source of truth for that; the lock only verifies, after the
  fact, that what's on disk matches what was last fetched under that declaration.
- Representing Terraform provider/module locks or toolchain locks. Those are separate,
  domain-appropriate lock formats (`.terraform.lock.hcl`, `.tools/toolchain.lock.yaml`,
  `versions.lock.yaml`) that this format does not attempt to duplicate or supersede — see
  `docs/prd/sbom-provenance.md` for how all of them feed one provenance graph.
- Signing or cryptographic attestation of vendored content. `Source.Digest`/`File.SHA256` are
  integrity checks against what was fetched, not a trust root.

## Schema

```yaml
version: 1
artifacts:
  <artifact-id>:
    name: vpc
    kind: remote # remote | oci | local
    target: components/terraform/vpc
    source:
      declared: github.com/cloudposse/terraform-aws-vpc.git?ref=v1.5.0
      resolved: github.com/cloudposse/terraform-aws-vpc.git?ref=v1.5.0
      digest: sha256:...
      etag: "\"a1b2c3\""           # cache metadata only, see below
      last_modified: "Tue, 01 ..." # cache metadata only, see below
      version_constraint: "^1.0.0" # only present for a range-declared version:, see below
      resolved_version: v1.5.0     # only present for a range-declared version:, see below
    files:
      - path: main.tf
        type: file
        mode: 420
        sha256: sha256:...
    included_paths: ["**/*.tf"]
    excluded_paths: []
    order: 0
```

`<artifact-id>` is a stable hash of the artifact's kind, target, and (for a mixin) its filename —
deliberately not a function of the declared source, so renaming a source URI while keeping the same
target is treated as a declared-source change on the *same* artifact, not a new one.
`included_paths`/`excluded_paths` are omitted entirely for artifacts recorded with no filtering
(mixins, and unfiltered sources), so a lock file written before these fields existed loads
identically to an explicit empty list.

`Source.ETag`/`Source.LastModified` are populated only for HTTP(S) sources with a captured response
header (empty for Git/OCI/local sources, which already have stronger identity in `Digest`) and are
**never** consulted by drift detection — cache metadata only, exactly as their own field comments
say.

## Coverage

`vendor.lock.yaml` records a verifiable receipt — declared source, resolved identity, and a
per-file digest — for every artifact vendoring materializes on disk, across every discovery path:
`vendor.yaml` sources (including sources restricted by `included_paths`/`excluded_paths`),
local-file `vendor.yaml`/`component.yaml` sources, remote and OCI `component.yaml` sources, and
remote/OCI/local-file mixins, at any nesting depth under a component type's base path. The lock is
never consulted to select what version to fetch — `component.yaml`/`vendor.yaml`'s `version:`
field is the sole source of truth for what gets fetched; the lock only verifies, after the fact,
that what's on disk matches what was last fetched under that declaration. Terraform
provider/module locks and toolchain locks are separate, domain-appropriate locks that this format
does not attempt to represent.

## Lock enforcement

`vendor.lock.enforcement: strict | warn | silent` (default `warn`) controls what `atmos vendor
pull` does when a package's on-disk state no longer matches its receipt. Overridable per-invocation
with `--lock-enforcement`.

- **`silent`** — re-fetch the drifted package, no reporting. The exact behavior of every `pull`
  before this feature existed.
- **`warn`** (default) — same re-fetch, plus one warning per drifted package naming why it
  drifted (no lock entry, declared source changed, included/excluded paths changed, or a specific
  file missing or checksum-mismatched).
- **`strict`** — refuse to run, before any fetch/copy/write, when any package has drifted and
  `--refresh-lock` was not explicitly passed. Lists every drifted package and its reason.
  `--dry-run` is unaffected at every level — it never mutates regardless of enforcement.

## `atmos vendor verify`

`atmos vendor verify [--component <name>] [--format table|json]` compares every lock-owned file on
disk against its receipt and reports drift, exiting non-zero when any is found — no fetch, no
network access, CI-friendly. It is the read-only counterpart to `atmos vendor update --check`:
`verify` checks whether what's already on disk still matches what was locked; `update --check`
checks whether a *newer* upstream version is available. The two are never conflated in either
command's help text.

## Design lineage

`vendor.lock.yaml`'s declared/resolved/digest triad is closest in spirit to **aqua's
`checksums.json`** — an integrity-verification receipt layered on top of a version already pinned
elsewhere (`aqua.yaml`) — not to Terraform's `.terraform.lock.hcl`, npm's `package-lock.json`, or
mise's `mise.lock`, all of which are actively consulted to *select* what gets installed (`npm ci`
fails on manifest/lock disagreement; mise's `locked` setting uses pre-resolved lock URLs instead of
re-resolving). For an exact-pinned `version:` (`component.yaml`/`vendor.yaml`'s field is already
the single, git-diffable, PR-reviewable source of truth for what to fetch), the lock stays purely a
receipt, exactly like aqua: aqua never needs this distinction in the first place because
`aqua.yaml` doesn't support version ranges at all, so its lock never has anything to pin beyond an
integrity check.

The closest same-domain prior art — declarative vendoring of Git repos, GitHub releases, Helm
charts, and OCI/image content into a target directory, the exact problem `vendor.yaml`/
`component.yaml` solve — is Carvel's **vendir**, not a general-purpose package manager. `vendir
sync` writes `vendir.lock.yml` next to `vendir.yml` on every run, recording each source's resolved
reference (a Git SHA, a resolved GitHub release URL, an image digest); by default this is exactly
Atmos's receipt role, regenerated fresh from `vendir.yml` every sync, never consulted to select
what to fetch. But vendir also ships an explicit `vendir sync --locked`/`-l` mode that instead
fetches using `vendir.lock.yml`'s already-resolved references rather than re-resolving `vendir.yml`'s
constraints — i.e. the exact same two-mode split Atmos landed on (lock-as-receipt vs.
lock-as-resolution-pin), independently arrived at by a tool in the same problem domain, just gated
by an explicit CLI flag applying to the whole sync rather than automatically per-source based on
whether a given `version:` is a range. That vendir needed both modes too is corroborating evidence
this is a genuine hybrid the domain calls for, not an Atmos-specific inconsistency.

The two formats otherwise diverge in scope, not just in that one mode-selection mechanism:

| | vendir | Atmos |
| --- | --- | --- |
| Granularity | Source-level resolved ref only (a commit SHA, an image digest, a release URL) | A SHA-256 for **every individual file** written, not just the source |
| Drift detection | None — no per-file checksums, so a manually edited file is indistinguishable from an untouched one without a fresh sync | `atmos vendor verify` diffs on-disk files against the lock's per-file checksums, zero network access |
| Lock-as-pin trigger | Global `-l`/`--locked` CLI flag applied to the whole sync | Automatic, per source, based on whether `version:` itself is a range vs. an exact pin — both can coexist in one project |
| Drift response policy | None | `vendor.lock.enforcement: silent\|warn\|strict` |
| Schema shape | Different field shapes per source type (git gets a SHA+title, images get digest+tag, Helm gets app+chart version, ...) | One uniform `Source` shape regardless of kind (remote/OCI/local) |
| Ownership/cleanup | None found — `vendir sync` mirrors the target directory from `vendir.yml` each run | The lock is also an ownership registry: `atmos vendor clean` only deletes lock-owned files, refusing a manually modified one without `--force` |

The structural difference underneath that table: vendir's lock only ever answers "what upstream ref
did we resolve," while Atmos's also answers "does what's actually on disk still match" — that
second question is what makes `vendor verify` and enforcement levels possible at all, and vendir has
no equivalent to either.

Atmos's `version:` field, however, *does* support a semver-range expression as an alternative to
an exact pin — and once ranges exist, the lock necessarily gains a second role for those sources
specifically: recording the first resolved concrete version so every subsequent pull reuses it (no
network call, no silent drift to a different version within the range) until an explicit `vendor
update` or `--refresh-lock` re-resolves it. This is deliberately closer to npm/mise's model, but
scoped only to range-declared sources — an exact pin never triggers it. A range and a
`constraints.version` ceiling are mutually exclusive on the same source: once `version:` is itself
a range, `constraints.version`'s entire purpose (bounding what `atmos vendor update` may bump an
*exact* pin to) no longer applies, and setting both is a hard validation error rather than a silent
no-op. `constraints.excluded_versions`/`no_prereleases` remain valid and layered on top of a range,
since they aren't expressible inside a bare semver constraint string either way.

An earlier, uncommitted planning note for this feature proposed applying the stronger
resolved-and-pinned model universally, even to exact pins ("later pulls fetch that locked commit,
not a mutable tag/branch") — that blanket version of the idea is explicitly not adopted: for an
exact pin, the manifest's `version:` string is already the complete, unambiguous answer, and having
the lock second-guess it would create two independent, potentially conflicting sources of truth for
no benefit. The narrower, range-scoped version above is adopted instead, because there the manifest
genuinely doesn't specify one answer and something has to.

Not every comparable tool has a lock file at all — Homebrew Bundle deliberately ships none
("`brew bundle` does not and will not have a concept of a 'Brewfile lock file'... like e.g.
`package-lock.json` or `Gemfile.lock`"), since Homebrew's rolling-release model doesn't support
pinning to older versions in the first place. Atmos's choice to have a lock is justified by its own
use case — infrastructure reproducibility, an auditable provenance trail feeding SBOM generation,
and verifiable-without-network drift detection at apply time.

## Known limitations

- A semver-range `version:` is supported only for Git sources — the only source type in this
  codebase with a tag-listing mechanism. A range declared on an OCI, local-file, or plain
  HTTP/S3 source fails clearly at resolution time rather than silently mis-templating.
- A mixin's own `version:` field does not support a semver range (only a component's top-level
  `source.version` does); a range there is templated as a literal, unresolved string like any
  other non-range value, typically failing at fetch time with an invalid-ref error.
- Per-component update batching (one isolated branch/PR per updated component) is a
  `component-updater.md`-scoped limitation, not a lock-format one — see that PRD's Known
  Limitations section.

## Testing

- `pkg/vendoring/lockfile` — table-driven tests for `Record`/`IsMaterialized`/`Verify`/`Clean`
  across plain, patterned, and mixin call shapes; drift-reason coverage for every enforcement path.
- `pkg/vendoring/install` — `FilterPending`'s enforcement-level matrix (strict/warn/silent ×
  drifted/clean); `ResolveDeclaredVersion`'s exact-pin fast path (zero lock/network access),
  first-resolution, zero-network cache reuse, `--refresh-lock`, and non-Git-source error, all
  against a fake `RemoteLister` (no real network access, per this repo's testing convention).
- `cmd/vendor` — `atmos vendor verify`'s table/JSON rendering and `--component` filtering; an
  end-to-end test proving `--lock-enforcement strict` blocks a drifted `vendor update --pull` and
  `--refresh-lock` clears it.
