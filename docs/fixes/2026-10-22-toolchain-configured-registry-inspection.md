# Fix: Inspect configured toolchain registries by name

**Date:** 2026-10-22

## Summary

`atmos toolchain registry list NAME` now loads the named configured registry, and the no-argument command reports the effective registry descriptors.

## Context

The inspection command hardcoded Aqua names. A working local registry could install tools but inspection rejected its configured name. The field test reproduced the rejection twice.

## Changes

Resolve exact configured names through the same registry constructors used for installation. Keep Aqua aliases and expose the built-in Atmos registry. The no-argument JSON/YAML formats return registry names, types, and priorities; text output reflects the same configuration. Unknown names retain the unknown-registry error. Inline registry results now use deterministic name order and honor offset/limit options, which were previously ignored.

## Validation

`TestInlineRegistryListPagination` verifies repeated stable pages and offset bounds. `TestListConfiguredNamedRegistry` checks actual inline registry contents, descriptor output, and unknown-name errors. Focused toolchain and registry tests passed twice. Actual CLI probes passed local registry table/JSON/YAML output, pagination, no-argument output formats, and repeated unknown-name controls. The original local-registry regression failed before the change. Scoped changed-line lint passed.

## Follow-ups

None.
