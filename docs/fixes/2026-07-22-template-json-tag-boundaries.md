# Fix: Decode JSON after whitespace-trimmed YAML function tags

**Date:** 2026-07-22

## Summary

Atmos now recognizes any non-tag-name character immediately following a YAML function tag, restoring `!template` decoding when Go-template whitespace trimming removes the separator.

## Context

Unsupported-tag validation required whitespace or end-of-string after a function name. A block-scalar `!template` value using `{{-` and `-}}` rendered as `!template[...]` or `!template{...}`, leaving the JSON as a literal string instead of decoding it.

## Changes

- Treat any character that cannot continue a YAML function tag name as a valid boundary in the shared matcher.
- Add template fixture coverage for whitespace-trimmed JSON list and map output.
- Cover zero-argument tags, JSON delimiters, and rejected near-miss tag names.

## Validation

- `go test ./internal/exec -count=1`
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh docs/fixes/2026-07-22-template-json-tag-boundaries.md`

## Follow-ups

None.
