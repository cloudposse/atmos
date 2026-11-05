# Fix: Preserve configured tool versions and relative vendor sources

**Date:** 2026-10-24

## Summary

Aggregate toolchain listings retain each configured entry's versions, and relative `file://` vendor sources resolve from their manifest or component directory.

## Context

When an alias and its canonical tool name both appeared in `.tool-versions`, aggregate output read the canonical entry for both. URL parsing also placed the `.` or `..` prefix of relative file URLs in the host field, so vendoring incorrectly resolved them from the filesystem root.

## Changes

- Read names, versions, and defaults from the exact configured entry, including available-version output with `--all`. Preserve aliases in aggregate plain, table, and JSON output so aliases and canonical entries remain distinguishable. Keep named-tool output unchanged. Continue using the canonical tool identity for release and installation lookups.
- Preserve relative file URL prefixes and resolve vendor sources and component mixins against their configuration directory. Keep absolute URL and Windows drive/UNC handling intact.
- Clarify that independent version tracks retain their values while descendants can inherit edits, and identify placeholder vendor progress output as illustrative.
- Separate third-party and Atmos imports in the reviewed tests. Guard referenced configuration fields at compile time and assert the contents of every aggregate result and the final pagination result.

## Validation

Regression tests reproduced the alias/default contamination and relative URL failures before the fixes. Focused tests passed across toolchain, component vendoring, vendor execution, init source records, registry listing, and inherited version edits. Local vendoring tests copy a source directory containing spaces from both `file://./` and `file://../` while the process runs in a different directory; they also verify single-file classification and absolute-source controls. Available versions use a mocked GitHub API. A follow-up regression reproduced duplicate canonical names in aggregate plain and JSON output before the fix. It now verifies distinct configured names, per-entry defaults, canonical installation status, and unchanged named-tool JSON/plain output. Focused aggregate, named-format, registry-listing, and pagination tests pass. `go build ./...`, patch-scoped custom golangci-lint, the fix-record validator, and `git diff --check` passed.

Additional regression tests exercise aggregate table rendering, invalid and unreadable version configuration, failed release lookups, configured registry precedence, unavailable built-ins, and catalog-free registry inventory. Focused tests pass with all configured-list and named-registry functions covered; coverage thresholds remain unchanged.

## Follow-ups

None.
