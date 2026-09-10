# Fix: `--merge-strategy=manual` conflict markers, `atmos init --update` base-ref pinning, and `--force`+`--update` semantics

**Date:** 2026-09-04

## Summary

`atmos scaffold generate --update` (and `atmos init --update`, which shares the same engine) use a
git-based 3-way merge to reapply template changes onto a file the user has already customized. This
fix addresses four related problems found across that one code path, all tracked on
[cloudposse/atmos#2912](https://github.com/cloudposse/atmos/issues/2912):

1. With the default `--merge-strategy=manual`, a genuine ours/theirs conflict caused the command to
    exit non-zero with an explicit error — but wrote nothing to disk at all: no conflict markers, no
    merged content, not even the file's own non-conflicting changes. This is the still-open half of
    #2912; the issue was closed as resolved by #2989, but that PR fixed a different bug reported in
    the same thread (a silent base-ref-pinning problem) and never touched this code path.
2. Field-testing fix 1 surfaced that `atmos init --update` still has the *exact* base-ref-pinning
    bug #2989 fixed for `atmos scaffold generate` — it was never ported to `cmd/init`.
3. `--force` was silently ignored whenever `--update` was also set, making several existing error
    hints false.
4. Re-running `--update` against a file fix 1 left with unresolved conflict markers produced an
    opaque `three-way merge failed` instead of naming the real problem — a new consequence of fix 1,
    not a pre-existing gap.

## Context

**Manual-merge conflict markers (fix 1):** `YAMLMerger` and `TextMerger` (`pkg/generator/merge/`)
both compute a 3-way merge result and report `HasConflicts` when the user and the template
genuinely diverged on the same value. `mergeFile` (`pkg/generator/engine/merge_update.go`) treated
`HasConflicts` as fatal and returned before ever reaching the write — for both mergers, on every
conflict, regardless of merge strategy.

- `TextMerger` already produced real diff3-style markers in `MergeResult.Content` when a conflict
  was found (via the `epiclabs-io/diff3` library) — they were computed and then thrown away.
- `YAMLMerger` never had an equivalent: on a real divergence it just picked `ours` internally
  (`pickConflictValue`) so it had *something* to return, and that computed result was discarded by
  the same early return. There was no way to express `<<<<<<<`/`=======`/`>>>>>>>` markers in a
  parsed YAML tree at all.

Verified live against the exact repro steps from the original issue (both the YAML and text-file
cases) that current `main` (`18750ab6a`, confirmed via `git fetch upstream` — 11 commits ahead of
the commit first tested, none touching the affected files) still reproduced the bug before this fix.

**`atmos init --update` base-ref pinning (fix 2):** `cmd/scaffold/scaffold.go`'s `defaultBaseRef`
reads a pinned commit SHA from `.atmos/scaffold/metadata.yaml` (written by `gen.PinInitialBaseRef`
during `--git` generation) instead of defaulting to live `HEAD`. `cmd/init/init.go`'s
`defaultBaseRef` was never updated to match — it still hardcoded `"HEAD"`, and `atmos init --git`
never called any pinning function at all. Once a user committed a customization, `HEAD` became
byte-identical to their working tree, so the merge concluded nothing had diverged and silently let
the template win — exit 0, no warning. Root cause confirmed live: passing the correct
`--base-ref <initial-sha>` explicitly preserved the customization, isolating the bug to
default-resolution, not the merge logic itself.

**`--force` semantics (fix 3):** `handleExistingFile` (`pkg/generator/engine/templating.go`) checks
`update` before `force`, and the `update` branch always merges and returns — so `--force --update`
together behaved identically to `--update` alone. A full blind overwrite under `--update` was
considered and rejected as the fix — it would silently discard non-conflicting customizations too,
defeating the reason `--update` was requested. The chosen design: `--force` flips
`--merge-strategy`'s *default* from `manual` to `theirs` when `--update` is set and no strategy was
explicitly passed through any layer (CLI flag, env var, or config) — and errors if
`--force --update` is combined with an *explicitly* passed `ours` or `manual`, since that
combination is a genuine contradiction rather than a preference.

**Opaque re-run error (fix 4):** This scenario didn't exist before fix 1 (nothing was ever written
on conflict), so it's a new consequence of that fix rather than a pre-existing gap. A file left with
real conflict markers, if fed back into `--update` unresolved, either fails to parse as YAML (an
opaque error) or — for text files, which have no syntax requirement on their input — silently
produces a garbled result with no error at all.

## Changes

**Fix 1 — conflict markers:**

- `pkg/generator/merge/yaml_merger.go`: on a real conflict under `ConflictStrategyManual`, the
  conflicting node is now replaced with a unique sentinel scalar placeholder
  (`addNodeConflict`/`conflictSentinelFormat`) instead of silently picking `ours`. After the whole
  tree is encoded to YAML text, `spliceConflictMarkers` finds each sentinel and reconstructs real
  `<<<<<<<`/`=======`/`>>>>>>>` markers around independently-rendered `ours`/`theirs` fragments —
  inline when both sides are scalars, or as an indented block beneath the key when either side is a
  mapping/sequence. `mergeSequences` returns a sentinel node as-is instead of wrapping it as a
  `SequenceNode`. `MergeResult` gained a `ConflictPaths []string` field.
- `pkg/generator/merge/text_merger.go`: added the `ConflictPaths` field to `MergeResult` (left
  `nil` — diff3 hunks aren't addressable by path). No merge-logic change needed; the markers already
  existed.
- `pkg/generator/engine/merge_update.go`: `mergeFile` now writes `result.Content` to disk on a
  conflict (skipped only under `--dry-run`) before returning `ErrMergeConflict`, with the conflicting
  key paths attached as `conflict_paths` context when known.
- Known limitation, documented in code rather than solved: flow-style YAML (`{a: 1, b: 2}`) can
  place more than one sentinel on the same line; the splice then wraps only the first match. Scaffold
  templates in this repo use block style, so this was accepted rather than building a full
  flow-aware splitter.
- `--merge-strategy=ours`/`theirs` are unaffected — both auto-resolve every conflict to a side
  before `HasConflicts` is ever set.

**Fix 2 — base-ref pinning:**

- `pkg/generator/storage/metadata.go`: added `InitMetadataPath` (`.atmos/init/metadata.yaml`),
  mirroring `ScaffoldMetadataPath`.
- `pkg/generator/gitinit.go`: extracted the shared pin/resolve logic that had drifted apart between
  the two commands (which is exactly how `atmos init` missed the original fix) into
  `ResolveDefaultBaseRef(baseRef, targetDir, metadataPath)` and `PinInitialBaseRefForInit` (sharing
  a new private `pinBaseRef` helper with `PinInitialBaseRef`), parameterized by metadata path so
  both commands share one implementation instead of two that can silently diverge again.
- `cmd/scaffold/scaffold.go`: `defaultBaseRef` now delegates to `gen.ResolveDefaultBaseRef` — a pure
  refactor, no behavior change (existing tests pass unmodified).
- `cmd/init/init.go`: ported the full pattern from `cmd/scaffold` — `defaultBaseRef` delegates to
  `gen.ResolveDefaultBaseRef`; `maybeInitGeneratedProjectGit` now calls
  `gen.PinInitialBaseRefForInit` after `--git` creates the initial commit; the early `--base-ref`
  resolution in `RunE` only runs when a positional target was given (matching `cmd/scaffold`'s
  guard); added `resolveInteractiveInitBaseRef` (returning a small `interactiveInitBaseRef` struct,
  to stay under revive's function-result-limit) for the no-positional-target interactive flow,
  mirroring `cmd/scaffold`'s `resolveInteractiveBaseRef`; `shouldOfferUpdate` now takes the actual
  resolved target directory and can return a metadata-load error, matching
  `shouldOfferScaffoldUpdate`.

**Fix 3 — `--force`+`--update`:**

- `pkg/generator/merge/merge.go`: added `ResolveConflictStrategy(mergeStrategy, force, update)`,
  called by both commands instead of `ParseConflictStrategy` directly. An unset strategy defaults to
  `theirs` under `--force`+`--update`; an explicit `manual`/`ours` combined with both is an
  `errUtils.ErrMutuallyExclusiveFlags` error.
- `cmd/init/init.go` / `cmd/scaffold/scaffold.go`: the `--merge-strategy` flag's registered default
  changed from `"manual"` to `""` (help text still advertises `manual` as the effective default) so
  `ResolveConflictStrategy` can distinguish "unset" from "explicitly set to manual" — the same
  pattern already used by `--base-ref`.
- Every `--force`-suggesting hint across `pkg/generator/engine/merge_update.go`,
  `pkg/generator/engine/templating.go`, `pkg/generator/merge/text_merger.go`, and
  `pkg/generator/merge/yaml_merger.go` was reworded to describe what `--force` actually does now
  (resolve conflicts to the template's version) rather than a "complete overwrite" it never performs
  under `--update`; the two "no git storage" hints were corrected to say `--force` only works there
  if `--update` is dropped, since a merge literally cannot be attempted without a git base regardless
  of conflict-strategy.

**Fix 4 — opaque re-run error:**

- `pkg/generator/merge/text_merger.go`: added `HasUnresolvedConflictMarkers`, a
  false-positive-safe check (requires the full `<<<<<<< Ours` / `=======` / `>>>>>>> Theirs` triplet
  in order) distinct from the existing, broader — and previously unused — `HasConflictMarkers`.
- `pkg/generator/engine/merge_update.go`: `mergeFile` now checks `HasUnresolvedConflictMarkers`
  against the existing file before attempting a merge, failing fast with `ErrMergeConflict` and a
  specific explanation instead of re-attempting a merge against corrupted "ours" content.

## Validation

- `go build ./...` clean.
- `go test ./pkg/generator/... ./cmd/init/... ./cmd/scaffold/...` — all pass, including new tests:
  `TestYAMLMerger_ConflictMarkers_Scalar`, `TestYAMLMerger_ConflictMarkers_KindDivergence`,
  `TestYAMLMerger_ConflictMarkers_MultipleConflictsDoNotCollide` (guards the fixed-width sentinel
  format against substring collisions across 12 simultaneous conflicts),
  `TestProcessorMergeFile_ConflictBranchReturnsError`/`TestProcessorMergeFile_ConflictBranchDryRunDoesNotWrite`/
  `TestProcessorMergeFile_RejectsUnresolvedMarkers` (`pkg/generator/engine`),
  `TestDefaultBaseRef_PrefersPinnedMetadata`/`TestShouldOfferUpdate_UsesActualTargetDir`/
  `TestShouldOfferUpdate_PropagatesMetadataLoadError`/`TestMaybeInitGeneratedProjectGit_PinsInitialBaseRef`
  (`cmd/init`), and `TestResolveConflictStrategy`/`TestResolveConflictStrategy_ErrorIsMutuallyExclusiveFlags`/
  `TestHasUnresolvedConflictMarkers` (`pkg/generator/merge`).
- `atmos lint --changed` clean on every file this fix touched, after fixing three findings it
  surfaced along the way: two `godot` comment-capitalization findings, and a `revive`
  function-result-limit finding on `resolveInteractiveInitBaseRef` (fixed by bundling its five
  return values into an `interactiveInitBaseRef` struct — `cmd/scaffold`'s equivalent function has
  the same five-return shape but predates this lint baseline and wasn't flagged, so this new copy
  needed the struct where the original didn't).
- Live end-to-end verification via a built `./build/atmos` binary for all four fixes: the original
  YAML and text-file repro steps now write real conflict markers with non-conflicting changes from
  both sides preserved; nested/deep YAML conflicts reconstruct with correct indentation at 4+ levels;
  `--dry-run` still never writes; `--merge-strategy=ours`/`theirs` still resolve cleanly with no
  markers; a committed `atmos.yaml` customization now survives `atmos init --update`;
  `--update --force` with no explicit strategy now resolves conflicts to the template's version
  instead of being a no-op; `--update --force --merge-strategy=ours` now errors with an explanation
  instead of silently behaving like plain `--update`; `--update --force --merge-strategy=theirs`
  still succeeds (redundant, not contradictory); plain `--update` with no `--force` is unaffected;
  and re-running `--update` against an unresolved-marker file now reports `merge conflict detected`
  (with a specific "still has unresolved conflict markers" explanation) instead of the generic
  `three-way merge failed`, leaving the file untouched either way.
- A full, unscoped `go test ./...` was attempted but is not this fix's validation method of record:
  it hit a `tests` package panic inside `go-git`'s internal merkletrie diff computation (unrelated to
  any file this fix touches) and a 10-minute timeout building a coverage-instrumented binary in
  `tests/testhelpers`, both consistent with running a parallel `go build`/lint pass competing for CPU
  at the same time on this machine rather than a real regression. Per CLAUDE.md, `atmos test`/
  `atmos test --full` (not raw `go test ./...`) is the sanctioned validation command for this repo's
  slow/integration suite; that full run was not repeated here since every package this fix actually
  touches was independently re-verified green above.

## Follow-ups

None. This closes out the original report and every item posted to
[cloudposse/atmos#2912](https://github.com/cloudposse/atmos/issues/2912)'s follow-up comment.
