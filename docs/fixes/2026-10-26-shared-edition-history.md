# Fix: share the edition journal between the CLI and website

**Date:** 2026-10-26

## Summary

Generate a complete public edition history from the same journal embedded in Atmos.

## Context

The configuration reference and CLI documentation contained hand-maintained sample tables that omitted newer changes. The actual journal lived in a Go literal, leaving website readers without a current index.

## Changes

Move the canonical entries to `pkg/edition/journal.yaml`, preserving their scalar types and explanatory comments. Embed and decode that file in the CLI. Generate `/cli/editions` before Docusaurus discovers documentation; the generated Markdown is ignored by Git and regenerated for builds and previews. Place Editions in the Configuration navigation group and link the history near the top of the `atmos list editions` command page. Replace the sample tables with links to the complete history. Update contributor guidance and snapshot instructions to point to the shared source.

Introduce editions by explaining how date pins defer newer defaults until the user opts in. Include an `atmos config set` example and links to configuring, listing, and describing editions.

## Validation

Compared all 24 entries against a pre-migration export: every field is unchanged. Generator tests cover complete coverage, date ordering, typed values, source links, re-reading changed data, malformed entries, and MDX escaping. Go tests cover scalar types, invalid embedded documents, edition resolution, and configuration defaults. Edition command tests, `go build ./...`, patch lint, affected EditorConfig validation, and the production website build passed (117 website tests). Browser verification confirmed all 24 transitions and the shared-source edit link; the history also appears in the generated Markdown exports.

Verified the documented `atmos config set` command against a temporary configuration: both creating and updating the edition write a quoted date string without an explicit type flag. The focused Reference navigation check passed. The broader sidebar test file has an existing Policies expectation that omits two entries already present in HEAD.

## Follow-ups

None.
