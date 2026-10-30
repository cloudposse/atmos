---
name: releases
description: Plan, validate, cut, or troubleshoot Atmos releases and versioned milestones. Use for stable releases, release candidates, nightly/feature releases, release preview issues, and artifact identity checks.
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
2. Run `atmos release cut`. Atmos repeats preflight and asks
    **Do you want to cut X.Y.Z?** using its native confirmation UI, default No.
    Never supply confirmation environment variables on behalf of the operator.
3. Confirmation binds the version, milestone and SHA. Changes invalidate it.
    The command dispatches exactly once and follows its uniquely identified run.
    On success it prints the verified release identity and links.
4. If interrupted, inspect the printed workflow URL/request ID. Do not blindly
    dispatch another run. Use `atmos release cut --resume` for an unfinished
    production release. It shows the original tag's source and saved workflow
    identity for confirmation, reconnects if the run is active, or reruns failed
    jobs. It never dispatches a replacement workflow or modifies a published
    release. Conflicts deliberately fail without overwriting tags or assets.

The local `cut` flow is one Starlark script using native confirmation and shell
steps; Go owns release policy and GitHub operations. Resume requires the original
tag and an unexpired preparation artifact from an unambiguous production run.
It also works before a draft exists if preparation completed and saved its plan.
Missing/expired artifacts, changed eligibility, or conflicting identities require
inspection; do not bypass those checks or rebuild into a partially uploaded draft.
After publication, retry only failed completion/bookkeeping jobs in the original
run; `cut --resume` deliberately refuses published releases.

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

## Verification and rollout

Use `go test -tags=mage ./magefiles/release/...`, the local-action test workflow, and the real
pinned GoReleaser rehearsal. Local fixtures do not exercise live OIDC signing,
GitHub permissions, immutable publication, or Homebrew's external review/build.
Confirm those in a controlled GitHub rehearsal before enabling production.
Keep Go caches enabled. Never publish test releases to production as a test.
