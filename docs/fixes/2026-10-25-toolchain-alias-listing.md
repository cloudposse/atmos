# Fix: Implement documented toolchain alias listing

**Date:** 2026-10-25

## Summary

Implement `atmos toolchain aliases` and restore its command, configuration, and skill documentation.

## Context

The command was documented but absent from the CLI. The field-test changes removed its documentation instead of providing the useful listing capability.

## Changes

List effective built-in and merged-configuration aliases, with configured names taking precedence. Sort the table by alias name and expose JSON/YAML mappings through `--format`. Preserve raw targets without registry resolution. Listing does not install tools or modify toolchain files. Restore examples and add command documentation and release notes.

Also clarify that OCI vendoring accepts files from any OCI image without an Atmos-specific format or repository layout. Use generic image references rather than implying a particular publisher or registry is required.

## Validation

Focused command/output tests, focused race tests, the full command subpackage suite, and patch-scoped custom-gcl lint pass. New command statement coverage is 91.18%. Eight disposable CLI cases plus a repeat run verify built-in defaults, imports, overrides, table/JSON/YAML output, flag/environment precedence, rejected inputs, and unchanged files. The full Go build and website production build pass; all 114 website prebuild tests pass. Skill structural validation also passes. Evidence is retained in `.context/quality-goal/batch6/`.

The Linux and Windows shard-7 acceptance failures were the same stale parent-command help snapshot: it omitted the new `aliases` entry. Regenerated that snapshot with the acceptance harness; no runtime behavior or workflow policy changed. The parent, install, and info help acceptance cases and the cross-platform line-ending comparison test all pass on two consecutive runs.

## Follow-ups

None.
