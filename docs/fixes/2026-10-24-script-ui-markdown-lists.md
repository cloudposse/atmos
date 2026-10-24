# Fix: Preserve Markdown lists and email addresses in script status messages

**Date:** 2026-10-24

## Summary

Starlark `ui.info`, `ui.success`, and `ui.warning` retain Markdown rendering while displaying list markers and email addresses correctly, including in redirected and no-color output.

## Context

The standalone ten-line script reproduced on current main at `cb23b3f701`: ordered markers lost their punctuation and spacing, bullets disappeared, and email addresses gained a duplicate `mailto:` destination. `print()` remained unaffected. The user explicitly requested keeping Markdown rendering.

The custom renderer omitted the entire stylesheet for the ASCII color profile, removing structural list styles along with color. Toast layout also stripped indentation from every rendered line, flattening nested lists. Glamour rendered an email autolink's label and its destination separately.

## Changes

- Add only the existing ASCII list layout rules to the custom renderer's uncolored stylesheet. Keep unrelated no-color heading and blockquote output unchanged; the general ASCII preset would add visible `#` and `|` markers.
- Extract toast layout into `pkg/ui/toast_markdown.go`; remove renderer margins and common document indent while preserving relative list and hanging indentation.
- Render terminal email autolinks once while preserving named Markdown links, URL rendering, and the HTML extension's email behavior. This incorporates the email fix also proposed in PR #3520.
- Cover every line of the standalone repro, exact multi-line ordered and unordered lists, escaped literal markers, inline Markdown, named links, nested and wrapped lists, and no-color/TrueColor output with rendered-string assertions.
- Document Markdown behavior and the Starlark escaping example `ui.info("1\\. literal")` in all three automation function pages and the Starlark agent skill. Markdown normalizes `2)` to `2.` and unordered markers to `•`; escaping punctuation preserves literal markers.

## Validation

The original CLI reproduced the reported output from a temporary directory without project configuration. Regression tests failed before their respective fixes. `go test ./pkg/script/starlark ./pkg/ui/... -count=1` passed, including the complete ten-line script and unchanged stdout assertion. The rebuilt CLI passed the exact script from a temporary directory in both no-color and forced-color modes. `go build ./...` passed, and changed-code custom lint reported zero issues. The website build (`npm run build`) and fix-log validator passed.

During review, exact no-color heading and quote assertions failed against the broader ASCII preset, then passed with the list-only stylesheet. The full UI and Starlark package tests passed again, including exact multi-line lists, literal Markdown escapes, and named-link destinations. Changed-code custom lint reported zero issues. The website build passed again with 82 prebuild tests; generated HTML for all three function pages contains the correctly escaped Starlark examples and literal output.

## Follow-ups

The shared fix reaches normal toast/say status output and `ui.Hint`. Log, alert, and streamed custom-command shell output bypass this renderer. Captured toast/say output uses its existing plain writer.

The separate structured-error renderer (`markdown.Renderer.RenderReadable`, used by `errors/formatter.go`) already retains list markers but still duplicates email destinations. A local reproduction returned `1. Contact a@b.com mailto:a@b.com`. This pre-existing issue is listed in the PR as requested; no separate issue or unrelated renderer change is included.
