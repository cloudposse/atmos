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
- Dry-run's `spec.files[].matrix` expansion gap, see below -- tracked here, not with an
  issue.

## CodeRabbit follow-up round (PR #2989, commit `c27b6e5ec4`)

CodeRabbit left five substantive review threads against this fix once it was pushed as
PR #2989. Four were valid and are now fixed on this branch; one was investigated and
determined to be a pre-existing, repo-wide test pattern, not a defect introduced by this
PR.

1. **Base ref resolved before interactive target selection (`cmd/scaffold/scaffold.go`) --
    fixed.** `atmos scaffold generate --update` with no positional target used to resolve
    `defaultBaseRef` against the empty/cwd target *before* the interactive flow picked the
    real directory, so any pin at the real directory was silently discarded in favor of
    `HEAD`. A new `resolveInteractiveBaseRef` helper (and `ScaffoldUI.ResolveTargetPath`,
    exported alongside the existing `Execute*` methods so callers can resolve the real target
    directory first without prompting twice) now resolves the target directory before
    resolving the base ref against it. `shouldOfferScaffoldUpdate` similarly now takes the
    actual resolved `targetDir` as a parameter instead of reading the stale
    `opts.targetDir`. Regression tests: `TestExecuteTemplateWithoutTargetDir_UpdateResolvesBaseRefAfterInteractiveTarget`,
    `TestExecuteTemplateWithoutTargetDir_NoUpdateSkipsTargetResolution`,
    `TestShouldOfferScaffoldUpdate_UsesActualTargetDir` (`cmd/scaffold/scaffold_mock_test.go`,
    `cmd/scaffold/scaffold_coverage_test.go`).
2. **New tests depend on an external `git` binary
    (`cmd/scaffold/scaffold_coverage_test.go`, `pkg/generator/source/resolver_test.go`) --
    determined stale, not fixed.** Verified against `653e596ec6^` (the commit immediately
    before this fix landed): both `scaffoldRunGitCommand` (`cmd/scaffold`) and `requireGit`
    (`pkg/generator/source`) already existed and already used the same
    `exec.LookPath("git")`-and-skip pattern before this PR touched either file. This is an
    established, repo-wide convention predating the PR, not a regression it introduced --
    left as-is rather than a heavy-lift rewrite to remove the external dependency
    repo-wide.
3. **Unreadable metadata treated as absent (`cmd/scaffold/scaffold.go`'s `defaultBaseRef`)
    -- fixed.** `defaultBaseRef` used to fall back to `"HEAD"` on *any* `Load()` error,
    including a corrupt/unreadable metadata file, defeating the whole point of bug 1's pin --
    a damaged pin file would silently re-introduce the original silent-overwrite bug instead
    of surfacing the problem. `storage.MetadataStorage.Load()`'s actual contract returns
    `(nil, nil)` specifically when the file is absent; `defaultBaseRef` now propagates any
    other `Load()` error instead of swallowing it, which changes its signature to
    `(string, error)` (both call sites -- `shouldOfferScaffoldUpdate` and
    `resolveInteractiveBaseRef` -- updated to propagate the error). Regression tests:
    `TestDefaultBaseRef_PropagatesUnreadableMetadataError`,
    `TestShouldOfferScaffoldUpdate_PropagatesMetadataLoadError`
    (`cmd/scaffold/scaffold_coverage_test.go`).
4. **`Target`/`Matrix` not expanded before printing dry-run paths
    (`cmd/scaffold/scaffold.go`'s `collectDryRunFiles`) -- partially fixed, matrix expansion
    scoped out as a follow-up.** `collectDryRunFiles` always rendered a discovered file's own
    `Path`, ignoring `spec.files[].target` (a straight path override) entirely and never
    expanding `spec.files[].matrix` (one file into zero-or-more real outputs, per
    `pkg/generator/ui`'s `processMatrixedFileEntry`/`processMatrixRow`). This fix handles
    `Target`: `pkg/generator/ui`'s previously unexported `fileOutputPath` is now exported as
    `FileOutputPath` (alongside the existing `FileSpecByPath`/`ResolveDelimiters` exports for
    the same reason) and `collectDryRunFiles` calls it instead of always using `file.Path`.
    **Matrix expansion is explicitly not implemented** -- a template using `spec.files[].
    matrix` still previews exactly one unexpanded path (its `Target`/`Path` template with no
    matrix values bound), under- or over-counting relative to what a real run produces. This
    is documented directly in `collectDryRunFiles`'s doc comment as a known gap, plus here:
    the correct fix reuses `engine.ExpandMatrix` + `processMatrixRow`'s per-row `when`/path
    logic via a new exported "plan output paths for this file" helper from
    `pkg/generator/ui`, rather than re-implementing matrix expansion a second time in
    `cmd/scaffold`; not attempted in this round because it's a genuine heavy lift on top of
    four other fixes, and no issue has been filed for it (none requested). Regression test:
    `TestCollectDryRunFiles_HonorsTargetOverride` (`cmd/scaffold/scaffold_coverage_test.go`);
    no test claims matrix expansion works, matching the gap above.
5. **Positional metadata arguments on `PinInitialBaseRef` (`pkg/generator/gitinit.go`) --
    fixed.** `PinInitialBaseRef(targetPath, headSHA, templateName, templateVersion, source
    string) error` had three same-typed trailing string parameters, inviting an
    accidental-swap bug at the call site. `templateName`/`templateVersion`/`source` are now
    passed via functional options (`WithTemplateName`/`WithTemplateVersion`/`WithSource`,
    backed by a new unexported `pinOptions` struct and `PinOption` type, matching this
    package's existing options pattern in `options.go`); `targetPath`/`headSHA` stay
    positional since they're the operation's actual subject, not configuration. The one call
    site (`cmd/scaffold/scaffold.go`'s `maybeInitGeneratedGitRepository`) was updated.
    Regression test: `TestPinInitialBaseRef_NoOptionsStillWritesBaseRef`
    (`pkg/generator/gitinit_test.go`), confirming the options are genuinely optional.

### Validation (CodeRabbit follow-up round)

- `go build ./...` -- clean.
- `go vet ./...` -- clean.
- `go test -count=1 ./cmd/scaffold/... ./pkg/generator/...` -- all pass.
- `gofumpt -l` over every touched file -- clean (two files needed a `gofumpt -w` pass for
  multi-line call-argument formatting `gofmt` alone wouldn't have caught).
- `./build/atmos lint --changed` (patch-scoped against `origin/main`) -- 0 issues.
