# Fix: Validate scoped auth with the shared static stack loader

**Date:** 2026-10-26

## Summary

Scoped auth validation rejects unresolved import paths instead of validating an incomplete stack.

## Context

Disabling templates in the ordinary loader leaves unmatched templated imports tolerated for callers that need that behavior. The scoped auth command could consequently report valid configuration while omitting identities from a required import. Disposable CLI fixtures reproduced this with both default and custom template delimiters: the templated import passed, while the equivalent literal import correctly exposed an invalid identity chain.

The shared static loader already implemented for Terraform backend inspection provides the required restrictions. Its implementation must be available in the earlier auth PR so scoped validation is safe independently of later stack layers.

## Changes

Reuse the shared `DescribeStacksErrorOptions.StaticOnly` loading path. It rejects dynamic and remote imports before optional-import handling or fetching, propagates static loading through nested imports, isolates evaluated caches, defers native includes, and skips locals and workspace synthesis. Static stack name templates retain pure Go built-ins. Ordinary callers keep their existing loader behavior.

Move the existing loader and decoder regression tests alongside the scoped auth caller. Add command-level required and optional import tests with default and custom delimiters, plus literal valid, invalid-identity, and optional-missing controls. Preserve edition-aware handling of unresolved native and legacy include expressions.

## Validation

Before moving the implementation, freshly built auth-layer and source-management-layer binaries ran the same five CLI fixtures. The auth layer incorrectly accepted all three dynamic-import cases; the source-management layer rejected them. Both versions rejected the literal invalid-identity control and accepted the literal valid control.

The scoped auth, static stack loading, and static include tests passed under race detection and shuffled order. The full auth package and ordinary stack/import and native include regressions also passed. The CLI built successfully, scoped custom lint reported zero issues, and diff checks passed. Re-running all five CLI fixtures with the rebuilt auth layer rejected every dynamic import and preserved both literal control results.

## Follow-ups

None.
