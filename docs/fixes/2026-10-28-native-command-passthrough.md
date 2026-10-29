# Fix: Preserve native Packer and Helmfile arguments after double dash

**Date:** 2026-10-28

## Summary

Packer `validate ... -- -syntax-only` reaches Packer with the native argument intact.

## Context

Cobra consumes `--` before RunE. The legacy adapter then reparsed the remaining native `-syntax-only` as Atmos shorthand, losing the boundary. Both current main and reconciled PR failed twice against the pinned Packer 1.16.0 builtin-null fixture.

## Changes

The shared command adapter and Helmfile adapter use `flags.SplitArgsAtDash` on Cobra RunE arguments before the synthetic operation prefix is interpreted. Explicit separators from legacy direct callers remain supported. No new flags or Packer-specific string rewrite were added. No stored configuration/default meaning changes; edition gating does not apply.

## Validation

Portable `TestNativeCommandSeparatorPreservesArgv` passed for Packer and Helmfile using the actual test subprocess, preserving order, duplicate values and embedded spaces. Real pinned Packer 1.16.0 init, validate and `-- -syntax-only` each passed twice in independent isolated fixtures. Terraform real plan and plan-diff controls also passed on Terraform 1.15.6 and OpenTofu 1.12.5.

Live evidence and exact command records are retained under `.context/terraform-repair-20261028/contracts/`; `*-isolated-*` cases explicitly use per-fixture encrypted file keyrings.

## Follow-ups

Final integrated validation, website build and affected help-cast regeneration are coordinated in the approved Terraform field-findings repair plan. Real cloud IAM and destructive production state operations were not tested.
