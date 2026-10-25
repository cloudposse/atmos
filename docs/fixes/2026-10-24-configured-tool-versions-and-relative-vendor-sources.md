# Fix: Preserve configured tool versions and relative vendor sources

**Date:** 2026-10-24

## Summary

Aggregate toolchain listings retain each configured entry's versions, and relative `file://` vendor sources resolve from their manifest or component directory.

## Context

When an alias and its canonical tool name both appeared in `.tool-versions`, aggregate output read the canonical entry for both. URL parsing also placed the `.` or `..` prefix of relative file URLs in the host field, so vendoring incorrectly resolved them from the filesystem root.

## Changes

- Read versions and defaults from the exact configured entry, including available-version output with `--all`. Continue using the canonical tool identity for release and installation lookups.
- Preserve relative file URL prefixes and resolve vendor sources and component mixins against their configuration directory. Keep absolute URL and Windows drive/UNC handling intact.
- Clarify that independent version tracks retain their values while descendants can inherit edits, and identify placeholder vendor progress output as illustrative.
- Separate third-party and Atmos imports in the reviewed tests.

## Validation

Regression tests reproduced the alias/default contamination and relative URL failures before the fixes. Focused tests passed across toolchain, component vendoring, vendor execution, init source records, registry listing, and inherited version edits. Local vendoring tests copy a source directory containing spaces from both `file://./` and `file://../` while the process runs in a different directory; they also verify single-file classification and absolute-source controls. Available versions use a mocked GitHub API. `go build ./...`, patch-scoped custom golangci-lint, the fix-record validator, and `git diff --check` passed.

## Follow-ups

None.
