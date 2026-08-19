# PRD: Native GoReleaser Release Pipeline

## Executive Summary

Atmos already uses GoReleaser — but not visibly, and not where anyone would think to look. The actual
`goreleaser release` invocation happens three layers deep inside an external, org-wide reusable workflow
(`cloudposse/.github`), triggered indirectly from `test.yml`/`nightlybuilds.yml`/`feature-release.yml`. This
repo's own `build.yml` — the file that looks like "the release workflow" — never touches GoReleaser at all;
it only reacts *after* a release is already published, re-downloading whatever GoReleaser uploaded and
signing that. Separately, the Debian/RPM/Alpine packages this project ships on Cloudsmith, and the Scoop
bucket, are built and published by a pipeline that doesn't exist anywhere in this repo — fully external,
fully unverified from here.

This PRD proposes collapsing the release pipeline into one in-repo, auditable GitHub Actions workflow that
invokes GoReleaser directly: build → archive → package (deb/rpm/apk) → sign → SBOM → publish, all inside the
same atomic run that produces the artifacts. No GoReleaser Pro, no second "meta" config repo — both
explicitly ruled out. Everything Atmos needs is available in GoReleaser OSS.

## Problem Statement

### User needs

- **Maintainers** need to be able to read one workflow file and understand how a release actually gets
  built, packaged, and published — today that requires reading four repos (`atmos`, `cloudposse/.github`
  three times over, plus whatever builds the Cloudsmith packages) to answer "what does `goreleaser release`
  actually do for this project."
- **Consumers of release artifacts** (via `cosign verify-blob`, SBOM consumers, provenance-attestation
  checks) need signatures and SBOMs that describe what was actually built, not what was later re-fetched from
  a public URL. A signature over a re-downloaded artifact only proves "this is what was in the release when I
  downloaded it," not "this is what GoReleaser produced" — a real, if narrow, gap for anyone treating those
  signatures as build provenance.
- **Package-manager users** (`apt`/`dnf`/`apk install atmos`) depend on a Cloudsmith pipeline nobody on this
  repo can currently see, test, or fix if it breaks.

### Current state (verified against the live workflows and the external repos they call)

**What's actually broken:**

- `build.yml`'s `sign-and-attest-release` job (`build.yml:144-231`) runs in a separate job, on a separate
  runner, after `needs: release` — it `gh release download`s whatever GoReleaser already published, then
  cosign-signs and SBOM-attests *that downloaded copy*. Anything able to tamper with a just-published release
  asset in that window (compromised token, malicious release edit) gets a valid-looking signature over
  tampered bytes.
- Debian/RPM/Alpine packages (Cloudsmith, per `website/docs/quick-start/install-atmos.mdx:52-93` and
  `website/static/install.sh`'s `install_via_cloudsmith`) and the Scoop bucket are built by *something*, but
  no `nfpm`, `cloudsmith`, or `scoop` reference exists anywhere in this repo's workflows or scripts. Fully
  external, fully opaque.

**What already works and is not being changed by this PRD:**

- GoReleaser itself (`.goreleaser.yml`, `.goreleaser.draft.yml`) already builds all platform binaries
  correctly, driven by `test.yml`'s `release` job → `cloudposse/.github`'s `shared-go-auto-release.yml` →
  `shared-auto-release.yml` → `github-action-auto-release` (composite) → `release-drafter/release-drafter`.
- **release-drafter** already does label-driven semver resolution (`major`/`minor`/`patch`/`no-release`
  labels, enforced pre-merge by `codeql.yml`'s `pr-semver-labels` job) and changelog generation well. This is
  not the broken part and this PRD does not propose replacing it.
- Homebrew formula bumping (`build.yml:25-43`), Docker build/push/sign (`build.yml:45-133`), the major-tag
  mover, release-branch management, and the release PR-comment bot all function today.
- The `release/feature` PR-label mechanism (`feature-release.yml`) already gives us a safe way to cut a real,
  disposable prerelease pre-merge — this is the natural test harness for this migration (see Verification).

**One independent bug worth a one-line fix regardless of this PRD:** `release-major-tag.yml` and
`shared-release-branches.yml`'s `major-release-tagger` job (called from `build.yml:21-23`) are exact
duplicates — same action, same version pin, same `prerelease == false` gate, both firing on every
`release: published` event. Worth deleting one, independent of everything else here.

## Goals

1. One in-repo GitHub Actions workflow owns the entire build→package→sign→SBOM→publish pipeline for binaries
    and packages, invoking `goreleaser release` directly — no external reusable-workflow hop for the actual
    release build.
2. Signing (cosign, keyless/OIDC) and SBOM generation (syft) happen *inside* the same `goreleaser release`
    run that produces the artifacts, via GoReleaser's native `signs:`/`sboms:` blocks — closing the
    build-vs-fetch gap.
3. Debian/RPM/Alpine packages are built natively via GoReleaser's `nfpms:` block and pushed to Cloudsmith
    from an explicit, in-repo step — reclaiming that pipeline from wherever it currently lives.
4. Full behavioral parity: normal releases, nightly prereleases, PR-label feature prereleases,
    label-driven semver + changelog (release-drafter, unchanged), Homebrew, signed multi-arch Docker images.
5. No GoReleaser Pro license. No second config-sharing repo.

## Non-goals

- macOS/Windows code signing and notarization. Not done today; genuinely new scope (Pro-gated on the
  native path, or a separate `gon`/`quill`/DigiCert step on OSS); a candidate follow-up, not part of this
  migration.
- Changing how release-drafter resolves versions or writes changelog text — it works, leave it alone.
- npm distribution (not shipped today, out of scope).

## Decisions already made

- **GoReleaser OSS, not Pro.** Confirmed against GoReleaser's own Pro-feature list: the only Pro-gated
  features relevant to Atmos are the native `--nightly` flag and native CloudSmith push — both have
  straightforward OSS workarounds (below). Everything else Atmos needs (`builds`, `archives`, `checksum`,
  `nfpms`, `brews`, `scoops`, `dockers`/`docker_manifests`, `sboms`, `signs`, native `changelog:`) is OSS.
- **No shared/"meta" config repo** (unlike Charmbracelet's `charmbracelet/meta` two-repo model). GoReleaser
  Pro's `includes: from_url` remote-config feature exists specifically to support that pattern — since we're
  not using it, that's one more reason Pro isn't needed. `.goreleaser.yml` stays a normal, complete, in-repo
  config file, as it already is today.

## Solution architecture

### New workflow: `.github/workflows/release.yml` (replaces `build.yml`'s role as release orchestrator)

A single job, triggered directly by GoReleaser's own tag-based model rather than reacting to a release
GitHub already created out-of-band:

```
checkout (fetch-depth: 0, needed for changelog + tag history)
  → setup-go
  → sigstore/cosign-installer
  → anchore/sbom-action/download-syft (syft binary for goreleaser's sboms: block)
  → docker/setup-qemu-action + docker/setup-buildx-action (multi-arch docker)
  → docker login (ghcr.io)
  → goreleaser/goreleaser-action (distribution: goreleaser, args: release --clean)
  → [nfpm-produced .deb/.rpm/.apk already in dist/, from the run above]
  → cloudsmith-cli push (deb/rpm/apk) against dist/*.deb, dist/*.rpm, dist/*.apk
  → upload dist/ as a workflow artifact (debugging, short retention)
```

Everything above the `cloudsmith-cli push` line is one atomic GoReleaser invocation — builds, archives,
checksums, `nfpms:` packages, Homebrew tap push (`brews:`), Docker multi-arch build+push
(`dockers:`/`docker_manifests:`), cosign signing of the checksums file (`signs:`, keyless/OIDC — no static
key), SBOMs of every archive (`sboms:`, syft-backed), and the GitHub Release itself (`release:` block,
already configured with `draft: true`/`prerelease: auto` in today's `.goreleaser.yml`) all happen together.
Docker image signing can use GoReleaser's `docker_signs:` block (confirmed OSS) to close the same
build-vs-fetch gap for images that `build.yml`'s current pull-then-sign approach has, closing it end to end.

### `.goreleaser.yml` additions

On top of the existing `builds`/`archives`/`checksum`/`release` blocks:

```yaml
nfpms:
  - formats: [apk, deb, rpm]
    vendor: cloudposse
    maintainer: "Cloud Posse <hello@cloudposse.com>"
    file_name_template: "{{ .ConventionalFileName }}"

brews:
  - repository:
      owner: cloudposse
      name: homebrew-tap
    directory: Formula
    homepage: "https://atmos.tools"
    # replaces build.yml's dawidd6/action-homebrew-bump-formula job

dockers:
  - image_templates: ["ghcr.io/cloudposse/atmos:{{ .Version }}-amd64"]
    goarch: amd64
    build_flag_templates: ["--platform=linux/amd64"]
  - image_templates: ["ghcr.io/cloudposse/atmos:{{ .Version }}-arm64"]
    goarch: arm64
    build_flag_templates: ["--platform=linux/arm64"]
docker_manifests:
  - name_template: "ghcr.io/cloudposse/atmos:{{ .Version }}"
    image_templates: ["...{{ .Version }}-amd64", "...{{ .Version }}-arm64"]

sboms:
  - artifacts: archive

signs:
  - cmd: cosign
    args: ["sign-blob", "--yes", "--output-signature=${signature}", "${artifact}"]
    artifacts: checksum

docker_signs:
  - cmd: cosign
    args: ["sign", "--yes", "${artifact}"]
    artifacts: manifests
```

(Sketch, not final — exact `nfpm` metadata, Homebrew tap repo permissions/token, and Docker build args need
real values during implementation, not in this PRD.)

### The two OSS workarounds

- **Nightly builds** (Pro-gated `--nightly` flag): reimplement as "compute tomorrow's-nightly version string
  (date+SHA or reuse release-drafter's resolver), create/move a real lightweight tag, run the *same*
  `goreleaser release --clean` job against it, marked prerelease." This is close to what the current pipeline
  already effectively does (and identical in spirit to how `gruntwork-io/terragrunt`'s "tip builds" work) —
  no functional loss, just workflow glue instead of a one-line flag.
- **CloudSmith push** (Pro-gated native integration): GoReleaser OSS's `nfpms:` block still produces real
  `.deb`/`.rpm`/`.apk` files in `dist/`; add one explicit `cloudsmith-cli push deb/rpm/apk` step (or
  Cloudsmith's own GitHub Action) after the GoReleaser step, same end result as the Pro block.

## Parity matrix

| Today | Owned by | After this PRD |
|---|---|---|
| Binary build+archive+checksum | `cloudposse/.github` (external, invisible) | `.github/workflows/release.yml`, in-repo |
| Signing/SBOM | `build.yml`'s downstream re-fetch job | Native `signs:`/`sboms:` inside the GoReleaser run |
| Homebrew | `build.yml`'s `dawidd6/action-homebrew-bump-formula` job | Native `brews:` block |
| Docker build+sign | `build.yml`'s `docker` job (build→pull→sign) | Native `dockers:`/`docker_manifests:`/`docker_signs:` |
| deb/rpm/apk | Unknown external pipeline | Native `nfpms:` + explicit CloudSmith push step, in-repo |
| Scoop | Unknown external pipeline | Deferred — flagged as open question below |
| Changelog + semver | release-drafter via `cloudposse/.github` chain | **Unchanged** — release-drafter, config/invocation TBD (see below) |
| Nightly prerelease | `nightlybuilds.yml` → external chain | `nightlybuilds.yml` (or folded into `release.yml`) → real tag + same GoReleaser run |
| Feature-branch prerelease | `feature-release.yml` → external chain, `.goreleaser.draft.yml` reduced matrix | Same trigger, points at the new in-repo workflow |
| Major-tag mover / release-branch mgmt / PR comment | `cloudposse/.github`'s `shared-release-branches.yml` | **Unchanged**, not broken, not in scope |
| Trivy image scan | `build.yml`'s `docker` job | Carries over as a step after the Docker publish, unchanged in spirit |

## Implementation phases

1. **Binaries + native signing/SBOM.** Stand up `release.yml`, move `builds`/`archives`/`checksum`/`signs`/
    `sboms` into `.goreleaser.yml`, retire `build.yml`'s `sign-and-attest-release` job. This alone fixes the
    headline security gap and is independently shippable.
2. **Docker.** Fold `build.yml`'s `docker` job into GoReleaser's `dockers:`/`docker_manifests:`/
    `docker_signs:`, keep the Trivy scan step immediately after. Retire `build.yml`'s `docker` job.
3. **Homebrew.** Native `brews:` block, retire `build.yml`'s `homebrew` job.
4. **Packages.** Add `nfpms:` + CloudSmith push step, reclaiming deb/rpm/apk.
5. **Nightly + feature prereleases.** Point `nightlybuilds.yml`/`feature-release.yml` at the new workflow;
    implement the real-tag nightly workaround.
6. **Decommission.** Once 1-5 are stable, stop calling `cloudposse/.github`'s `shared-go-auto-release.yml`
    entirely; delete `.goreleaser.draft.yml` if its reduced-matrix behavior gets folded into a flag/variable on
    the main config instead. Fix the `release-major-tag.yml` duplicate noted above (independent, can happen
    any time).

Each phase should land as its own PR, verified via the `release/feature` label mechanism before merging
(cuts a real, disposable prerelease — see Verification) rather than risking a real release cut.

## Open questions

- **release-drafter placement.** Keep invoking it through `cloudposse/.github`'s chain (smaller diff, but
  keeps one piece of the pipeline external/opaque — the thing this PRD is otherwise fixing), or bring it
  in-repo too (`release-drafter/release-drafter` action directly + copies of the four org-level config
  variants — `auto-release.yml`/`auto-pre-release.yml`/`auto-feature-release.yml`/
  `auto-release-hotfix.yml` — currently only readable from `cloudposse/.github`, not overridden here)? Leaning
  towards bringing it in-repo for full auditability, but that's more surface area to get right and isn't
  the security-critical part — could be its own follow-up PRD.
- **When does the build actually happen?** Today, GoReleaser runs on *every push to `main`/`release/v*`*,
  continuously updating a draft release so it's always ready to publish; publishing later is just a
  visibility flip, no rebuild. Moving to a tag-push trigger (Charmbracelet's model) means the build only
  happens once, at actual release time — simpler and cheaper, but loses the "draft is always current"
  property. Needs a explicit decision, not an assumption.
- ~~**Scoop.**~~ **Resolved**: not our pipeline at all. `atmos.json` lives in Scoop's own official
  `ScoopInstaller/Main` bucket (confirmed via `gh api`), maintained entirely by Scoop's own bot via
  `checkver`/`autoupdate` polling our GitHub releases directly for `atmos_$version_windows_{amd64,386}.exe`
  and `atmos_$version_SHA256SUMS`. No `scoops:` block needed, no token, no action from this repo, today or
  after migration. **New constraint this creates**: that manifest hardcodes our current binary/checksum
  filename pattern — any later phase that changes archive format (currently raw `formats: [binary]`, no
  zip/tar.gz) or checksum naming must preserve it, or Scoop installs break silently until their bot notices.
- ~~**Homebrew tap token.**~~ **Resolved**: `GH_BOT_TOKEN` already exists as a secret in the `release`
  environment (confirmed via `gh api repos/cloudposse/atmos/environments/release/secrets`) — it's exactly
  what `build.yml`'s current `homebrew` job already uses (`build.yml:42`,
  `dawidd6/action-homebrew-bump-formula`) to push to `cloudposse/homebrew-tap`. `brews:` needs the same write
  access to the same repo, so this credential carries over directly; Phase 3 is not credential-blocked.
- **CloudSmith credentials/repo names.** Entirely unknown from this repo today; needs whoever owns the
  current external pipeline (or CloudSmith account access) to supply the actual repo/org names and an API
  token before Phase 4 can be implemented, not just designed.

## Verification

- Each phase's PR gets the `release/feature` label to cut a real, disposable prerelease pre-merge
  (`feature-release.yml`'s existing mechanism, `.goreleaser.draft.yml`'s reduced platform matrix)  —
  exercises the actual GoReleaser run, Docker push, signing, and (once implemented) package build without
  touching the real release cadence.
- `cosign verify-blob`/`cosign verify` against the resulting checksums file and Docker manifest, confirming
  signatures validate and — critically — that the signed bytes match a fresh `git checkout` + local build,
  not just "whatever is currently on the release" (the property this whole PRD exists to guarantee).
- `nfpm`-produced `.deb`/`.rpm`/`.apk` installed in a throwaway container per format, smoke-tested with
  `atmos version`.
- `install-sh-smoke.yml` continues to pass unmodified — the public `install.sh` contract to end users
  shouldn't change shape, only what feeds it.
