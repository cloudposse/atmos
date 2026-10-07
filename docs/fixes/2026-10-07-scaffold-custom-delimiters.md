# Fix: `atmos scaffold` honors `spec.delimiters` in path validation, `ProcessTemplate`, and `--update` merge

**Date:** 2026-10-07

## Summary

A scaffold template can set `spec.delimiters` (for example `["[[", "]]"]`) so file payloads can
contain GitHub Actions `${{ }}` expressions literally. Three call sites in
`pkg/generator/engine` still hardcoded the default `{{`/`}}` pair, so custom delimiters were
ignored there:

1. `validateRenderedPath` rejected any rendered path containing `{{` or `}}`, even when the
    scaffold's active delimiters were `[[ ]]`. A rendered path that legitimately contained a
    literal `{{` was wrongly rejected, and an unrendered `[[ .x ]]` path marker was not
    detected.
2. `ProcessTemplate` (public API) always rendered with the default delimiters, ignoring the
    scaffold config it was handed.
3. `mergeFile` carried a second `IsTemplate` re-render branch that used nil config, nil values,
    and default delimiters.

## Context

This is "bug 4", deferred in `docs/fixes/2026-08-24-scaffold-update-git-dryrun-fixes.md`
(PR #2989) and originally reported by a client against `v1.226.0`.

Findings on this branch:

- `validateRenderedPath` was the only live defect: `ProcessFile` already had the scaffold's
  delimiters in scope (from `extractDelimiters`) when it called it.
- `ProcessTemplate` has no production callers, but it is public API and silently ignored the
  config's delimiters.
- The `mergeFile` branch was dead as well as wrong: its only caller, `handleExistingFile`,
  already renders the content with the correct delimiters, config, and values, and clears
  `IsTemplate` before calling.
- Audit of the remaining default-only helpers: `ContainsUnprocessedTemplates` and
  `ValidateNoUnprocessedTemplates` have no production callers. The real path uses the
  `WithDelimiters` variants (`templating.go`, `ValidateNoUnprocessedTemplatesWithDelimiters`),
  so they are left unchanged.
- Making `ProcessTemplate` call `extractDelimiters` exposed a latent panic: a typed nil
  `*config.ScaffoldConfig` inside the `interface{}` argument was dereferenced in
  `tryExtractFromPointerConfig`. It is now guarded.

## Changes

- `pkg/generator/engine/templating.go`
  - `validateRenderedPath(renderedPath, originalPath, delimiters)` takes the active delimiter
    pair and checks it for unrendered markers, falling back to the defaults when
    `len(delimiters) != 2`. `ProcessFile` passes its already-extracted `delimiters`. The doc
    comment is updated.
  - `ProcessTemplate` now renders with `extractDelimiters(scaffoldConfig)`; its doc comment
    states that it honors the config's delimiters.
  - `tryExtractFromPointerConfig` tolerates a typed nil `*config.ScaffoldConfig`.
- `pkg/generator/engine/merge_update.go`
  - `mergeFile` no longer re-renders. The dead `IsTemplate` branch is removed and
    `file.Content` is used directly as "theirs". The now-unused `targetPath` parameter is
    dropped (`mergeFile(existingPath, file)`), and the doc comment states that the caller must
    pass already-rendered content.
- Tests (`pkg/generator/engine/templating_coverage_test.go`, `update_test.go`)
  - New: `TestValidateRenderedPath_HonorsActiveDelimiters`,
    `TestProcessTemplate_HonorsScaffoldConfigDelimiters` (pointer, value, map, typed nil, and
    nil configs), `TestProcessFile_CustomDelimitersPathWithLiteralBraces`,
    `TestProcessFile_UpdateCustomDelimitersKeepsLiteralExpressions` (full `--update` path), and
    `TestProcessorMergeFile_UsesContentAsRendered`.
  - Existing `validateRenderedPath` and `mergeFile` call sites updated for the new signatures.
  - `TestProcessorMergeFileTemplateProcessingError` removed, since it asserted the deleted
    render branch's error. `TestProcessorMergeFile_TemplateProcessingSuccess` is replaced by
    `TestProcessorMergeFile_PreRenderedContentSuccess`, which passes pre-rendered content.

## Validation

- Before the fix (tests adapted to the old signatures so the package compiled),
  `go test ./pkg/generator/engine/ -run '<new tests>'` failed:
  `TestValidateRenderedPath_HonorsActiveDelimiters` (unrendered `[[ ]]` markers not rejected;
  literal `{{` and `${{` rejected with `unprocessed template variable found`),
  `TestProcessTemplate_HonorsScaffoldConfigDelimiters` (`function "github" not defined`),
  `TestProcessFile_CustomDelimitersPathWithLiteralBraces` (`unprocessed template variable
  found`), and `TestProcessorMergeFile_UsesContentAsRendered` (`template execution failed`).
  `TestProcessFile_UpdateCustomDelimitersKeepsLiteralExpressions` passed both before and
  after, because the dead branch was never reached; it is a regression guard.
- `go build ./... && go test ./pkg/generator/... ./cmd/scaffold/... ./cmd/init/...`: all
  packages pass after the fix.
- `gofumpt -l` on the changed files: clean (the pre-existing unformatted
  `templating_test.go` was not touched).
- `atmos lint --changed`: 0 issues.
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh docs/fixes/2026-10-07-scaffold-custom-delimiters.md`:
  passes.

## Follow-ups

None.
