# Fix: Align termination-protection docs with the companion implementation

**Date:** 2026-10-09

## Summary

The component and delete references describe apply as enabling termination protection without
disabling it when the setting is false.

## Context

The earlier #3136 layer reconciles termination protection in both directions, so its reference
pages document that setting false and re-applying disables protection. The #3137 companion
implementation adds an early return for false, preserving existing protection. This documentation
change belongs alongside that implementation, after the #3136 correction propagates through the stack.

## Changes

- Restore the enable-only apply guidance in both reference pages.
- Keep `--disable-termination-protection` as the explicit deletion escape hatch.
- Keep PR-stack history in fix records rather than user-facing reference pages.

## Validation

- Verified `applyTerminationProtection` returns without an API call when the setting is false.
- Focused termination-protection and apply tests passed at companion head
  `f99bd703d367b17680178d7f461c574159754fd8`.
- Fix-log validation and `git diff --check` passed.
- The combined website build runs after the paired documentation patches are applied at their
  respective owning layers.

## Follow-ups

None.
