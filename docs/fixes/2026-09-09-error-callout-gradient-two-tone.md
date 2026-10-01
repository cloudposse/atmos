# Fix: Explanation callout gradient no longer collapses to a two-tone jump on short callouts

**Date:** 2026-09-09

## Summary

`errors/formatter.go`'s `renderExplanationCallout` applies a background color gradient across the wrapped
lines of an error's "explanation"/"hint" callout. For short callouts (2-3 lines, the common case for real
error messages), `gradientRatio(i, total)` sampled only the gradient's two pure endpoint colors with zero
interpolation in between, producing a jarring two-tone box instead of a smooth gradient. `gradientRatio` now
floors its step count at `explanationGradientMinSteps` (3), so short callouts sample a narrow, blended slice
near the gradient's start instead of jumping straight to both extremes. Callouts with 4+ lines already
exceeded this floor and render byte-for-byte identically to before.

## Context

Found via a real-AWS field-test session: a live error was raised on a real AWS account and its rendered
explanation callout was visually inspected in a real terminal, showing an abrupt color seam between the
callout's two lines. The raw ANSI escape codes were captured from the actual terminal output and confirmed
the two lines used the gradient's pure endpoint backgrounds directly, with no blending: line 1
`rgb(51,32,79)` (`#33204f`, the configured `explanationGradientStart`) and line 2 `rgb(18,60,92)` (`#123c5c`,
the configured `explanationGradientEnd`).

Reading `gradientRatio` confirmed the root cause: `gradientRatio(index, total) = index / (total-1)`. For
`total == 2` this returns exactly `0.0` and `1.0` -- the two pure endpoints -- and for `total == 3` it returns
`0.0, 0.5, 1.0`, still touching both pure endpoints. Only callouts with 4 or more lines avoided sampling both
extremes. Most real error explanations and hints wrap to 2-3 lines, making the broken case the common case,
not an edge case.

## Changes

- `errors/formatter.go`: added the `ExplanationGradientMinSteps` constant (value `3`) and updated
  `gradientRatio` to use `max(total-1, ExplanationGradientMinSteps)` as its step denominator instead of
  `total-1` directly. This only changes behavior for `total < 4` (2 and 3 line callouts); for `total >= 4` the
  denominator is unchanged from before, so long callouts render identically to before the fix.
- `errors/formatter_test.go`:
  - Updated `TestRenderExplanationCallout_ColorAddsGradientBackground` to stop asserting the old two-tone
    behavior (it previously asserted a 2-line callout contained both the pure start *and* pure end colors,
    which was the bug baked into the test suite).
  - Added `TestRenderExplanationCallout_ShortCalloutIsSubtleGradient`, which renders a 2-line callout and
    asserts its second line is not the gradient's pure end color, and that the two lines' background colors
    are within half the full gradient's color distance of each other (i.e. close shades, not maximally
    distant extremes).
  - Added `TestRenderExplanationCallout_LongCalloutKeepsFullGradient`, which renders a 6-line callout and
    asserts the first/last lines still hit the pure start/end colors exactly (proving the long-callout case is
    unregressed) while every consecutive pair of lines still differs (proving visible progression, not a flat
    fill).
  - `restoreCalloutColorsAfterReset`/`calloutColorPrefix` (the Glamour full-reset handling) were not touched;
    `TestRenderExplanationCallout_RestoresColorsAfterNestedReset` remains green unchanged.

## Validation

Before/after RGB values captured directly from `renderExplanationCallout` output (TrueColor profile forced):

| Callout | Line | Before (RGB) | After (RGB) |
|---|---|---|---|
| 2-line | 1 | `(51,32,79)` | `(51,32,79)` (unchanged) |
| 2-line | 2 | `(18,60,92)` (pure end -- the bug) | `(40,40,83)` (close shade, not the pure end) |
| 6-line | 1 | `(51,32,79)` | `(51,32,79)` (unchanged) |
| 6-line | 2..5 | `(44,38,81) .. (25,54,89)` | `(44,38,81) .. (25,54,89)` (unchanged) |
| 6-line | 6 | `(18,60,92)` | `(18,60,92)` (unchanged) |

Commands run:

```bash
go build ./...
go test ./errors/...
GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml --allow-serial-runners --new-from-rev=origin/main
```

- `go build ./...` -- passed.
- `go test ./errors/...` -- passed, including the three new/updated gradient tests and the pre-existing
  Glamour-reset test.
- `custom-gcl run --new-from-rev=origin/main` -- passed for `errors/formatter.go` and
  `errors/formatter_test.go` (one `godot` finding on the new constant's doc comment was fixed by capitalizing
  its first word, matching the file's existing convention for unexported-constant doc comments, e.g.
  `explanationForeground`). Unrelated findings reported by the same lint run in `internal/exec/` and
  `pkg/utils/` belong to other in-progress work on this branch and were left untouched.

## Follow-ups

None.
