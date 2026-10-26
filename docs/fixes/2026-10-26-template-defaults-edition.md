# Fix: edition-aware template enablement defaults

**Date:** 2026-10-26

## Summary

Honor the documented enabled defaults for stack templates, Sprig, and Gomplate while preserving earlier editions.

## Context

A minimal `atmos.yaml` omitted these keys and received Go zero values, disabling templates. The documented defaults and no-config-file fallback already enabled them. A settings-driven conditional-import example exposed the mismatch.

## Changes

Set the three Viper defaults to true and journal their previous false values on 2026-10-26. Earlier pins retain disabled defaults when the settings are omitted; explicit values win. Leave the existing no-config-file fallback unchanged. Regenerate the defaults snapshot, document migration, and remove redundant enablement from the example.

## Validation

Config-loading tests cover unpinned, before/on/after transition, month pins, explicit overrides, and unchanged no-config-file behavior. Snapshot regeneration, journal invariants, the full configuration and edition suites, and `go build ./...` passed. The scoped `atmos test` run passed, and a separate JSON-reporting import/template run recorded 56 passing tests. Direct CLI checks confirmed the edition boundary and numeric dev/prod values of 7 and 366 without explicit template settings. The regenerated cast passed its validator and visual review. Patch-scoped lint reported zero issues; branch-wide lint still reports the pre-existing complexity violation in `cmd/secret/set.go`. EditorConfig for affected files and `git diff --check` passed. All 113 website prebuild tests and the production website build passed; the final prose edits were also compiled directly as MDX.

## Follow-ups

None.
