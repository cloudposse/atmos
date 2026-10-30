# Fix: Implement Terraform backend configuration inspection

**Date:** 2026-10-25

## Summary

Implement `atmos terraform backend list` and `describe` for merged stack configuration.

## Context

Both commands were documented but returned unimplemented errors. Stack-wide listing also attempted component-specific authentication with an empty component name before reaching its stub.

## Changes

Inspect concrete Terraform components without authentication, Terraform execution, or remote backend queries. Respect stack selection, imports, and inheritance, and keep dynamic stack expressions unevaluated. Support deterministic table, JSON, and YAML output. Return explicit missing-selection errors and an empty list when a stack has no configured backends. Retain authentication for create/update/delete. The shared static loader skips locals, workspace templates, and native includes, with isolated decoder caches; it explicitly rejects remote or templated imports. Scoped auth validation now uses this loader too, closing the same execution gap there and reporting deferred auth includes as incomplete validation.

## Validation

Backend and provisioner package tests, focused race tests across commands/loading/decoding, scoped auth race regressions, and the full auth package passed. All 24 disposable CLI checks passed across two runs without fixture changes. Patch lint reported zero issues; skill structure checks passed. The website production build passed with the backend documentation and release notes. The final auth documentation wording was checked separately with the MDX compiler.

## Follow-ups

None.
