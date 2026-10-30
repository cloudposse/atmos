# Fix: Align scaffold merge guidance with execution

**Date:** 2026-10-22

## Summary

Scaffold guidance accurately describes failure on manual conflicts, the configurable change threshold, and the recorded initial-generation merge base.

## Context

The skill reference claimed that manual conflicts succeeded, `--max-changes` did not exist, and updates always used the current `HEAD`. Runtime tests contradicted those descriptions.

## Changes

Document nonzero exits for unresolved manual conflicts, `--max-changes` with default 50 and zero disabling its threshold, and the recorded initial-generation base with a `HEAD` fallback. Align the command page's base-ref and conflict descriptions.

## Validation

Disposable generation/update tests cover default and explicit base refs, manual/ours/theirs conflicts, dry-run byte preservation, and `--max-changes=0`. A separate generation with `--git`, followed by a committed local customization, proves updates retain the original-generation base. Negative thresholds fail with exit code 2.

## Follow-ups

None.
