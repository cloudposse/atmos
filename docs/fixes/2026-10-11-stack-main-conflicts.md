# Fix: Preserve stack regressions while rebasing onto current main

**Date:** 2026-10-11

## Summary

Resolve overlapping test-isolation and workflow changes when rebasing the CloudFormation PR stack onto main at `f104f5ce9`.

## Context

Main now includes additional auth, toolchain, color, and CI changes. Some test isolation and Go download allowlist fixes overlap with changes already present in the stack.

## Changes

- Preserve main's mock Aqua registry routing and Windows executable-name coverage.
- Retain separate local and mock-routed remote include fixtures, including cache isolation.
- Keep main's ordering where hardened-runner endpoint sets are identical.
- Preserve both the locals benchmark and CloudFormation tag-hint regression when cascading the stack.

## Validation

Go 1.26.9 focused race tests passed for toolchain installation/update, auth whoami, and YAML includes: `pkg/toolchain` (14.858 seconds), `pkg/ai/tools/atmos` (3.140 seconds), and `tests` (12.704 seconds). Combined-stack build and CloudFormation checks are recorded separately in workspace evidence after the cascade.

## Follow-ups

None.
