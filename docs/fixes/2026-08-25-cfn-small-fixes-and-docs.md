# Fix: backend-delete warning text, `--include-dependents` validation, `validate` success message, docs drift

**Date:** 2026-08-25

## Summary

Four small, independent fixes bundled together: `backend delete`'s destructive-action warning
referenced "Terraform state file(s)" even for CFN's artifact buckets; `--include-dependents` silently
no-op'd when passed without `--affected`; `validate` produced zero output on success; and two
sections of `aws-cloudformation.mdx` had drifted from the actual implementation.

## Context

All four found during the same field-test pass on `atmos aws cloudformation`.

## Changes

**Backend-delete warning text** (`pkg/provisioner/backend/s3_delete.go`): the `.tfstate`-suffix
count and "Terraform state file(s)" wording is shared code — used by both `atmos terraform backend
delete` and `atmos aws cloudformation backend delete` via the same registered `DeleteS3Backend`
function, so the fix couldn't simply remove the wording (still correct for Terraform's own callers).
Added a `stateFileTagging{Suffix, Label string}` parameter, read from the raw `backend_config` map's
new optional `state_file_suffix`/`state_file_label` keys via `stateFileMarkers()`, defaulting to
Terraform's existing `.tfstate`/"Terraform state file(s)" when absent (zero behavior change for every
existing Terraform caller). CFN's `BuildSyntheticBackendConfig`
(`pkg/component/aws/cloudformation/backend.go`) now sets both to `""`, suppressing the sub-count
entirely for CFN's artifact buckets. `deleteBackendContents`'s resulting 6-argument signature was
bundled into a `deletionCounts` struct to stay under this repo's 5-argument function limit.

**`--include-dependents` without `--affected`** (`cmd/aws/cloudformation/cloudformation.go`): new
`validateLogsFollowChart`-sibling `validateIncludeDependents`, called from `validateOperationArgs`,
rejects the combination with a new sentinel `ErrAwsCloudFormationIncludeDependentsRequiresAffected`
— `graphSelectionForBulk` (`executor_bulk.go`) only ever reads this flag inside its `--affected`
branch, so passing it with `--all` or tags/labels-only previously did nothing silently.

**`validate` success message** (`pkg/component/aws/cloudformation/validate.go`): `validateTemplate`
now takes a `stackName` parameter and prints `ui.Success("<stack>: template is valid")` after a
successful `ValidateTemplate` call — previously the only output on success was the experimental-
feature banner, indistinguishable from a hang short of checking the exit code.

**Docs** (`website/docs/stacks/components/aws-cloudformation.mdx`):
- The `provision` entry in "Available Configuration Sections" now names all three `apply --target`-
  selectable kinds (`aws/s3`, `git`, direct-deploy default) plus the `aws/stackset` not-via-`--target`
  caveat, cross-referencing the "Delivery Targets" section instead of re-summarizing it (so the two
  can't drift apart again).
- Added a "Delivery Targets" → `kind: aws/s3` note documenting `provision.backend.enabled: true`
  (previously undocumented on this page at all, despite `from-rain.mdx` already describing it and
  this session having implemented the real behavior — see
  `2026-08-25-cfn-backend-auto-provisioning.md`).
- The hooks section rewrote its verb list from an incomplete Phase-1-era enumeration (6 verbs) to
  the general rule: only `diff`/`apply`/`delete` fire hook events, every other verb (named
  explicitly) does not.

## Validation

- New tests: `TestValidateOperationArgs_RejectsIncludeDependentsWithoutAffected/
  AcceptsIncludeDependentsWithAffected`; `TestValidateTemplate` (updated to assert the new success
  message via `captureStderr`); `TestShowDeletionWarning_EmptyLabelSuppressesStateFileMention`,
  `TestStateFileMarkers_DefaultsWhenAbsent/CallerOverride`; `TestBuildSyntheticBackendConfig` extended
  to assert the new override keys.
- `go test ./pkg/component/aws/cloudformation/... ./cmd/aws/cloudformation/... ./pkg/provisioner/backend/...`
  — all pass.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding remains).
- `cd website && npm run build` — succeeded, no broken links/MDX errors.

## Follow-ups

None.
