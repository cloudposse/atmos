# Fix: Match artifact download version comments to the pinned release

**Date:** 2026-10-09

## Summary

Correct `actions/download-artifact` version comments to `v8.0.1` so SHA
verification compares the pinned commit with its actual release tag.

## Context

The `verify` job on #3339 failed because the moving `v8` tag now points to
`v8.0.2` (`9000827ccba6bdab643e8b6fd33ac0654aef8333`), while the workflow
pins `v8.0.1` (`3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c`).

## Changes

Use the exact `v8.0.1` comment in the VHS workflow and all three attempts in
the artifact download retry action. The pinned executable code is unchanged;
the existing tag-to-SHA verification remains enabled.

## Validation

- Queried upstream GitHub tag refs and verified all four corrected references
  match the `v8.0.1` commit SHA.
- Workflow structure validation with `actionlint -shellcheck=` passed. Full
  actionlint reported only existing shell quoting warnings, with identical
  output before and after this change.
- Affected-file schema, EditorConfig, and CI validation passed.

## Follow-ups

None.
