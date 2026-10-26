# Fix: demonstrate conditional imports across environments

**Date:** 2026-10-26

## Summary

Show one shared import condition selecting different log retention periods for dev and prod.

## Context

The previous example repeated the condition beside a literal `pci: true` or `pci: false` in each stack. That obscured why conditional imports are useful: an environment's classification should select behavior defined once in shared configuration. Review also requested log retention instead of an audit-logging toggle.

## Changes

Both environments import `catalog/environment`, which imports the app defaults and conditionally includes the PCI mixin. Dev declares `pci: false` and retains 7 days; prod declares `pci: true` and selects 365 days. Update the Terraform input, blog, import reference, README, custom test command, and generated cast together. The durations illustrate configuration selection rather than a complete compliance policy.

## Validation

The example's `atmos test` passed: stacks validate, label and tag selectors identify `prod/app`, and descriptions report 7 days for dev and 365 for prod. Replacing the shared label condition with tag membership in an isolated copy produced the same results. Terraform formatting and the regenerated cast validator passed; the rendered cast was visually checked. EditorConfig, `git diff --check`, all 113 website prebuild tests, and the production website build passed.

## Follow-ups

None.
