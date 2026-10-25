# Release pipeline audit, preflight, and live progress

## Failures and corrections

The first attempt of run `40006399777` failed to mint a release App token because
its installation lacked Issues permission. The installation now grants Issues
write, satisfying preparation, verification and publication token requests.

The second attempt failed to load `release-prepare/action.yml`: the composite
manifest referenced `vars.ATMOS_GUARDED_RELEASES_ENABLED`, a workflow context that
was unavailable there. The same defect existed in verification and finalization.
All three actions now receive the setting through a required explicit input.
The authoritative `true` check remains intact.

The audit found repository release immutability disabled even though the release
environment's cutover flag was enabled. With operator approval, repository
immutability was enabled during this audit and read back as `enabled: true`.
Without an early check, the pipeline can
build and publish before noticing that the result is mutable. The operator CLI
now checks the live setting before dispatch and fails on disabled or unreadable
settings. GitHub's settings endpoint requires Administration read access, so this
check uses the operator's credentials; the release App gains no administration
privileges. Workflow-only triggers still require an administrator to enable and
verify immutability before setting the environment's cutover flag. The setting
and final publication are not atomic; the existing post-publication immutability
check and downstream distribution guard remain necessary.

The publisher also failed to reject a draft whose prerelease flag changed during
upload. Its final identity check now includes release classification; downstream
verification checks classification and the exact target commit as well.

## Operator progress

Both publication commands require `gh` before dispatch. The CLI continues to
correlate a unique request ID with its exact workflow run. Once discovered, it
runs `gh run watch RUN_ID --repo OWNER/REPO --exit-status` with terminal streams
attached instead of polling completion silently. Watch failures propagate;
interruption prints the exact run URL and reconnect command without cancelling
or resubmitting the workflow. Success still requires API verification of the
workflow conclusion and immutable published release identity.

Candidate preflight, dispatch and completion messages explicitly say release
candidate. The initial summary explains that preparation assigns the RC suffix.
Stable output and captured confirmation JSON retain their existing meanings.

## Audit coverage and fast regression checks

- Inspected the prepare/build/verify/publish/complete graph, exact-SHA checkouts,
  scoped App tokens, OIDC permissions, artifact-ID transfer, tag reservations,
  binary/package identity checks, signature/provenance checks, and upload retries.
- Checked nine local manifests and thirteen pinned upstream action input
  contracts against their exact upstream revisions. The provenance action does
  support the custom predicate inputs used here.
- Added local checks that reject implicit workflow contexts in release composite
  manifests, check guard forwarding, and recursively validate local action input
  contracts. CI runs these before installing GoReleaser for the build rehearsal.
  Ordinary workflow actionlint did not detect the invalid composite context.
- Exercised actual pinned GoReleaser 2.18.3 packaging for stable, candidate,
  nightly and feature tags using the existing fixture rehearsal, along with
  publisher failures, partial-upload recovery and full release package tests.

## Live rollout requirements and limits

- Repository immutable releases are now enabled and verified. Keep them enabled
  before dispatching an RC.
- Merge the workflow fix before a new dispatch. Retrying the old workflow run
  retains its original workflow revision and the invalid composite manifests.
- The feature-releases environment was also missing
  `ATMOS_GUARDED_RELEASES_ENABLED` at audit time. Feature releases remain disabled
  until that environment is deliberately configured after immutability cutover.
- No live release, tag or draft was created by this audit. Local rehearsals cannot
  prove production OIDC certificates, live attestation verification, immutable
  publication, or downstream Homebrew/Docker delivery. A real candidate run is
  still required after configuration and workflow rollout.

References: [failed workflow](https://github.com/cloudposse/atmos/actions/runs/40006399777),
[GitHub immutable-release settings API](https://docs.github.com/en/rest/repos/repos#check-if-immutable-releases-are-enabled-for-a-repository).
