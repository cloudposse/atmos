# Release tag and artifact integrity

## Incidents

The `v1.233.0` tag resolves to `12f83b5248253662d8af06030e3966034a7ef5d4`, but its
Darwin arm64 asset embeds Go's `vcs.revision=b392e54f74a9c436d2c27fc8ba1b82605e95b58e`.
The latter precedes the credential-injection fix by three commits.

The [draft build](https://github.com/cloudposse/atmos/actions/runs/39352125013)
checked out `b392e54f7` and uploaded binaries for its locally calculated `v1.233.0`.
The draft retained `main` as its target. When published, the tag was created at
the newer `12f83b5`, leaving the earlier binaries attached.
The [publication workflow](https://github.com/cloudposse/atmos/actions/runs/39357239296)
then rebuilt the correct commit with `--skip=publish`. It added a source SBOM,
but did not replace the stale executable assets. The evidence supports stale
draft assets surviving publication, rather than a late build overwriting correct assets.

The subsequent patch attempt had the other mismatch: the
[draft build](https://github.com/cloudposse/atmos/actions/runs/39611235648)
calculated `v1.234.0` from labels, while the
[publication run](https://github.com/cloudposse/atmos/actions/runs/39672354878)
requested assets named for `v1.233.1` and received a 404. The initial incident
report called the intended patch `1.231.1`; the inspected run records use
`v1.233.1`. Both attempted releases were subsequently deleted.

## Publication contract

Draft creation writes notes only. Stable publication is an explicit dispatch of
`publish-release.yml` with a draft ID and full source commit SHA. Nightly and
feature workflows draft notes and then invoke the same local `guarded-release`
composite action. Job environments, permissions and the repository-wide
`atmos-release` concurrency group remain in the calling workflows.

The selected draft's tag is the authoritative version. Preparation requires an
empty draft, pins its target to the requested commit, and creates its tag only
if absent. An existing tag must already resolve to that commit, including when
it is an annotated tag. Existing tags and assets are never overwritten.

The build checks out that exact commit and uses `GORELEASER_CURRENT_TAG` to
prevent a newer tag or label-derived version from taking precedence. Build
configuration and verification tooling come from the workflow revision.
GoReleaser signs and builds locally with `--skip=publish`; the action stages
assets using the public names in its artifact manifest.

`magefiles/release/verify.go` checks metadata, checksums, all ten expected
platform binaries, Go's embedded commit and clean-tree flag, and the embedded
release identity. The CLI derives its displayed version and commit from that
same identity, which survives stripping and `-trimpath`. Old source commits
without that identity cannot pass verification; backport the identity support
before releasing such a branch.

After attestation and upload, the publisher checks the complete asset set and
GitHub's SHA-256 digests against the verified manifest, then rechecks the draft
ID, tag, target and remote tag commit immediately before setting `draft: false`.
Any mismatch or failed upload leaves the release unpublished. Downstream
distribution and major-tag updates require the immutable release's manifest
and asset digests to match. `release-distribution.yml` (formerly `build.yml`) handles downstream distribution without rebuilding release executables.

Failures may leave a pinned tag or partially uploaded draft. They are preserved
for investigation. Do not move the tag or overwrite the assets to retry; use a
fresh empty draft with the same validated tag/commit, or choose a new version.

## Enable immutable releases after deployment

Merge these workflows and local actions, then let older release-writing runs
finish before enabling immutable releases. The publisher stays paused until
`ATMOS_GUARDED_RELEASES_ENABLED` is set to `true`.

Enable **Release immutability** in the repository's **Settings → General →
Releases** section, then set that repository Actions variable to `true`.
An administrator can also perform the same two setting changes with `gh`:

```sh
gh api --method PUT repos/cloudposse/atmos/immutable-releases
gh api repos/cloudposse/atmos/immutable-releases --jq .enabled
gh variable set ATMOS_GUARDED_RELEASES_ENABLED --body true --repo cloudposse/atmos
```

Confirm that the second command returns `true` before running the third.
This is a one-time repository setting change, not a release action.
The current [Settings app repository plugin](https://github.com/repository-settings/app/blob/master/lib/plugins/repository.js)
does not support the separate immutable-releases API, so adding a key to
`.github/settings.yml` would not enable it.

Use **Publish verified release** for stable drafts. All uploads finish before
publication, as required by
[GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases).
The implementation branch does not itself change the live setting.

Anyone retaining direct publishing permissions can bypass these workflows;
administrators can also disable immutability. Preventing that requires
restricting the publishing path and administrative access. The manifest is a
consistency guard, not proof against a privileged publisher forging a manifest.

## Validation

Go regression tests reproduce the two incident identities, inspect real stripped
ELF, Mach-O and PE binaries without executing them, verify the complete shipping
matrix, and reject missing platforms, dirty builds, tampering and unsafe staging
paths. Node tests exercise the action entrypoint, tag pinning, partial uploads,
changed tags/commits, unexpected assets, digest failures, downstream verification
and permission failures.

Local action calls use GitHub's `$/.github/actions/...` syntax, resolving from
the workflow revision without a checkout solely to load the action. Actionlint
1.7.12 predates this syntax; the repository lint configuration excludes only its specific missing-ref
diagnostic for these calls. Go tooling and release source still need checkouts.

## Post-publication maintenance

After verification, `release-distribution.yml` directly invokes the existing Cloud Posse
release-branch-manager action for stable releases and `actions/github-script`
for release notifications. The notification module and its regression tests
live in `.github/scripts/release-comment.cjs` and `release-comment.test.cjs`;
`atmos test local-actions javascript` discovers the tests automatically.
The separate guarded `release-major-tag.yml` workflow remains the sole writer
of the moving major-version tag.
