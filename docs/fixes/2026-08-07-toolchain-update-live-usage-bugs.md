# Fix: `toolchain update`/`toolchain lock` version-compare, live progress, and styling bugs

**Date:** 2026-08-07

## Summary

Found by the user running the real `build/atmos toolchain update` binary against their own
project, immediately after an earlier same-day fix batch for the same commands: (1) tools pinned
with a literal `v` prefix (e.g. `peteretelej/tree v1.3.0`) were always reported as "updated" even
when nothing changed, because the version comparison never normalized the prefix stripped by
`pkg/github.GetReleaseVersions` — this also silently triggered a real, unnecessary reinstall and
rewrote `.tool-versions`, and miscounted the summary line (`Updated 5 tool(s), 0 up to date...`
when 2 of the 5 were actually unchanged); (2) `update`/`lock` printed zero output while a batch
ran (network-bound, can take tens of seconds), then dumped every result at once when the last
concurrent worker finished, indistinguishable from a hang; (3) the per-tool result lines used a
hand-rolled `✓`/`✗` glyph printed through the plain `ui.Writef`, so they rendered with no color at
all, unlike every other themed status line in the CLI; (4) the summary line always printed every
category even at zero (`Updated 0 tool(s), 1 up to date, 0 skipped, 0 failed`), which reads as
noise. All four are fixed. The initial fix for (2) was a single static "Updating N tool(s)..."
line; the user explicitly asked for parity with `atmos toolchain install`'s existing live
spinner/progress-bar batch UI instead, so that was implemented as a proper follow-up in this same
pass (see Changes). The `field-test` skill was also updated with a new hypothesis category so
this class of bug is checked for on future passes.

## Context

This is a follow-up to the same-day fix batch documented in
`docs/fixes/2026-08-07-toolchain-update-lock-version-pinning-fixes.md`. That pass was driven by
the `field-test` skill, executed largely through the Bash tool — which is exactly why these bugs
slipped through: the version-compare bug only manifests on a real, currently-latest tool whose
GitHub tag happens to use a `v` prefix (not covered by the fixture used during that pass), and the
progress/styling bugs are only obvious watching a real terminal — captured Bash output doesn't
make a "worked instantly vs. hung then dumped" difference visually apparent, and ANSI color codes
are easy to overlook in a raw text transcript. The user caught all of these within seconds of
running the freshly-fixed binary for the first time, and specifically rejected a first-pass fix
for the "looks hung" complaint (a single upfront `ui.Infof` line) as insufficient, asking for the
same live per-tool spinner + progress-bar experience `atmos toolchain install` already has for its
own concurrent batch mode.

Research (via an Explore agent) confirmed `install.go`'s `batchEvent`/`batchRenderer` combo
(worker pool → event channel → single collecting goroutine as the sole terminal writer) is the
only place in the codebase that already solves "N concurrent items, one live-updating multi-line
display" safely — `lock.go`'s own `spinnerControl`/bubbletea-per-worker approach is explicitly
documented and avoided as unsafe for concurrent use (hardcoded off in `lockOneTool`). Rather than
duplicate that pattern or risk regressing `install`'s already-shipped, tested implementation by
generalizing it in place, a new, install-agnostic version of the same architecture was extracted
into `pkg/toolchain/batch_progress.go` for `update`/`lock` to share going forward.

Adopting live, completion-order printing (matching `install`'s own convention) is a deliberate,
explicit trade-off: `update`/`lock` previously guaranteed output was printed in original target
order regardless of concurrency, buffering every result until the whole batch finished to do so.
That guarantee is now gone — lines print as each tool finishes, which is not necessarily target
order — in exchange for the live-progress UX the user asked for. This was a conscious choice, not
an oversight; see the updated tests below for the narrower guarantee that replaces it.

## Changes

- `pkg/toolchain/update.go` — `updateExactPinnedTool`'s `newest == current` comparison now uses
  the existing `normalizeVersion()` helper (already used by `list.go` for the same reason) on both
  sides before comparing, so a `v`-prefixed pin correctly matches an unprefixed but identical
  fetched version.
- `pkg/toolchain/update.go`, `pkg/toolchain/lock.go` — stripped every hand-rolled `✓`/`✗`/`⊘`
  glyph from status messages.
- `pkg/toolchain/batch_progress.go` (new) — `runConcurrentBatchWithLiveProgress[I, T]`, a
  generic, reusable worker-pool-plus-live-renderer, modeled directly on `install.go`'s
  `batchEvent`/`batchRenderer`: a spinner per in-flight item, an overall N/M progress bar, and
  each completed item's line printed live (via `ui.Success`/`ui.Info`/`ui.Error`, selected by a
  caller-supplied `batchLineStyle`) as soon as it finishes, with the same non-TTY/debug-log
  fallback `install.go` uses. Workers never touch the terminal directly.
- `pkg/toolchain/update.go`, `pkg/toolchain/lock.go` — `RunUpdate`/`RunLock` now drive their
  concurrent batches through `runConcurrentBatchWithLiveProgress` instead of the old
  buffer-everything-then-print-in-target-order workers (`runUpdatesConcurrently`/
  `runLockConcurrently`, both removed). `reportUpdateOutcomes`/`reportLockOutcomes` were renamed
  to `tallyUpdateOutcomes`/`tallyLockOutcomes` and now only count outcomes and print the summary
  — the per-tool lines are already printed live by the batch renderer.
- `pkg/toolchain/update.go`, `pkg/toolchain/lock.go` — `printUpdateSummary`/`tallyLockOutcomes`'s
  summary line now omits zero-count categories entirely (`Updated 3 tool(s), 1 up to date` instead
  of `Updated 3 tool(s), 1 up to date, 0 skipped, 0 failed`).
- `website/blog/2026-08-06-toolchain-update-command.mdx` — updated the two literal example output
  blocks to match the new suppressed-zero summary format.
- `.claude/skills/field-test/SKILL.md` — added a Phase 2 hypothesis category for live-progress and
  `ui.*` styling verification on batch/concurrent commands (with a note that Bash-tool-captured
  runs can mask this class of bug, and a concrete `--force-color` + raw-escape-code inspection
  technique), and a Phase 4 execution reminder to manually recount summary/tally lines against
  their detail lines rather than trusting them at face value.

## Validation

- New regression test `TestUpdateOneTool_ExactPin_VPrefixMismatchIsUpToDate`
  (`pkg/toolchain/update_test.go`) written first and confirmed failing against the pre-fix code,
  then confirmed passing after the fix.
- New tests `TestRunConcurrentBatchWithLiveProgress_ResultsPreserveItemOrder` and
  `_RunsEveryItem` (`pkg/toolchain/batch_progress_test.go`) cover the guarantee that replaces the
  removed target-order-printing guarantee: results stay indexed to the original items regardless
  of completion order (verified by making the first item take the longest and the last item the
  shortest, forcing reversed completion order), and every item is processed exactly once even when
  `maxConcurrency < len(items)`.
- `TestRunLock_ReportsInTargetOrder`/`TestRunUpdate_ConcurrencyPreservesOrder` were renamed to
  `TestRunLock_ReportsAllTargets`/`TestRunUpdate_ConcurrentAllSkippedReportsEveryTarget` and
  their order assertions removed, since target-order printing is no longer the contract (see
  Context) — they now only assert every target is reported.
- `go build ./...` — clean.
- `go vet ./...` — clean (full repo).
- `go test ./pkg/toolchain/... ./cmd/toolchain/...` — all packages pass.
- Live end-to-end verification against a real, isolated fixture with real network access
  (`.context/field-test-toolchain/`, `ATMOS_XDG_CACHE_HOME` redirected away from the shared
  cache), run through `script -q /dev/null ... --force-tty --force-color` to force a real
  pseudo-TTY (required for the live-renderer code path, which checks `isTTY()`): confirmed the
  animated braille spinner and gradient progress bar render and update live for both
  `atmos toolchain update` and `atmos toolchain lock`, completed items print as colored toast
  lines above the redrawn region, the `peteretelej/tree v1.3.0` case is correctly reported "up to
  date" with an accurate suppressed-zero summary (`Updated 3 tool(s), 1 up to date, 3 skipped`),
  and `atmos toolchain lock jq yq` shows the same live UI.
- `gofmt -l` — clean on every touched Go file.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (fixed one cyclomatic-complexity
  finding in `RunUpdate` by extracting `renderUpdateOutcome`, and two `godot` comment-wording
  findings in `batch_progress.go`, during the pass).
- Not yet committed, pushed, or opened as a PR.

## Follow-ups

None. (The live per-tool progress UI for concurrent batches — noted as deliberately deferred in
an earlier version of this fix record — has been implemented; see Changes.)
