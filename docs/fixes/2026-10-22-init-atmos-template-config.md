# Fix: Generate a loadable CLI configuration from the atmos template

**Date:** 2026-10-22

## Summary

`atmos init atmos` now generates an `atmos.yaml` that Atmos can load immediately.

## Context

The embedded template emitted `version: 1`. The current CLI configuration defines `version` as a structured version-policy section, so every subsequent command failed with `version expected a map or struct, got int`.

## Changes

Remove the obsolete scalar schema-version field from the embedded CLI template. The scaffold manifest retains its separate template version metadata.

## Validation

`TestFieldTestAtmosTemplateLoads` generates the real embedded template and loads it with `InitCliConfig`. It failed on the numeric version before the change and passed afterward. Logs: `.context/skills-field-test/init-ansible-before.log` and `init-ansible-after.log`.

The rebuilt CLI also generated the template and successfully ran `describe config` from its output (`fixed-init-atmos-generate/load` evidence).

## Follow-ups

None.
