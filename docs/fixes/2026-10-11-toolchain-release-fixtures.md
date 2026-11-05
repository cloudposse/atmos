# Fix: Consolidate toolchain release fixtures after the main rebase

**Date:** 2026-10-11

## Summary

Register each toolchain-info release once so acceptance snapshots remain deterministic.

## Context

The main rebase combined two independent registrations for replicated, fzf, and jq.
The mock correctly returned both registrations, adding unexpected release rows on
Linux, macOS, and Windows. The jq registrations also disagreed on the version.

## Changes

Keep one table that registers both Aqua metadata and the existing release fixture
for each tool. Preserve main's versions and metadata, including jq-1.8.1, so the
existing snapshots continue to assert the original output without sanitizing rows.

## Validation

Reproduced all three CI snapshot failures locally before the fix. The complete
`TestCLICommands/atmos_toolchain_info` group passes after the fix with local mocked
GitHub releases; snapshots and assertions remain unchanged.

## Follow-ups

None.
