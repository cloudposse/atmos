# Fix: Resolve scoped providers during secret key generation

**Date:** 2026-10-24

## Summary

`atmos secret keygen` honors the existing stack, component, and component-type flags when selecting providers.

## Context

On main at `cb23b3f701`, `secret list -s dev` saw stack-scoped providers while `secret keygen -s dev` reported no configured vault.

## Changes

Resolve provider definitions from the selected scope without evaluating secret values. Support provider-only declarations and reject ambiguous provider overrides. Update user documentation and the secrets agent skill. Isolate test key files from workstation defaults.

## Validation

The regression failed before the fix. Ran `go test ./cmd/secret -count=1` and package lint on the isolated PR. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation. Tests use local fixtures and require no cloud credentials.

## Follow-ups

None.
