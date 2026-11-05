# Quality Gate: current release evidence and status recovery

## Problem

PR #3537 passed the live release-policy gate and had a successful `Release milestone`
commit status, but remained not ready because a previous `Release milestones`
workflow was cancelled. That workflow reconciles repository-wide state; a replacement
run can publish the correct status without replacing the old workflow on this PR's head.
Its preview jobs can also fail independently of the PR's release eligibility.

Scheduled recovery previously checked old ready reports for failing statuses, but
skipped old not-ready reports after their workflows finished. A recovered external
status could therefore leave a stale verdict indefinitely without a new commit.

## Resolution

Exclude the trusted release-reconciliation workflow from aggregate CI results and
completion waiting. Exclude its checks only when GitHub Actions ownership and known,
unshared check-suite identity establish their origin. Unknown suites, other apps,
PR-triggered workflows, and actual CI failures remain blocking. The live release-policy
gate and current `Release milestone` commit status remain enforced.

Reports record their blocking head and synthetic-merge commit statuses in a hidden JSON
marker. The existing 15-minute recovery schedule compares those contexts with the current
combined statuses, including pagination. A changed or removed status selects the PR for
a full reevaluation; it does not grant readiness. Unchanged blockers do not select it.
Old reports containing release-workflow results or commit-status evidence get one refresh
to adopt the new format. Reports blocked only on reviews, signatures, or conflicts incur
no additional status reads.

There are no workflow-trigger, concurrency, queue, or quality-requirement changes.
This takes effect after the trusted reporter code reaches the default branch. Existing
affected reports become eligible on a subsequent scheduled run, subject to Actions delays
and the reporter's existing run budget.

## Verification

Regression tests fail before the fix and pass after it. Coverage includes obsolete
cancelled/failed/queued release runs, a subsequent failing milestone status revoking
readiness, unrelated and ambiguous checks, old-head status recovery, unchanged blockers,
API errors, pagination, merge statuses, legacy reports, and idempotent publication.

```shell
go test -race -tags mage ./magefiles/qualitygate
go test -tags mage ./magefiles/...
```
