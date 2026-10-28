# Fix: Terraform source refresh preserves runtime artifacts and source provenance

**Date:** 2026-10-28

## Summary

Source refresh now preserves Terraform runtime artifacts, reuses successful provisioning within
one command, and records the fetched source identity.

## Context

Terraform source provisioning previously replaced an existing vendored directory wholesale.
Changing a Git source version or refreshing a zero-TTL source could delete local state,
provider/module installations, generated files, and saved plans. A later plan could therefore
propose creating resources that already existed. A zero-TTL source could also refresh several
times within one CLI invocation when hooks and output lookups resolved separate component maps.

## Changes

Custom state detection streams top-level state metadata without a file-size cutoff.
Inspection I/O failures stop publication. Refresh locking resolves existing ancestor
symlinks so aliases of the same physical directory cannot publish concurrently.

Windows acceptance testing exposed that the cache lock used for publication is deliberately
best-effort on Windows. Source publication now uses `pkg/filelock` directly for native and
in-process exclusion on every platform. It retains the existing two-second acquisition bound
and lock path, observes caller cancellation while waiting, and does not impose that acquisition
deadline on the subsequent download. The legacy workdir migration guard checks the path type
before reading it: Windows can classify `ReadDir` on a regular file as not-found, which must
not allow Atmos to switch away from existing caller data.
The unused pre-receipt download wrapper is removed, and source progress setup passes its
expanded target options by pointer rather than copying them.

Terraform `source:` refresh and `terraform source pull --force` stage the complete download
before changing the live source. A `.atmos/source-files.json` receipt identifies files owned by
the fetched source. Refresh removes obsolete owned files, installs the new source, and preserves
files created by Terraform, Atmos, or hooks. The component directory and runtime artifacts stay
in place. Publication journals each touched source file and its provenance receipts; a copy,
publication, metadata, or cancellation failure rolls back the changed files. A rollback that
itself fails retains its journal and reports its location.

Protection includes nested module roots, configured local backend paths and workspace directories,
state backups, `.terraform`, canonical/per-instance provider locks, generated files, and saved
plans with custom filenames. Actual state/plan contents are recognized when they have replaced a
previously source-owned file. A source collision with a runtime artifact is rejected before
publication; provider locks remain under the lock restore/persist lifecycle. Active local state
lock markers and destination symlink traversal also prevent publication. Separate instances use
separate target locks; this does not introduce a universal scheduler for independent Terraform
processes sharing one mutable directory.

Legacy workdirs have no source ownership inventory. Their unknown files are retained, ordinary
incoming Terraform configuration is adopted as source, and ambiguous non-Terraform collisions
fail safely. This intentionally cannot prove that an unmatched legacy file is stale. Once an
inventory has been recorded, subsequent refreshes remove obsolete source-owned files. A source
fixture named `examples/terraform.tfstate` remains ordinary source content unless it is the
configured runtime location or contains actual state; the filename alone does not protect it.

Successful source provisioning is reused across hooks and nested lookups for the current CLI
invocation. The cache key includes the destination checkout, component instance, full source
specification, and authentication scope. Failed attempts are not cached; changed sources or
missing targets are reprovisioned. The next invocation starts a new scope, so zero TTL continues
to refresh on the next command. A reused result preserves the runtime's already-resolved module
subdirectory. Refresh retains `.terraform`; normal init fingerprinting decides whether changed
inputs require initialization or backend reconfiguration.

Git provenance now uses the commit captured by the downloader before `.git` is removed, including
Git URI subdirectories. Materialized content without its own verifiable Git root uses a
`sha256:` content identity. It no longer inherits the consuming repository's HEAD. Receipt fields
remain compatible, and shared-source receipts also expose the source identity and kind.

Explicit source workdirs additionally record the component **instance** as owner. The new
working-directory precedence behavior validates ownership both on warm reuse and again while
holding the publication lock. Two instances cannot silently claim the same explicit target.
Existing aliases whose legacy receipt records only the base component require an explicit
ownership migration; the safety check does not infer that another instance owns their state.
The working-directory precedence transition and edition gate are documented separately.

## Validation

Windows job `123115466755` reproduced concurrent publication through an ancestor alias,
two successful ownership claims on one explicit workdir, and acceptance of a regular file at
the legacy workdir path. The existing tests remain enabled. An additional regression checks
cancellation while waiting for publication, no target creation on failure, and successful retry.
The complete source and filelock suites pass locally, including the race detector; source
package lint reports zero issues and the Windows source test binary cross-compiles. Native Windows execution
of this correction still requires CI; cross-compilation alone cannot verify Windows locking.

Additional portable tests verify rollback after a backup failure, preservation when an owned
source file becomes a nonempty runtime directory, refusal of unverifiable explicit-workdir
ownership and receipt-path conflicts, and propagation of state-header read errors. Git sources
with unborn or invalid metadata fall back to an identity derived from the staged content. These
tests also assert unchanged state/source bytes on failure; no production behavior was changed
by this coverage follow-up.

Archived pre-fix code fails regression tests for deletion/replacement of valid custom
state larger than 64 MiB and for aliased concurrent ownership claims. The fixed code
passes these cases, a large non-state source control, the complete source suite, and
the complete source race suite. Source lint reports zero issues.

The reconciled PR and current main reproduced state loss, zero-TTL saved-plan failure, and wrong
Git provenance twice on both Terraform 1.15.6 and OpenTofu 1.12.5. Fixed real-engine repetitions
preserve nested local-state lineage, serial, resources and values while planning the actual
source change; receipts match the fetched commit. Zero-TTL saved-plan applies succeed after
stack variables change and retain the reviewed values. Further cases use nested modules,
`state/custom.bin`, and `reviewed/release.artifact`: forced source refresh preserves both file
hashes, and a failed Git refresh preserves source metadata, state and plan bytes.

Go regression tests cover source-owned deletion versus runtime preservation, unchanged runtime
inodes, injected publication failure and cancellation rollback, metadata failure, symlink and
active-lock refusal, saved-plan/state collisions, concurrent fresh component maps, failed-fetch
retry, authentication/checkout isolation, changed selectors, missing targets, and invocation
reset. Downloader tests exclude enclosing consumer repositories from provenance.

Evidence is retained in `.context/terraform-repair-20261028/sources/`. These local tests do not
establish authenticated private HTTP/OCI source behavior or Windows execution. The final
integrated acceptance suite and field matrix remain the PR-wide verification gate.

## Follow-ups

These source safety and provenance corrections do not change a configuration default or the
meaning of the configured source/version/TTL: source refresh continues to update source-owned
content, and zero TTL expires between commands. They repair unintended deletion of runtime
artifacts and incorrect receipts, so no destructive legacy code path is retained behind an
edition gate. The separate change to explicit working-directory precedence is edition-gated.

Legacy workdirs without ownership inventories retain unmatched files because their provenance
cannot be established safely. Authenticated private HTTP/OCI sources and Windows execution
remain outside this local field matrix.
