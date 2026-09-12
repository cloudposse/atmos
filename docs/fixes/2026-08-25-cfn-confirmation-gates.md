# Fix: `backend create`/`update` and `changeset delete` now require confirmation

**Date:** 2026-08-25

## Summary

Two mutating `atmos aws cloudformation` verbs had no confirmation gate, unlike every sibling
mutating verb (`apply`, `delete`, `changeset execute`, `stackset create/update/delete`):
`backend create`/`update` silently overwrote an *existing* bucket's encryption, versioning,
public-access, and tags with no prompt (the warning printed only *after* the overwrite already
happened), and `changeset delete` never prompted at all, even without `--auto-approve`.

## Context

Found during a field-test pass. `backend create` run twice against a live Floci bucket confirmed the
overwrite happens unconditionally: `⚠ Applying Atmos defaults to existing bucket ...` prints as a
post-hoc warning, not a pre-action prompt. `changeset delete` deleted immediately with no prompt in
either case, live-verified.

## Changes

**`changeset delete`** (`pkg/component/aws/cloudformation/confirm.go`): added
`OperationChangesetDelete` to `confirmedOperationVerbs` — the same gate `changeset execute` already
uses. `--changeset-name` was already required; `--auto-approve` was already registered on this verb.

**`backend create`/`update`** (`cmd/aws/cloudformation/backend/`):
- Added a new `BackendExists(ctx, params) (bool, error)` method to the `Provisioner` interface
  (`backend_helpers.go`), implemented via the same `ResolveS3BackendTarget`/`DescribeS3BackendTarget`
  path `backend describe` already uses. Mock regenerated via this package's existing
  `//go:generate mockgen` directive.
- Added `--auto-approve` to both `createCmd`/`updateCmd`.
- Added `confirmExistingBackendOverwrite` (`backend_create.go`, shared by both commands via
  `executeCreateOrUpdate`): when the target bucket already exists and `--auto-approve` isn't set,
  prompts via `pkg/flags.PromptForConfirmation` — the same shared helper `source delete` already
  uses — *before* `prov.CreateBackend` runs, not after. A fresh create (bucket doesn't exist yet)
  never prompts; nothing to confirm.
- `executeCreateOrUpdate`'s five string/bool parameters were bundled into a `createOrUpdateArgs`
  struct to stay under this repo's 5-argument function limit (`argument-limit` lint rule).

## Validation

- New tests: `TestRequireConfirmation_ChangesetDeletePrompts/AutoApproveSkipsPrompt/DeclinedAborts`;
  `TestConfirmExistingBackendOverwrite_AutoApproveSkipsCheck/DoesNotExist_NoPrompt/
  BackendExistsError/Exists_ReachesPrompt` (the last proven via `ErrInteractiveNotAvailable`
  surfacing in the non-TTY test environment — proof the real prompt code path is reached, not
  silently skipped).
- `go test ./pkg/component/aws/cloudformation/... ./cmd/aws/cloudformation/...` — all pass.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding remains).

## Follow-ups

None.
