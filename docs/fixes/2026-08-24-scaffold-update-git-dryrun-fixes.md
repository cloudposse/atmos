# Fix: `atmos scaffold` `--update` silent overwrite, `.git` leakage, and `--dry-run` mismatch

**Date:** 2026-08-24

## Summary

Three bugs reported by a client evaluating `atmos scaffold` (`[EXPERIMENTAL]`) against
`v1.226.0`:

1. `scaffold generate --update` could silently discard a customization the user had
    already committed to the generated project, as long as the template's own change
    landed on a different line.
2. A `.git` directory present in a template source (a template directory that was itself
    cloned, or a `git::` remote source fetched into a fresh temp dir) was copied wholesale
    into generated output.
3. `--dry-run` could list/count more files than a real (non-dry-run) generation actually
    produces: directory entries were counted as files, `spec.files[].when: false` was
    ignored, and file paths using a template's custom `spec.delimiters` rendered raw and
    unsubstituted.

A fourth reported bug (hardcoded `{{ }}` delimiters in three call sites: `ProcessTemplate`,
`validateRenderedPath`, `mergeFile`) is explicitly out of scope for this fix — the client
has their own patch and will submit it separately as a PR.

None of the four bugs had an existing GitHub issue or PR tracking them.

## Context

The client's report included exact repro commands run against a `v1.226.0` binary and file
references against that tag. Investigation confirmed all three in-scope bugs by reading the
code on this branch (which already differs from `v1.226.0` in unrelated ways, but the same
defects were present):

- Bug 1's root cause: `--update`'s default base ref (`cmd/scaffold/scaffold.go`'s
  `defaultBaseRef`) always resolved to live `HEAD`. Once a user's customization was
  committed, git saw it as part of the "base," so the 3-way merge
  (`pkg/generator/engine/merge_update.go`) treated it as unchanged from base and let the
  freshly rendered template win with no conflict.
- Bug 2's root cause: `pkg/generator/templates/embeds.go`'s `readTemplateFiles` walked
  every subdirectory with no name-based exclusion, and the go-getter `clone()` path used
  for scaffold's fresh temp-dir fetches never strips `.git` (only a separate, unused-by-
  scaffold `update()` git-getter branch does).
- Bug 3's root cause: `cmd/scaffold/scaffold.go`'s `renderDryRunFileList` had three
  independent defects: `renderFilePath` passed `nil` for the scaffold config so custom
  delimiters were never applied; it never checked `file.IsDirectory`; and it never
  evaluated each file's `spec.files[].when` condition, unlike real generation
  (`pkg/generator/ui`'s `processSingleFileEntry`).

## Changes

**Bug 2 — `.git` exclusion** (`pkg/generator/templates/embeds.go`): `readTemplateFiles`
now skips any directory entry named `.git` before recursing, via a new
`excludedTemplateDirNames` map. This protects both `LoadConfigurationFromDir` (local
directory templates) and remote `git::`-fetched templates, since both funnel through the
same reader.

**Bug 3 — dry-run parity** (`cmd/scaffold/scaffold.go`): `loadDryRunValues` now also
returns the parsed `*config.ScaffoldConfig` (nil when the template has no `scaffold.yaml`).
A new `collectDryRunFiles` helper mirrors real generation's own selection rules exactly:
skips the `scaffold.yaml` manifest and `IsDirectory` entries, and gates each remaining file
by `spec.files[].when` (via `config.FileSpec`, looked up through the newly exported
`generatorUI.FileSpecByPath`). `renderFilePath` now resolves the template's own delimiters
(via the newly exported `generatorUI.ResolveDelimiters`) and renders through
`ProcessTemplateWithDelimiters` instead of always defaulting to `{{ }}` via
`ProcessTemplate` -- deliberately bypassing `ProcessTemplate` itself (whose hardcoded
delimiter default is bug 4, out of scope) rather than fixing it.

**Bug 1 — `--update` base ref pinning** (`pkg/generator/gitinit.go`,
`cmd/scaffold/scaffold.go`, `pkg/generator/storage/metadata.go`): `InitGitRepository` now
also returns the resolved SHA of the initial commit it creates (empty when it skipped
because the target was already a git repo). A new `PinInitialBaseRef` persists that SHA to
`.atmos/scaffold/metadata.yaml` (via the pre-existing but previously never-wired-up
`storage.GenerationMetadata`/`MetadataStorage` types, plus a new `ScaffoldMetadataPath`
helper) immediately after that first commit -- the one guaranteed moment atmos itself knows
a commit contains pristine, unmodified generated content. `defaultBaseRef` now takes
`targetDir` and, when the caller passed no explicit `--base-ref`, prefers this pinned SHA
over live `HEAD`; an explicit `--base-ref` still always wins, and targets with no pin
(pre-fix scaffolds, or `--no-git` targets where atmos never controlled the initial commit)
still fall back to `HEAD`, unchanged from today. Because the pin is written once, at the
one commit `InitGitRepository` itself creates, and never overwritten by later `--update`
runs (atmos doesn't auto-commit on update), every future `--update` diffs against the same
true pristine baseline regardless of what's committed afterward.

Scope note: this fix only helps when `--git` was used at initial generation (the default).
A target where the user brought their own pre-existing git repo and ran the first generate
with `--no-git` has no reliable "pristine content" commit atmos can identify, so `--update`
there still falls back to live `HEAD` -- unchanged from before this fix, not a regression.

## Validation

- `go build ./...` -- clean.
- `go vet ./...` -- clean.
- `go test ./pkg/generator/... ./cmd/init/... ./cmd/scaffold/...` -- all pass.
- `atmos lint --changed` (patch-scoped against `origin/main`) -- 0 issues (one `godot`
  finding fixed along the way).
- Every new/changed test was confirmed to fail before its corresponding fix and pass after,
  per CLAUDE.md's bug-fixing workflow, including two explicit mutation checks (temporarily
  reverting the `.git`-exclusion and the base-ref-pin-preference logic and re-running the
  affected tests to confirm they catch the regression).
- `./build/atmos test` (full suite, all packages): one failure, in
  `github.com/cloudposse/atmos/tests` (`TestCLICommands`, unrestricted). The captured
  output is a goroutine dump from a timeout inside `simulateTtyCommand`
  (`tests/cli_test.go:681`, blocked in `os/exec.Cmd.Wait`), with sibling goroutines blocked
  on TLS/HTTP2 reads -- a subprocess-based CLI test hung waiting on a network-dependent
  preflight, in a subsystem unrelated to scaffold/generator code. This matches a
  previously-known local-environment limitation (emulator/network preflight hangs when
  Docker/Podman isn't running locally; CI is unaffected), not a regression from this fix.
  Re-running the CLI-level scaffold suite in isolation
  (`go test ./tests/... -run 'TestCLICommands/scaffold' -v`) -- all 10 scaffold snapshot
  subtests (`scaffold-basic`, `scaffold-matrix-list`, `scaffold-matrix-computed`,
  `scaffold-matrix-freetext`) -- passed cleanly, confirming this fix does not affect the
  scaffold CLI-integration surface.
- Manual re-run of the client's exact repro commands against a locally built binary was not
  performed in this session (no client-provided fixture template was available); the
  automated reproduction tests in `cmd/scaffold/scaffold_coverage_test.go` (
  `TestExecuteTemplateGeneration_UpdateFlag_PreservesCommittedEdit`,
  `TestLoadConfigurationFromDir_ExcludesGitDirectory`,
  `TestResolve_RemoteGitExcludesGitDirectory`, `TestCollectDryRunFiles_ExcludesDirectories`,
  `TestCollectDryRunFiles_RespectsWhenCondition`, `TestRenderFilePath_CustomDelimiters`)
  reconstruct the reported scenarios directly from the bug report's own repro steps.

## Follow-ups

- Bug 4 (hardcoded `{{ }}` delimiters at `pkg/generator/engine/templating.go`'s
  `ProcessTemplate`/`validateRenderedPath` and `merge_update.go`'s `mergeFile`) is
  intentionally not fixed here; the client is submitting their own patch as a follow-up PR.
  No issue filed (none requested).
- `atmos init` shares `gen.InitGitRepository` with `atmos scaffold` and likely has the same
  underlying `--update` base-ref staleness issue, but pinning was only wired into
  `cmd/scaffold/scaffold.go`'s `maybeInitGeneratedGitRepository` per the approved fix scope.
  Not tracked with an issue; flagging here only.
- The `--no-git` / bring-your-own-git-repo case for bug 1 remains unfixed (see Scope note
  above) -- would need a content-hash-based snapshot independent of git history to close.
  Not tracked with an issue.
