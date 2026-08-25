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
- Dry-run's `spec.files[].matrix` expansion gap (originally flagged here as a follow-up) is
  now fixed -- see item 4 in the CodeRabbit follow-up round below. Superseded, not a live
  follow-up.
- ~~Hooks and `os.MkdirAll` run unconditionally regardless of dry-run~~ -- **fixed** in a
  later CodeRabbit round on this same PR (a `potential_issue` finding against this exact
  entry, since routing every `--dry-run` through the real generation path meant a scaffold
  template's hooks now ran their real side effects during what a user would reasonably
  expect to be a no-op preview). `executeWithSetup` now checks `!ui.processor.DryRun` before
  `os.MkdirAll`, `BeforeScaffoldGenerate`, `AfterScaffoldGenerate`, and
  `config.SaveProjectRecord` -- none of them run during a dry run. See
  `TestExecuteWithSetup_DryRunHasNoPersistentSideEffects` (`pkg/generator/ui/ui_test.go`).
- `ExecuteWithDelimiters`'s leading `"Generating %s in %s\n\n"` banner (`pkg/generator/ui/
  ui.go`) is not dry-run-aware either, unlike the summary line this round fixed -- it was
  already worded this way for the previously-shipped `--dry-run --update` path, so it's not a
  new regression, but it's the same class of "implies a write happened" wording gap. Left
  alone this round to keep the fix scoped to the summary line CodeRabbit's finding was about;
  worth a follow-up pass. Not tracked with an issue (none requested).

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
    (`cmd/scaffold/scaffold.go`'s `collectDryRunFiles`) -- fixed, superseding the partial
    `Target`-only fix originally landed here.** The previous round's `collectDryRunFiles`
    reimplemented real generation's file-selection rules a second time (skip `scaffold.yaml`
    and directories, gate by `spec.files[].when`, resolve `spec.files[].target`) but never
    expanded `spec.files[].matrix` at all -- a matrix-enabled file still previewed as exactly
    one unexpanded path, under- or over-counting relative to a real run's
    `processMatrixedFileEntry`/`processMatrixRow` output. Re-investigating for a full fix
    found that `--dry-run --update` (added later in this same fix round, see the "dry-run
    parity" entry above) had *already* solved this the correct way: instead of previewing
    file paths standalone, it drives the exact same real-generation call
    (`executeTemplateGeneration` -> `ScaffoldUI.ExecuteWithBaseRef` ->
    `pkg/generator/ui`'s `executeWithSetup`/`processFileEntry`/`processMatrixedFileEntry`/
    `processMatrixRow`/`writeOneOutput` -> `engine.Processor.ProcessFile`) with
    `engine.Processor.DryRun` set, so `ProcessFile` computes rendering and the 3-way merge
    but skips the final disk write. That path already got matrix expansion, `Target`,
    custom delimiters, and `spec.files[].when` for free, simply by being the real
    implementation rather than a parallel one.

    The fix: `executeScaffoldGenerate`'s `if opts.dryRun` branch no longer branches on
    `opts.update` at all -- both plain `--dry-run` and `--dry-run --update` now call
    `scaffoldUI.SetDryRun(true)` followed by `executeTemplateGeneration`. This let
    `renderDryRunPreview`, `renderDryRunHeader`, `loadDryRunValues`, `findScaffoldConfigFile`,
    `renderDryRunFileList`, `collectDryRunFiles`, `renderFilePath`, and `printFilePath` be
    deleted outright from `cmd/scaffold/scaffold.go` (along with the `condition`, `engine`,
    and `generatorUI` imports they were the only users of) -- there is now a single
    generation implementation instead of a hand-maintained preview shadowing it.

    Two consequences of routing through the real path, both correctness improvements over the
    old standalone preview:
    - `filesystem.ValidateTargetDirectory` now runs for every `--dry-run` (previously only
      for `--dry-run --update`): previewing against an existing, non-empty target directory
      without `--force`/`--update` now fails with the same `ErrTargetDirectoryNotEmpty` a
      real run would produce, instead of silently listing files regardless of the target's
      actual state. `ValidateTargetDirectory` returns `nil` immediately for a target that
      doesn't exist yet (`os.Stat` -> `os.ErrNotExist`), so the primary real-world use case --
      previewing into a directory that hasn't been created yet -- is unaffected.
    - `executeWithSetup`'s `os.MkdirAll(targetPath, ...)` initially ran during every
      `--dry-run` too (creating the empty target directory itself, though never any file
      inside it), matching the previously-shipped `--dry-run --update` path's existing
      behavior at the time. A later CodeRabbit round on this same PR flagged that as a
      real persistent-side-effect bug (see the Follow-ups entry above) -- `MkdirAll` is
      now skipped entirely in dry-run, alongside hooks and project-record persistence.

    Small independent improvement while in this code: `executeWithSetup`'s and
    `executeWithCommandValues`'s post-run summary lines (`"Generated %d files."` /
    `"Initialized %d files."`) unconditionally implied files were written even when
    `engine.Processor.DryRun` was true. Both are now dry-run-aware (`"Would generate %d
    files."` / `"Would initialize %d files."`, plus the equivalent `%d would fail.` error
    wording), extracted into two small, directly unit-testable functions
    (`generationSummaryLine`, `initializationSummaryLine` in `pkg/generator/ui/ui.go`) since
    `executeWithSetup`/`executeWithCommandValues` always flush and reset the UI output buffer
    before returning, which would otherwise make the exact wording unobservable from a test.

    Regression tests: `TestProcessFileEntry_DryRunMatrixExpansion`, `TestGenerationSummaryLine`,
    `TestInitializationSummaryLine` (`pkg/generator/ui/matrix_test.go`) prove matrix expansion
    (including the matrix-driven `target:` and the matrix-gated `when:` prune) is honored in
    dry-run mode with nothing written to disk, and pin the four summary-wording combinations.
    `TestExecuteTemplateGeneration_DryRunMatrixExpansion`,
    `TestExecuteScaffoldGenerate_DryRunNonexistentTargetDirectory`,
    `TestExecuteScaffoldGenerate_DryRunNonEmptyTargetWithoutForceOrUpdate_Errors`,
    `TestExecuteScaffoldGenerate_DryRunPropagatesInvalidScaffoldConfig`
    (`cmd/scaffold/scaffold_coverage_test.go`) prove the routing fix end-to-end: matrix
    expansion through the real `cmd/scaffold` entry point, the nonexistent-target-directory
    use case, the new "non-empty target without `--force`/`--update` now errors" behavior, and
    invalid-scaffold.yaml propagation. Tests that exercised the deleted standalone preview
    functions directly (`TestLoadDryRunValues*`, `TestFindScaffoldConfigFile`,
    `TestRenderFilePath*`, `TestCollectDryRunFiles*`, `TestRenderDryRunPreview*`,
    `TestPrintFilePath_WithoutTargetDir`) were removed since those functions no longer exist.
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

### Validation (matrix-expansion route-through round, item 4 above)

- `go build ./...` -- clean.
- `go vet ./...` -- clean.
- `go test -count=1 ./cmd/scaffold/... ./pkg/generator/...` -- all pass, including
  `./cmd/init/...` (unaffected: `atmos init` shares `pkg/generator/ui`'s `executeWithSetup`/
  `executeWithCommandValues`, whose summary-wording change is dry-run-gated and therefore a
  no-op for `init`'s existing non-dry-run callers).
- `gofumpt -l` over every touched file -- clean.
- `./custom-gcl run --new-from-rev=origin/main` (patch-scoped against `origin/main`, this
  repo's real PR lint gate) -- 0 issues (one `godot` finding on a new test's doc comment
  fixed along the way).
- `grep -rn "would be generated\|Files that would be generated" tests/ website/` -- no hits
  against any scaffold dry-run CLI test case or snapshot; `tests/test-cases/scaffold.yaml` has
  no `--dry-run` cases at all, so no snapshot regeneration was needed.
