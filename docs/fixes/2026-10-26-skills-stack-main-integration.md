# Fix: Preserve YAML function identity while integrating the skills stack

**Date:** 2026-10-26

## Summary

Reconcile the skills field-test stack with current main without losing literal protection,
static inspection, scoped auth validation, or Terraform lifecycle behavior.

## Context

Main introduced edition-aware native YAML function identity, expanded Terraform hooks,
and updated secret resolution while the dependent PRs were under review. Resolving only
textual conflicts would leave duplicate lifecycle declarations and expose internal YAML
transport values through backend inspection.

## Changes

- Combine edition, literal-preservation, and deferred-include cache policies.
- Keep doctor's context diagnostics and scoped auth validation aware of native tags and
  legacy quoted expressions. Inspect typed global auth values after merging.
- Preserve edition selection during deferred config-import decoding, including environment
  overrides, without treating modern quoted function text as executable.
- Keep main's prepared Terraform hook lifecycle and the stack's captured destroy output.
- Render backend JSON/table expressions on copies and restore native YAML tags for YAML
  output. Preserve source maps and reject dynamic static-inspection inputs.
- Retain current SOPS path handling and both subprocess test helpers. Align YAML function
  skill guidance with editions and keep the merged Starlark skill within its size limit.

## Validation

- Full auth, backend, provisioner, YAML utility, config, doctor, SOPS, and function-source
  package tests passed.
- Focused Terraform lifecycle, literal, static inspection, and edition regression tests
  passed; backend YAML output tests verify native node tags and marker suppression.
- Twenty-four live backend CLI checks passed across two runs with fixture files unchanged.
- Repository skill structure, size, and fence checks passed.

## Follow-ups

None. CI and review approval must be checked against the newly published stack heads.
