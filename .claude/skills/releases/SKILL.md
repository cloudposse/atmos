---
name: releases
description: Plan, validate, cut, or troubleshoot Atmos releases and versioned milestones. Use for stable releases, release candidates, nightly/feature releases, release preview issues, artifact identity checks, and Aqua/Mise installer compatibility.
---

# Atmos releases

Release version comes exclusively from the lowest open stable-semver milestone.
Labels express compatibility; they never choose the version. Read
`docs/prd/release-orchestration.md` for architecture, rollout requirements, and limits.

## Operator path

1. Run `atmos release check` from the repository. It selects the next milestone,
    resolves the exact default-branch SHA, verifies included changes and requires
    every milestone PR to be merged and present in that SHA. Closed-unmerged PRs
    block: merge them or move them out. Informational issues may remain open.
2. Check Aqua installer compatibility as described below, especially after changes
    to signing workflows, workflow refs, asset names, or checksum/bundle layout.
    A successful internal signature check alone does not prove users can install.
3. Run `atmos release cut`. Atmos repeats preflight and asks
    **Do you want to cut X.Y.Z?** using its native confirmation UI, default No.
    Never supply confirmation environment variables on behalf of the operator.
4. Confirmation binds the version, milestone and SHA. Changes invalidate it.
    The command dispatches exactly once and follows its uniquely identified run.
    On success it prints the verified release identity and links.
5. If interrupted, inspect the printed workflow URL/request ID. Do not blindly
    dispatch another run. Retry the existing failed run with the same identity;
    conflicts deliberately fail without overwriting tags or assets.

`atmos release candidate` allows unfinished milestone PRs, uses a unique
`-rc.N` tag (starting at 1 and increasing per version), and leaves the milestone open.
Preparation reserves the number; retries recover the same run's reservation.
Failed builds consume their reserved number. Scheduled nightly and manual
feature workflows use the same verification/publishing pipeline. Feature releases
are dispatched with a PR number and never create release preview issues.

Actions run titles show the release type/version and the CLI correlation ID.
After preparation, job names and summaries show the exact reserved tag. Each
executed stage summarizes success or failure, checkpoint outcomes, source SHA,
and artifact links. A skipped job cannot write a summary.

## Milestones and pull requests

- Every PR needs exactly one of `major`, `minor`, `patch`, `no-release`.
- Except `no-release`, every PR needs a versioned milestone. Only the lowest open
  milestone is merge-eligible. Patch changes fit minor/major milestones; minor
  changes cannot go into a patch milestone; major changes need a major milestone.
- Run `atmos release milestones ensure` to maintain the next three minor slots.
- Patch and major versions are explicit: `atmos release milestones create vX.Y.Z`.
- Run `atmos release milestones check` to reconcile statuses. The `Release milestone`
  status must be required by repository protection at cutover.
- Stable milestone preview issues are informational. Their hidden marker is an
  association, never authority. Comments/closing an issue do not cut releases.

## Workflow boundary

Actions repeats eligibility and authenticates Erik/Andriy by exact numeric user
ID, including rerun initiators. There is no dual approval. Build jobs receive OIDC and attestation permissions to sign with GoReleaser
and record build provenance, but no release publisher/App credentials. Fresh trusted jobs verify binary version, full
source SHA, clean VCS metadata, complete platform/package matrix, hashes,
signatures, and attestations before upload and publication. Homebrew and other
distribution jobs follow published-release verification. Homebrew builds its own
bottles from tagged source; these are not byte-identical to upstream binaries.

Do not edit a draft's version/source in the UI or publish it manually. Direct
publishing privileges can bypass workflow checks. Immutable releases prevent
post-publication changes only once enabled; they do not authorize publication.
Do not claim this role boundary is solved by a CI check.

## Aqua and Mise are release dependencies

This applies to releases now; it does not wait for the 1.235.0 tag-signing migration.
Read `docs/prd/release-tag-signing.md` for the version boundaries and recovery steps.

- Check upstream `aquaproj/aqua-registry` at
  `pkgs/cloudposse/atmos/registry.yaml`. It defines consumer download/checksum and
  signer policy. The neighboring `pkg.yaml` is Aqua's installation test matrix,
  not metadata consumed by Atmos. Pin historical fixtures with separate `name`
  and `version` fields so the updater cannot advance regression cases.
- Compare the actual staged certificate SAN and OIDC issuer with the rule selected
  for the full release tag. Creating/checking out a tag does not change the OIDC
  identity of a workflow dispatched from `main`. Do not substitute a tag identity
  until signing actually runs at that tag. Keep checking binary version, source
  SHA, checksums and signed provenance independently.
- Any signer, ref, asset or bundle change requires an upstream compatibility check
  and, if incompatible, a version-scoped Aqua PR before publication. Preserve old
  checksum-only releases and historical signers; test the previous release, the
  first affected RC, and the stable boundary. Stable/RC changes must not silently
  change nightly/feature policy. Do not disable verification or broaden the new
  rule to arbitrary workflows to make a release pass.
- The guarded pipeline runs `release:verifyInstaller` before sealing and again at
  publication, using staged bytes and a freshly resolved, commit-pinned upstream
  Aqua recipe. Record the revision and the verification result. If that pipeline
  change is not deployed yet, perform the equivalent staged check manually.
  Stale/incompatible policy blocks publication: merge the registry correction,
  then rerun failed verification/publication jobs with the existing build. Never
  manually publish to bypass the check or replace an immutable release's assets.
- Verify real GitHub OIDC with the first migrated 1.235.0 RC before 1.235.0 stable.
  Its signer must be `release-tag-build.yml@refs/tags/<full-tag>`, including `v`.
  Earlier releases and nightly/feature builds keep their documented signers.

Mise's Atmos backend consumes Aqua too. Its verifier already supports the Cosign
identity options; a signing-policy change normally belongs in Aqua, not Mise's
verifier code. Check `jdx/mise/vendor/aqua-registry/metadata.json` and the included
Atmos recipe: Mise's release preparation (`xtasks/release-plz`) refreshes that
snapshot, while its contribution checks reject direct vendored-file edits. After
an Aqua merge, verify a Mise release includes it and test an installation with
verification enabled. If propagation stalls, coordinate a maintainer refresh.
Open a Mise code PR only for a demonstrated parser/verifier incompatibility.
An Aqua merge alone does not repair already installed Mise binaries or cached
recipes. Floating/custom registry users must refresh downloaded metadata too.

Release handoff must record Aqua PR/revision, install verification results,
Mise snapshot/release coverage (or the remaining propagation dependency), and how
to remove temporary overrides and refresh only the affected cached recipe. Do not
claim the incident is resolved for every consumer merely because our own gate passes.

## Verification and rollout

Use `go test -tags=mage ./magefiles/release/...`, the local-action test workflow, and the real
pinned GoReleaser rehearsal. Local fixtures do not exercise live OIDC signing,
GitHub permissions, immutable publication, or Homebrew's external review/build.
Confirm those in a controlled GitHub rehearsal before enabling production.
Keep Go caches enabled. Never publish test releases to production as a test.
