# Release tag signing for 1.235.0

Milestone: [1.235.0](https://github.com/cloudposse/atmos/milestone/11).

## Immediate installer repair

Atmos v1.233.2 and v1.233.2-rc.3 have a Cosign certificate SAN of
`https://github.com/cloudposse/atmos/.github/workflows/release-pipeline.yml@refs/heads/main`.
The previous v1.233.0 certificate names the shared workflow in `cloudposse/.github`.
Creating a tag and checking out its commit in a workflow dispatched on `main` does
not change that workflow's authenticated OIDC ref.

[Aqua PR #63297](https://github.com/aquaproj/aqua-registry/pull/63297) preserves the
historical rules, limits the shared-workflow rule to `semver("< 1.233.2-0")`, and
uses the exact `release-pipeline.yml@refs/heads/main` identity thereafter. Releases
through 1.227.0 keep their existing checksum-only policy. `pkg.yaml` is Aqua's test
matrix; Atmos consumes the adjacent `registry.yaml`, not `pkg.yaml`.

After that PR merges, remove the temporary `cloudposse/atmos` signer override from
`toolchain.registries` in `atmos.yaml` (or the temporary custom registry URL). Keep
other registry entries. Refresh the cached Atmos recipe before retrying installation:

```sh
# macOS/Linux; honors the same cache-base overrides as Atmos.
atmos_cache_base="${ATMOS_XDG_CACHE_HOME:-${XDG_CACHE_HOME:-$HOME/.cache}}"
rm -f "$atmos_cache_base/atmos/toolchain/registry/https___raw.githubusercontent.com_aquaproj_aqua-registry_main_pkgs_cloudposse_atmos_registry.yaml.yaml"
atmos toolchain install
```

This removes only the cached default Atmos recipe, not installed tools. If using a
pinned/custom registry, advance its revision to one containing the fix and remove
that recipe's corresponding cache entry instead. Windows users should remove the
same recipe from their configured Atmos toolchain registry cache directory.

Mise currently embeds an Aqua snapshot. Coordinate its registry refresh after the
Aqua merge; an update to Aqua alone does not alter already released Mise binaries.
Mise's floating-registry setting can fetch upstream metadata, subject to its cache.
No change to Mise's signer verifier is expected: it already processes the exact
certificate identity in Aqua's Cosign options.

## Version boundary and trust contract

Starting with the first 1.235.0 RC, `cut` and `candidate` builds use
`.github/workflows/release-tag-build.yml`, dispatched at the reserved full tag.
The gate compares the prepared milestone version with 1.235.0. Earlier stable
releases and RCs, plus all nightly and feature builds, retain the existing flow.

For migrated releases, the certificate identity must be exactly:

```text
https://github.com/cloudposse/atmos/.github/workflows/release-tag-build.yml@refs/tags/{{.Version}}
```

`{{.Version}}` includes `v` and any RC suffix. The issuer remains
`https://token.actions.githubusercontent.com`. Binary versions, embedded source
SHA, checksums, signed release-source predicates, standard provenance, and SBOMs
remain independently verified.

## Parent and child workflow association

Preparation, authorization, tag/RC reservation, verification, and publication run
in the trusted parent workflow on `main`. The child has read-only repository and
Actions access plus OIDC/attestation permissions. It receives no publication App
key or release environment secrets. It invokes the existing composite build action
directly, so signing occurs in the tagged workflow rather than a reusable main
workflow.

The parent passes its run ID, immutable plan artifact ID, and a deterministic
request digest over repository, parent run, plan artifact, tag, and source SHA.
The original plan and RC reservation retain the parent run ID. The child checks:

- Exact tag ref, workflow ref, source SHA, and tagged workflow SHA.
- Parent repository, workflow, branch, approved actors, and artifact ownership.
- Current release eligibility, the original RC reservation, and the live tag SHA.
- The triggering actor again when a child is rerun.

The parent rejects ambiguous request matches, failed children, mismatched run
identity, missing or expired artifacts, and artifacts from another run or attempt.
It downloads by artifact ID and run ID. Verification explicitly pins the signing
workflow, workflow SHA, and authenticated provenance invocation ID to that child.

Parent and child concurrency groups differ: `atmos-release` and
`atmos-tag-build-<ref>`. Publication and completion explicitly require successful
upstream jobs while allowing the unused build path to be skipped.

## Retry behavior

Rerun the failed parent jobs to reuse the existing successful child build. If the
child failed, rerun that child with an authorized maintainer account first, then
retry the parent job. The parent collects only that child's current successful
attempt. Publication continues to use the existing sealed artifact checks.

If dispatch timed out or no child is visible on a parent retry, the workflow fails
closed instead of redispatching. Inspect the original request ID in Actions.
Duplicate child runs for one request block publication. Rerunning all parent jobs
creates a new plan artifact; this is deliberately not treated as the same request.
Do not create another stable release request or move/delete the reserved tag to
work around these checks.

## Installer compatibility gate and rollout

Before sealing staged artifacts, and again immediately before the final publication
checks, fetch Aqua's current `main` revision with caching disabled and load the
Atmos recipe pinned to that commit. Run the Atmos installer verifier against the
staged native binary and its local checksum/bundle sidecars. Require SHA256 and
Cosign, and record the Aqua revision in the job log and successful step summary.
Reject missing metadata, stale signer policy, checksum mismatches, and sidecars
outside the staged release. Network errors block publication.

Rollout order:

1. Merge the immediate Aqua repair and refresh consumers' cached/bundled metadata.
2. Merge the version-gated Atmos workflow changes before reserving a 1.235.0 tag.
3. Complete [Aqua PR #63298](https://github.com/aquaproj/aqua-registry/pull/63298) for stable/RC tags at `>= 1.235.0-0`, requiring
    the exact tagged identity above. Keep the main identity for 1.233.2 through
    1.234.x and for nightly/feature tags. Do not broaden trust to both identities
    for migrated stable releases. Refresh Mise's bundled registry as needed.
4. Run a real 1.235.0 RC through GitHub Actions. Inspect its SAN, issuer, tagged
    workflow SHA, source SHA, child invocation, and successful installer gate.
    Install it with Atmos and Aqua with verification enabled.
5. Publish 1.235.0 stable only after that live RC verification succeeds.

The fresh-registry gate intentionally prevents the first migrated RC from being
published while Aqua still trusts only the main workflow. Its staged child build
can be inspected before publication and reused after the registry fix merges.
Existing immutable releases are never re-signed or replaced.

## Validation status

The immediate repair was verified by native Aqua installs of 1.233.0,
1.233.2-rc.3, and 1.233.2 with checksum verification enabled, package validation on
six OS/architecture combinations, and 19 passing Conftest assertions. Atmos
1.233.0 also installed those versions with the proposed registry metadata and
signature verification enabled. The original signer policy fails the new release.

Automated migration tests cover the version boundary, tag/ref/SHA mismatches,
parent and artifact association, RC reservation, duplicate dispatch including
pagination, failed/missing children, successful retry reuse, wrong invocation,
stale installer signer policy, and checksum/bundle failures. Live GitHub OIDC
validation of a 1.235.0 RC remains a rollout requirement; local mocks do not prove
the emitted certificate identity.

The new installer gate was also exercised with real staged release assets and
Cosign against Aqua revision `c58fff4f5c5ca41f8c5c2f91d71988194e96e4ce`:
1.233.0 passed; 1.233.2 failed with the original certificate identity mismatch.
This demonstrates that the publication gate detects the incident before upload.
The native Aqua reader is used so versioned checksum and signature metadata remain
intact; a YAML regression test covers historical, stable, RC, nightly and feature
policies.
