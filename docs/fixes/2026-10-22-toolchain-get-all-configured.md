# Fix: List configured tools when toolchain get has no argument

**Date:** 2026-10-22

## Summary

`atmos toolchain get` now lists the tools declared in `.tool-versions` as documented.

## Context

The command accepted an optional tool name but passed the empty name to tool-spec parsing, so the documented no-argument invocation failed. The field test reproduced the failure twice.

## Changes

Read configured tool names in deterministic order and reuse the existing version resolution and installation checks. Plain output contains each configured tool name and default version, preserving aliases. JSON returns an array of the existing named-tool shapes: default-version objects normally, version-list objects with `--all`. Named-tool output stays compatible. Empty configuration produces an actionable error.

## Validation

`TestGetAllConfiguredTools` and `TestGetAllConfiguredToolsAvailableVersions` verify multiple tools, ordering, defaults, output shapes, and per-tool remote limits. Focused toolchain tests passed twice. Actual CLI probes installed a local HTTP tool and verified installed/uninstalled JSON flags, plain/table formats, named-tool controls, and empty/missing-file errors. The original regression failed before the change. Scoped changed-line lint passed.

## Follow-ups

None.
