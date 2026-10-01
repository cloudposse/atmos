# Fix: `workdir clean --all --dry-run` deletes everything, and workdir docs describe the pre-hash naming format

**Date:** 2026-08-24

## Summary

`atmos terraform workdir clean --all --dry-run` silently ignored `--dry-run` and deleted every
workdir for real, printing a misleading "cleaned" success message instead of a preview.
`CleanAllWorkdirs` now accepts and honors a `dryRun` parameter, matching the sibling
`CleanExpiredWorkdirs` function's existing behavior. Separately, every published doc page and
several blog posts describing the workdir directory-naming format (`website/docs/stacks/components/provision/workdir.mdx`,
`website/docs/cli/configuration/settings/provision.mdx`, all four
`website/docs/cli/commands/terraform/workdir/*.mdx` pages, and three blog posts) still showed the
pre-hash-suffix format (`<stack>-<component>`, e.g. `dev-vpc`) and, in several cases, CLI output
that no longer matches the real command at all. Both were found during a field-test pass of PR `#2985`
(`osterman/fix-workdir-h-char-injection`, the workdir hash-suffix naming change).

## Context

A field-test pass of PR #2985 was asked to verify the branch's new `<stack>-<component>-<hash>`
workdir naming scheme end-to-end. While updating `website/docs/cli/commands/terraform/workdir/clean.mdx`'s
example output against the real CLI, `atmos terraform workdir clean --all --dry-run` was found to
actually delete `.workdir/` despite `--dry-run` being passed -- reproduced twice, 100%
consistent. Root cause: `pkg/provisioner/workdir/clean.go`'s `CleanAllWorkdirs` took no `dryRun`
parameter at all (unlike `CleanExpiredWorkdirs`, which already had one), and
`cmd/terraform/workdir/workdir_clean.go`'s `RunE` never threaded the parsed `--dry-run` flag into
the `--all` branch. A second, independent `CleanAllWorkdirs` implementation existed on
`cmd/terraform/workdir/workdir_helpers.go`'s `DefaultWorkdirManager` (the one the real CLI command
actually calls) with the identical gap, and it also only removed `.workdir/terraform/`, not the
full `.workdir/` base the docs and `provWorkdir.CleanAllWorkdirs` describe.

This is pre-existing on `main` -- `pkg/provisioner/workdir/clean.go` and
`cmd/terraform/workdir/workdir_clean.go` have zero diff in PR #2985 -- not something introduced by
the naming-scheme change under test. It was fixed directly at the user's request once flagged,
rather than left for a separate pass, per the field-test skill's fix-log handoff.

The stale docs are a direct consequence of PR #2985 changing the naming format without updating
any of the pages describing it, and were confirmed independently (not just by field-test research)
against live CLI output built from the current worktree.

## Changes

**`--all --dry-run` bug:**
- `pkg/provisioner/workdir/clean.go`: `CleanAllWorkdirs` now takes a `dryRun bool` parameter. On a
  dry run it lists every workdir it would remove (via new helper `listAllWorkdirNames`) and
  returns without deleting. `Clean`'s `case opts.All` now passes `opts.DryRun` through (it
  previously dropped it on the floor, identically to the CLI-level bug).
- `cmd/terraform/workdir/workdir_helpers.go`: `WorkdirManager.CleanAllWorkdirs` interface and
  `DefaultWorkdirManager.CleanAllWorkdirs` now take `dryRun bool`. The default implementation was
  reimplementing removal inline (and scoped to `.workdir/terraform/` only); it now delegates to
  `provWorkdir.CleanAllWorkdirs`, the same delegation pattern `CleanExpiredWorkdirs` already used,
  which incidentally also fixes the terraform-only scoping to match the documented "clean
  everything under `.workdir/`" behavior.
- `cmd/terraform/workdir/workdir_clean.go`: `RunE` now passes the parsed `dryRun` flag into
  `cleanAllWorkdirs`, which forwards it to the manager and skips the "All workdirs cleaned" success
  message on a dry run (that message previously printed unconditionally, which would have kept
  lying even after the underlying delete was fixed).
- Regenerated `cmd/terraform/workdir/mock_workdir_manager_test.go` via `go generate` (mockgen) for
  the interface signature change.
- Updated every existing call site across
  `pkg/provisioner/workdir/{clean_test.go,integration_test.go}` and
  `cmd/terraform/workdir/{workdir_helpers_test.go,workdir_clean_cmd_test.go,workdir_integration_test.go}`
  to pass the new parameter, and added new tests: `TestCleanAllWorkdirs_DryRun` and
  `TestListAllWorkdirNames`/`TestListAllWorkdirNames_MissingBase` (package `workdir`),
  `TestDefaultWorkdirManager_CleanAllWorkdirs_DryRun` (package `cmd/terraform/workdir`), and
  `TestCleanAllWorkdirs_DryRun`/`TestCleanAllWorkdirs_DryRunSuppressesSuccessMessage` (CLI-level,
  mock-based) -- each asserting the workdir(s) survive a dry run and/or that the manager receives
  the correct `dryRun` value.

**Stale docs:**
- `website/docs/stacks/components/provision/workdir.mdx`: format string and directory-layout
  example now show the `<hash>` suffix, with a sentence explaining what it's for.
- `website/docs/cli/configuration/settings/provision.mdx`: same format-string fix.
- `website/docs/cli/commands/terraform/workdir/list.mdx`: example table/JSON output rewritten to
  match the real `COMPONENT STACK TYPE VERSION LAST_ACCESSED PATH` columns and real JSON field set
  (the previous example used a `WORKDIR/COMPONENT/STACK/SOURCE/CREATED/UPDATED` table and JSON
  shape that no longer exist in the CLI at all, independent of the hash-suffix change).
- `website/docs/cli/commands/terraform/workdir/show.mdx`: example output and field list now
  include `Source Type`/`Last Accessed` (present in real output, missing from the doc) and drop
  the false "partially masked for security" claim about Content Hash (no masking exists in the
  code).
- `website/docs/cli/commands/terraform/workdir/describe.mdx`: removed a fabricated `--format json`
  example -- `workdir describe` has no `--format` flag at all -- and fixed the YAML example to
  match real output. Fixed a "Troubleshooting" example that grepped for `enabled: true`, a string
  that no longer appears anywhere in real `describe` output.
- `website/docs/cli/commands/terraform/workdir/clean.mdx`: all example output (specific clean,
  `--all`, `--all --dry-run`) rewritten to match real CLI output, including the now-fixed
  `--all --dry-run` behavior.
- `website/blog/2025-12-28-component-workdir-isolation.mdx`,
  `website/blog/2025-12-30-terraform-source-provisioner.mdx`,
  `website/blog/2026-02-05-version-aware-jit-provisioning.mdx`: directory-name examples and format
  strings updated to include the hash suffix; the `2026-02-05` post's TTL-cleanup example output
  also corrected to match real wording (`Cleaning N expired workdir(s)...`, not `Cleaning expired
  workdirs...`).
- Deliberately left unchanged: `website/blog/2026-08-17-container-config-validation-and-workdir-path-encoding.mdx`,
  a point-in-time changelog entry about an earlier, different naming fix -- rewriting it to
  describe this later change would misrepresent what that specific PR shipped. Also left
  unchanged: `docs/prd/component-workdir.md` (an internal design doc, not a published page).

## Validation

- `go build ./...` -- clean.
- `gofumpt -l pkg/provisioner/workdir/ cmd/terraform/workdir/` -- clean.
- `go vet ./pkg/provisioner/workdir/... ./cmd/terraform/workdir/...` -- clean.
- `go test ./pkg/provisioner/workdir/... ./cmd/terraform/workdir/...` -- all pass.
- Live CLI reproduction before and after the fix, using a real build (`atmos build`) against the
  `tests/fixtures/scenarios/workdir` fixture: confirmed `--all --dry-run` deleted `.workdir/`
  before the fix, and confirmed after the fix it reports `Dry run: would clean N workdir(s):` with
  every workdir left on disk, while a subsequent real `--all` (no `--dry-run`) still deletes
  correctly.
- Every literal example in the edited doc/blog pages (workdir names, hashes, command output) was
  generated from real `atmos terraform plan`/`workdir list`/`workdir show`/`workdir describe`/
  `workdir clean` runs against the fixture, and hashes cross-checked against an independent Python
  `hashlib.sha256` calculation of `workdirPathHash`'s formula.
- `website && npm run build` -- succeeds (`[SUCCESS] Generated static files in "build"`); the only
  warnings present are pre-existing and unrelated to the edited files (an MDX-normalize fallback on
  three unrelated pages, and a broken anchor on an unrelated changelog page).
- `cmd/terraform/workdir/mock_workdir_manager_test.go` was regenerated via `go generate` (mockgen),
  not hand-edited.

## Follow-ups

None.
