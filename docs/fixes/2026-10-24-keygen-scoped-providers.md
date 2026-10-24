# Fix: Resolve scoped providers during secret key generation

**Date:** 2026-10-24

## Summary

`atmos secret keygen` honors the existing stack, component, and component-type flags when selecting providers.

## Context

On main at `cb23b3f701`, `secret list -s dev` saw stack-scoped providers while `secret keygen -s dev` reported no configured vault.

## Changes

Resolve provider definitions from the selected scope without evaluating secret values. Support provider-only declarations and reject ambiguous provider overrides. Update user documentation and the secrets agent skill. Isolate test key files from workstation defaults.

Preserve provider-only stacks even when no component context exists: load their imported manifest data through the existing stack loader, reuse stack-name resolution, and evaluate only providers and their local dependencies in `pkg/secrets/scoped`. Reuse the component registry when filtering and describing stacks so registered component types retain their providers. Explicit component/type selectors still require a matching component before inheriting a stack-level provider; a missing selector cannot silently fall back to another scope. The shared `internal/exec` processor now contains fewer hardcoded type lists. Separate scoped configuration loading and section selection from key generation so the new scope handling stays within the existing cyclomatic complexity limit.

## Validation

The regression failed before the fix. Ran `go test ./cmd/secret -count=1` and package lint on the isolated PR. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation. Tests use local fixtures and require no cloud credentials.

The provider-only and registered-type regressions both failed before the review fixes. The full `cmd/secret` suite and focused stack description, stack-name, empty-stack and component-filter tests pass. Coverage and focused provider-evaluation tests also exercise imported providers, all stack naming modes, templated key-file paths, missing providers/stacks, explicit missing component/type selectors, malformed manifests and provider evaluation failures. Changed-code lint and a CLI build pass. A full lint pass over the secret command and scoped provider packages verifies the key-generation complexity findings are resolved; it still reports existing findings in `cmd/secret/list.go` and `cmd/secret/set.go`, outside this PR.

## Follow-ups

None.
