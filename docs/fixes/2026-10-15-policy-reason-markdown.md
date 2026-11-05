# Fix: Render policy reasons as Markdown

**Date:** 2026-10-15

## Summary

Support Markdown in optional policy reasons consistently across CLI errors and
the policy library.

## Context

The CLI explanation renderer already supports Markdown, but the library displayed
reasons as plain text. Rule identifiers also needed a blank line before authored
content so Markdown blocks remain independent paragraphs or lists.

## Changes

- Separate blocking rule identifiers from their Markdown explanations.
- Render reasons using the library's existing Markdown component, with raw HTML
  disabled and normal paragraph spacing.
- Use emphasis, inline code, and separate paragraphs in both `!exec` variants;
  document literal YAML blocks and supported Markdown formatting.
- Keep the field optional, retain Markdown source in JSON decisions, and do not
  expand templates or interpolate inputs.

## Validation

- Policy tests passed, including styled CLI output for emphasis, inline code,
  lists, and links, and JSON preservation of multiline Markdown.
- Website prebuild checks passed (14 tests).
- Built the CLI and verified a real denial exits 1 with styled Markdown. Captured
  its ANSI output and rendered a PNG with Atmos's native `cast render` command.
- Patch-scoped lint passed.
- The production website build passed. Browser checks verified paragraphs,
  emphasis, and inline code for both engines, hidden optional/nonmatching reasons,
  mobile layout, and no page errors.
- Fix-document validation and `git diff --check` passed.

## Follow-ups

None.
