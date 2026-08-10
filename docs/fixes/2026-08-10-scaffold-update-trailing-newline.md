# Fix: `scaffold generate --update` no longer strips trailing newlines during text-based 3-way merges

**Date:** 2026-08-10

## Summary

`atmos scaffold generate --update` unconditionally stripped the trailing newline from every
non-YAML file it 3-way-merged, whether or not the file's content actually changed. `TextMerger.Merge()`
now appends one newline to each of `ours`/`base`/`theirs` before handing them to `diff3.Merge`, which
cancels out `diff3`'s guaranteed loss of exactly one trailing newline and preserves the original count.

## Context

- Reported in #2887: any file merged through `--update` (Terraform, shell scripts, Markdown, arbitrary
  templates — anything going through the text merger, not the YAML merger) reproducibly lost one trailing
  newline per run, even on a genuine no-op where nothing on the template side changed.
- Root cause: `TextMerger.Merge()` delegates the actual merge to `epiclabs-io/diff3`, which reads
  `base`/`ours`/`theirs` line-by-line via `bufio.Scanner` (`ScanLines`) and rejoins the merged lines with
  `strings.Join(lines, "\n")`. `ScanLines` strips every line's terminator, including the last, and gives no
  way to recover afterward whether the original input ended with a trailing newline. For content ending in
  N trailing newlines, the round-trip through `diff3` always reconstructs exactly N-1 (nothing to lose when
  N = 0). Verified directly: generating a file with 3 trailing newlines and running `--update` with nothing
  changed on the template side reproducibly came back with 2.
- PR: https://github.com/cloudposse/atmos/pull/2891.

## Changes

- `pkg/generator/merge/text_merger.go`: `TextMerger.Merge()` appends one `newlineSeparator` ("\n") to each
  of `ours`, `base`, and `theirs` before passing them to `diff3.Merge`. Bumping every input's trailing count
  to at least 1 makes `diff3`'s guaranteed loss of exactly one newline cancel out, so the original count
  survives for both no-op merges and genuine template changes. This must apply to all three inputs, not
  just `theirs` — appending it only to `theirs` would make an otherwise-identical `ours`/`theirs` pair (a
  common no-op shape) differ by one trailing newline as far as `diff3` is concerned, turning a no-op into a
  spurious detected change/conflict.
- An earlier iteration of this fix added a separate `matchTrailingNewline` post-processing step; it was
  removed in favor of the simpler pre-merge newline append once it was clear the append alone makes the
  counts cancel correctly (see commit `f77a925e5`).
- No change to conflict detection, threshold behavior, or `ConflictStrategy` handling — out of scope, and
  unaffected since the appended newline is identical across all three inputs.
- `pkg/generator/merge/text_merger_test.go`: consolidated the trailing-newline regression coverage into a
  single table-driven test, `TestTextMerger_TrailingNewlinePreservation`, asserting exact byte-for-byte
  output across a no-op merge (0/1/2/3 trailing newlines, plus an internal blank line) and a genuine
  template change (theirs with 0/1/2 trailing newlines). CodeRabbit feedback on the PR added further EOF
  edge cases to this table.

## Validation

- `go test ./pkg/generator/merge/...` — passes, including `TestTextMerger_TrailingNewlinePreservation`.
- Manual repro from #2887 (generate a scaffold file with a trailing newline, run `--update` with no
  template changes, diff the file before/after) no longer shows the newline being dropped.

## Follow-ups

None.
