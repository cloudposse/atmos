# Fix: Infer the source for in-place init updates

**Date:** 2026-10-22

## Summary

Running `atmos init --update` inside a generated project reuses its saved scaffold source and updates the current directory without requiring a positional template.

## Context

Although generation saves `.atmos/scaffold.yaml`, a source-less noninteractive update reached template selection before reading that record and failed with `template name must be provided`.

## Changes

Resolve omitted update sources from the current project's record. Embedded sources use the saved template name; external sources retain their original location. Saved answers and explicit overrides still flow through the existing generator merge logic. Explicit sources retain existing behavior, and interactive calls outside a recorded project retain the template/target prompts. A noninteractive call without a record receives an actionable error.

## Validation

`TestFieldTestInitUpdateFromRecordedTemplate` failed before the fix and passed afterward. It generates a project, commits its initial files in a disposable Git repository, adds a user customization, and updates without a positional template. The original answer and customization survive. Additional source-resolution tests cover explicit sources, new initialization, missing records, and recorded external locations. Initial before/after logs are in `.context/skills-field-test/init-ansible-{before,after}.log`.

Actual noninteractive `init --update` and `init --update --merge-strategy theirs` both succeeded in a disposable standalone Git repository (`fixed-init-standalone-update-0/1`).

## Follow-ups

None.
