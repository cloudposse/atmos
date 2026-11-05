# Fix: Set and validate stack secrets without a component

**Date:** 2026-10-24

## Summary

Stack-scoped secrets can be written and validated using only `--stack`.

## Context

On main at `cb23b3f701`, `secret set NAME=value -s dev` and `secret validate -s dev` required an unrelated component for stack-scoped declarations. SOPS did not support the suggested global-scope workaround.

## Changes

Allow componentless writes for consistent stack/global declarations and providers across inherited contexts. Validate shared declarations across the stack while retaining the component requirement for instance-scoped operations. Add offline SOPS roundtrip coverage and update user and agent documentation.

## Validation

Regressions failed before the fix. Ran `go test ./cmd/secret -count=1` and package lint on the isolated PR, including local write/validate coverage and ambiguous-context rejection. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation. Test key sources are isolated from workstation defaults.

## Follow-ups

None.
