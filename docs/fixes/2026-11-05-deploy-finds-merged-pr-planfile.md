# Fix: preserve stored-plan verification across rewritten commits and merge queues

**Date:** 2026-11-05 (extended 2026-11-06)

## Problem

Planfiles are keyed by stack, component, and the commit actually checked out when planning.
Deployment may run on a different SHA: a squash/rebase result, a synthetic PR merge commit,
a merge-queue commit, or the commit that eventually lands on the base branch. This is a
GitHub workflow and merge-strategy problem, independent of Atmos Pro.

The original fix covered PR-head plans followed by post-merge deploys. It excluded queue
runs and could not find plans generated on synthetic commits when those commits were rewritten.

## Resolution

Plan selection now follows this order:

1. Exact checked-out commit. Existing artifacts win with no additional lookups.
2. Identical source tree. Automatic uploads on capable providers publish a small reference
    to the original artifact under a repository/server/stack/component-scoped tree key.
    This bridges synthetic-to-landed commit rewrites without copying plan data. The original
    artifact must still exist; a reference cannot resurrect a deleted or expired plan.
3. A checkout-bound PR revision:
    - Post-merge: the unique merged PR whose `merge_commit_sha` matches the checkout.
    - Queue: the PR head from an active GitHub queue entry whose `headCommit.oid` matches
      the checkout. Removed or rebuilt entries do not supply a mutable latest head.
    - PR event: the event's head revision, provided the checkout matches that head or the
      event's synthetic merge commit.

Only artifact-not-found permits moving to another source. Storage permission, integrity,
and transport errors stop selection. Lookup errors are logged; the existing missing-plan
policy determines whether deployment may proceed. Optional provider capabilities preserve
existing behavior for other CI providers. No Atmos Pro setting gates these features.

Deploy still generates a fresh plan and compares it using the configured plan-diff policy.
It does not replay the downloaded plan, combine plans from several PRs, or try alternatives
until one passes. A queue that combines changes to the same component may need a new plan
on its combined commit. `warn`, `off`, and `required` retain their existing meanings.

## Additional safeguards

- Clear the previous stored-plan file before downloading, so reused runners cannot accidentally
  verify against a leftover artifact after a failed lookup.
- Preserve the landed checkout's provider lockfile during verification instead of overwriting it
  with an older plan's dependency selections.
- Reject ambiguous merged-PR matches, including matches on separate API pages.
- Hide internal source-tree references from the public planfile list; bulk storage deletion can
  still clean them up.
- Bound source-resolution requests with a timeout. GitHub source and queue resolution require
  `contents: read` and `pull-requests: read`, respectively.

## Validation

Coverage includes the real GitHub provider against HTTP fixtures plus a real bundled local
artifact store for queue-to-PR lookup, upload-to-download flows across rewritten commits,
removed/rebuilt entries, exact-commit priority, Pro enabled/disabled, source-tree isolation,
reference upload failure, deleted originals, malformed references, storage errors, stale local
files, provider lockfile preservation, and list output compatibility. Existing deploy tests
exercise strict/warn/off verification and bulk deployment.

GitHub queue responses are fixtures; these tests do not replace a live merge-queue workflow
exercise. Tree references are published by new automatic plan uploads; existing plans remain
accessible through their original commit keys and PR-head resolution.
