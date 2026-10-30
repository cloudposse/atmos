# Fix: Preserve native filenames when vendoring local files

**Date:** 2026-10-22

## Summary

Local file vendoring preserves native filesystem names, including Windows drive paths and filenames containing `#`.
The vendor acceptance fixture uses a relative filesystem source for its local components.

## Context

URL parsing treated Windows drive paths as opaque URLs with an empty path. File staging then tried to copy a file onto its temporary directory, failing the Windows complete-fixture workflow. Parsing local filenames as URLs also truncated names containing `#`. Separately, an acceptance fixture used an absolute `file:///../../../fixtures/...` URI for an intended relative source; preserving absolute URI semantics exposed that invalid path on Linux and macOS.

## Changes

Extract native absolute path basenames before URL parsing and preserve staged filesystem names when choosing the final destination. Correct the relative fixture source and remove the integration helper's special rewrite of the invalid URI so tests exercise the checked-in source representation.

## Validation

The vendor and installer package suites pass. Focused execution tests pass for absolute file URIs, the checked-in local fixture sources, and a local Git vendoring workflow. Regression tests cover native and slash-normalized paths, special filenames, directory and renamed-file targets, unchanged source bytes, and rerun receipts. The rebuilt CLI passes eight disposable-fixture invocations covering both local component sources and both complete-fixture local-file operations, including reruns and copied-byte checks. Native Windows runtime verification runs in CI; local verification used macOS.

## Follow-ups

None.
