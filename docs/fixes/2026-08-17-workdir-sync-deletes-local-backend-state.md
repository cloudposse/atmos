# Fix: workdir sync no longer deletes local-backend Terraform state on re-provision

**Date:** 2026-08-17

## Summary

`pkg/provisioner/workdir/fs.go`'s `SyncDir` (via `deleteRemovedFiles`) deletes any file in a
JIT workdir that isn't present in the source component directory, so the workdir tracks the
source tree exactly. `shouldSkipSyncFile`/`shouldSkipSyncDir` protected provider lock files and
the workspace-specific `terraform.tfstate.d/` directory from that, but not a plain
`terraform.tfstate` (or its `.backup`) at the workdir root — the default workspace's
local-backend state file. Since state is never part of a component's source tree,
`deleteRemovedFiles` treated it as an orphan and deleted it on every re-provision — every
`terraform plan`/`apply`/`test` after the one that created it. A local-backend component's
actual infrastructure history was silently destroyed on its second run.

## Context

Surfaced while fixing a CodeRabbit review round on PR #2879 (see the sibling fix doc,
`docs/fixes/2026-08-17-pr2879-coderabbit-round-workdir-and-bootstrap-fixes.md`), while writing a
regression test for that PR's legacy-workdir-migration fix. Initially deferred as a Follow-up
there since it's an unrelated bug in sync's delete logic, not a workdir-*path* issue — but Erik
confirmed this matches a recurring pain point documented in a dogfooding effort
(`bugs.md` in the `atmos-1-22-example-ci` workspace, bug #3: "Local backend paths for
source-provisioned nested components are fragile"), which the branch that doc lives on exists
specifically to unblock. Fixed directly rather than left as a tracked gap.

## Changes

- `pkg/provisioner/workdir/fs.go`: added `terraformStateFile` (`terraform.tfstate`),
  `terraformStateBackupFile` (`terraform.tfstate.backup`), and
  `terraformStateLockInfoFile` (`.terraform.tfstate.lock.info`) constants.
  `shouldSkipSyncFile` now also protects these three exact filenames (in addition to its
  existing `*.terraform.lock.hcl` suffix check), so both `syncSourceToDest` and
  `deleteRemovedFiles` — which already share this one function — leave them alone in both
  directions: never copied in from source (state should never legitimately exist there), and
  never deleted from the workdir as an "orphan."
- `docs/fixes/2026-08-17-pr2879-coderabbit-round-workdir-and-bootstrap-fixes.md`: updated its
  Follow-ups section to point here instead of describing this as open.

## Validation

- New: `TestSyncDir_PreservesLocalBackendState` (`pkg/provisioner/workdir/fs_test.go`) — unit
  level, mirrors the existing `TestSyncDir_PreservesLockFiles` shape: a workdir holding
  `terraform.tfstate`, `terraform.tfstate.backup`, and `.terraform.tfstate.lock.info` (none
  present in source) survives a `SyncDir` call intact.
- New: `TestServiceProvision_PreservesLocalBackendStateAcrossReprovision`
  (`pkg/provisioner/workdir/integration_test.go`) — end-to-end, reproducing the real symptom:
  provisions a workdir, writes `terraform.tfstate`/`.backup` into it (standing in for a real
  `terraform apply`), re-provisions exactly as a subsequent `plan`/`apply`/`test` would, and
  asserts the state survives instead of being silently deleted. Both tests fail against the
  pre-fix `shouldSkipSyncFile` (confirmed by temporarily reverting just the new filename checks)
  and pass post-fix.
- `go build ./...`, `go vet ./...` — clean.
- `atmos lint --changed` (`./custom-gcl run --new-from-rev=origin/main`) — 0 issues.
- `go test ./pkg/provisioner/...` — all pass.

## Follow-ups

None. `.terraform/` itself (the provider plugin cache/working directory Terraform manages) was
already fully excluded from sync via `shouldSkipSyncDir`'s `terraformDataDir` check, so it isn't
part of this fix.
