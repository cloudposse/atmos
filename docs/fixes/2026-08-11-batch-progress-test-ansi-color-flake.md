# Fix: `TestLiveBatchRenderer_*`/`TestLiveBatchDisplay_*` failed on CI due to ANSI-split assertions

**Date:** 2026-08-11

## Summary

The `Acceptance Tests (linux)` and `Acceptance Tests (macos)` GitHub Actions jobs both failed in
`pkg/toolchain` with the same two test failures:

```
--- FAIL: TestLiveBatchRenderer_StartTickCompleteRenderAndClear
    Error: "...\x1b[32m✓\x1b[0m ...tool-a\x1b[0m\x1b[97m done\x1b[0m\n..." does not contain "tool-a done"
--- FAIL: TestLiveBatchDisplay_WithRenderer_DelegatesEveryMethod
    Error: "...\x1b[32m✓\x1b[0m \x1b[97mtool\x1b[0m\x1b[97m done\x1b[0m\n" does not contain "tool done"
```

Both tests (added in the prior patch-coverage pass, `2bb3ff031b`) asserted a completed batch
item's rendered line with `assert.Contains(t, output, "tool-a done")`. `liveBatchRenderer.complete`
renders that line via `ui.Success`, which styles the label and the trailing message as separate
color runs -- in the CI environment's active color profile, that inserted a reset+re-open escape
sequence between `tool-a` and ` done` (`...tool-a\x1b[0m\x1b[97m done...`), breaking the literal,
contiguous substring match. Locally these tests passed because the color profile resolved
differently there (color effectively disabled), so the bug wasn't caught before this branch's
tests were pushed.

## Context

This is a straightforward environment-dependent flake class, not a logic bug in the renderer
itself: the two tests are asserting real behavior correctly, they just weren't robust to *how*
that behavior is styled. The repo already has an established pattern for this exact problem
(`ansiEscapeRE`/`stripANSI` in `cmd/secret/handler_helpers_test.go`) -- this fix ports that same
pattern into `pkg/toolchain`, rather than inventing a new one or trying to force a specific color
profile in the test (which would only paper over the assertion, not make it robust to whichever
profile a given CI runner or local machine actually resolves).

## Changes

- `pkg/toolchain/batch_progress_test.go` — added `ansiEscapeRE`/`stripANSI` (same regex and
  behavior as the existing `cmd/secret` helper) and applied `stripANSI(...)` before the two
  `assert.Contains` checks that assert on a multi-word, `ui.Success`-styled phrase
  (`"tool-a done"`, `"tool done"`). The other `assert.Contains` checks in this file assert on
  single words (`"tool-a"`, `"tool-b"`), which stay contiguous regardless of color styling, so
  they were left as-is.

## Validation

- `go test ./pkg/toolchain/ -run 'TestLiveBatchRenderer_StartTickCompleteRenderAndClear|TestLiveBatchDisplay_WithRenderer_DelegatesEveryMethod' -v` — both pass.
- Re-ran the same two tests with `CI=true FORCE_COLOR=1 ATMOS_FORCE_COLOR=true` to simulate a
  color-forced environment closer to what CI apparently resolves — both still pass, confirming
  the fix is robust to the actual root cause (not just coincidentally green locally again).
- `go test ./pkg/toolchain/...` — full package tree passes.
- `go build ./...` — clean.
- `./custom-gcl run --new-from-rev=<merge-base>` — 0 issues (fixed one `godot` false-positive
  along the way, caused by `e.g.`-style abbreviation periods confusing the sentence-splitter --
  same class of issue as `docs/fixes/2026-08-08-toolchain-live-renderer-windows-ci-deadlock.md`'s
  predecessor commit hit).

## Follow-ups

None.
