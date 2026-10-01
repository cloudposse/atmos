# Fix: CI test summary fallback recovers a file/line when available, without corrupting the table

**Date:** 2026-08-19

## Summary

When `terraform test`'s per-run `run "name"... pass/fail` lines aren't captured and
`ParseTestOutput` falls back to the trailing summary line, the synthesized aggregate row now
recovers the failing assertion's file/line from a terraform `Error:` diagnostic block when
exactly one survived in the captured output, instead of always rendering a bare "per-run detail
unavailable" placeholder with no location. The block's raw message text is deliberately not
copied into the row — it's already rendered safely elsewhere, and copying it in was found (via a
field-test pass) to corrupt the CI summary's markdown table.

## Context

`docs/fixes/2026-08-14-ci-summary-test-table-fallback-dropped.md` fixed the results table
disappearing entirely on this fallback path by synthesizing a single aggregate
`plugin.TerraformTestRun` row. That row carried no `File`/`Line` detail — even in cases where
terraform's `Error:` diagnostic block (file, line, and assertion message) was still present in the
captured text, only the per-run `run "..."... pass/fail` status lines were missing. The gap was
reported as: CI test summaries provide only aggregate pass counts and a reproduction command, with
no individual test-run/assertion detail.

A reproduction test (`TestTestTemplate_SummaryFallback_LosesRunDetail`) confirmed the rendered CI
summary genuinely dropped a failing run's file/line when the captured text contained only the
trailing `Failure! N passed, M failed.` line. A first attempt at fixing this recovered the file,
line, **and** full message by joining terraform's `Error:` block(s) verbatim into the synthesized
row's `Error` field. A field-test pass on that attempt found two problems before it shipped:

1. **Duplication, not recovery.** `ParseTestOutput`'s pre-existing, unrelated `result.Errors =
    ExtractErrorBlocks(output)` step (unchanged by either fix) already populates the summary's
    separate fenced `` ```hcl ``` `` code block with the same message whenever an `Error:` block
    survives — this was true even before either fix, on `main`. So joining the same raw text into
    the row's `Error` field mostly duplicated information already visible below the table, rather
    than closing a real gap.
2. **Markdown table corruption.** The raw block text is multi-line and routinely contains a
    literal `|` (e.g. `condition = a || b` in an HCL assertion). Spliced unescaped into a table
    cell (`templates/test.md` registers no escaping — `pkg/ci/templates/loader.go`), the embedded
    newline terminates the GFM table row mid-cell and dumps the remaining text as loose paragraph
    text below the table; an embedded `|` would additionally split into extra spurious columns.
    Confirmed by rendering the actual template against a realistic fixture (an HCL condition with
    `||`) — the row broke exactly as predicted.
3. A secondary finding: when more than one `Error:` block survived (multiple failing assertions),
    the row's `File`/`Line` was attributed to the *first* block only, while the (now-removed)
    `Error` field's message spanned all of them — a misleading single-location attribution for a
    multi-failure aggregate row.

## Changes

- `pkg/ci/plugins/terraform/parser.go`:
  - `errorLocationRe` (added by the first attempt) is unchanged: matches the `on <file> line <N>:`
    locator inside a terraform `Error:` diagnostic block.
  - `synthesizeFallbackRun` now calls `ExtractErrorBlocks` and only attributes `File`/`Line` when
    **exactly one** block is present (`len(blocks) == 1`) — with more than one, neither is set,
    since attributing a single location to a multi-failure row would misrepresent it.
  - The row's `Error` field is no longer populated at all in the fallback path. The full message
    remains available via the separate `result.Errors` fenced block, which this change does not
    touch.
- `pkg/ci/plugins/terraform/test_parser_test.go`:
  - `TestParseTestOutput_SummaryFallback_RecoversErrorDetail` updated to assert `run.Error` is
    empty and that the message is available via `result.Errors` instead.
  - New `TestParseTestOutput_SummaryFallback_MultipleErrorBlocks_NoLocationAttributed`: two
    distinct failing assertions in two files → `File`/`Line`/`Error` all empty on the row, both
    messages still present in `result.Errors`.
- `pkg/ci/plugins/terraform/test_template_test.go`:
  - Added a `tableRowLine` test helper that locates the results-table row and fails if there isn't
    exactly one line matching it (catching content that leaked outside the table).
  - `TestTestTemplate_SummaryFallback_RecoversErrorDetail` rewritten to assert the row has exactly
    5 columns (`strings.Count(row, "|") == 6`), that the HCL condition's `||` never leaks into the
    row, and that the message text appears exactly once in the whole rendered output (in the
    fenced block, not duplicated into the row) — assertions strong enough that the corrupted
    version of this fix would have failed them (the original version only used loose
    `assert.Contains` checks, which passed even on the corrupted output).
  - New `TestTestTemplate_SummaryFallback_MultipleErrorBlocks_NoLocationAttributed`: template-level
    counterpart of the parser test above.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/ci/plugins/terraform/... -run 'TestParseTestOutput|TestTestTemplate' -v` — all
  pass, including the new/updated tests above and the retained
  `TestTestTemplate_SummaryFallback_LosesRunDetail` (the residual case: no `Error:` block survives
  at all — still genuinely unrecoverable, unaffected by this change).
- `go test ./pkg/ci/...` — full package, all pass.
- `atmos lint --changed` — 0 issues.
- Manually rendered the template against a realistic fixture (HCL condition containing `||`) and
  visually confirmed the results table stays a single valid row, with the full message appearing
  once in the fenced code block below it.

## Follow-ups

None. The residual case (no `Error:` diagnostic block at all survives in the captured output) has
no recoverable detail in the text and remains covered by
`TestTestTemplate_SummaryFallback_LosesRunDetail` as a known, documented limit of this legacy
text-parsing fallback path.
