# Fix: Resolve scoped providers during secret key generation

**Date:** 2026-10-24

## Summary

`atmos secret keygen` honors the existing stack, component, and component-type flags when selecting providers.

## Context

On main at `cb23b3f701`, `secret list -s dev` saw stack-scoped providers while `secret keygen -s dev` reported no configured vault.

## Changes

Resolve provider definitions from the selected scope without evaluating secret values. Support provider-only declarations and reject ambiguous provider overrides. Update user documentation and the secrets agent skill. Isolate test key files from workstation defaults.

Preserve provider-only stacks even when no component context exists: load their imported manifest data through the existing stack loader, reuse stack-name resolution, and evaluate only providers and their local dependencies in `pkg/secrets/scoped`. Reuse the component registry when filtering and describing stacks so registered component types retain their providers. Explicit component/type selectors still require a matching component before inheriting a stack-level provider; a missing selector cannot silently fall back to another scope. The shared `internal/exec` processor now contains fewer hardcoded type lists. Separate scoped configuration loading and section selection from key generation so the new scope handling stays within the existing cyclomatic complexity limit. Guard the naming-mode fixture rewrite so changes to the shared fixture cannot silently stop exercising pattern and filename selection.

Merged current main on 2026-10-25 while retaining both scoped-keygen guidance and the independently added stack-scoped set/validate guidance. Updated provider-resolution test inputs to the explicit YAML function tag representation now required by main; bare strings remain literals. Carried over the existing Terraform init-hook test isolation fix after reproducing its shuffled-test state leak on the merged tree. No production changes were needed for this integration.

The provider error-propagation regression now runs the Go test binary as a failing subprocess instead of using a shell builtin. Provider resolution must preserve the typed exit code 7, which also proves that the helper process received its environment and ran; startup failures cannot satisfy the assertion. The invalid-template case remains, and the stack naming schema field has a compile-time guard.

## Validation

The regression failed before the fix. Ran `go test ./cmd/secret -count=1` and package lint on the isolated PR. Changed-code lint, `go build ./...`, and the website build passed on the combined implementation. Tests use local fixtures and require no cloud credentials.

The provider-only and registered-type regressions both failed before the review fixes. The full `cmd/secret` suite and focused stack description, stack-name, empty-stack and component-filter tests pass. Coverage and focused provider-evaluation tests also exercise imported providers, all stack naming modes, templated key-file paths, missing providers/stacks, explicit missing component/type selectors, malformed manifests and provider evaluation failures. Changed-code lint and a CLI build pass. A full lint pass over the secret command and scoped provider packages verifies the key-generation complexity findings are resolved; it still reports existing findings in `cmd/secret/list.go` and `cmd/secret/set.go`, outside this PR.

Integration validation on 2026-10-25: the two explicit-tag provider fixtures failed before their migration and pass afterward. The full `pkg/secrets/...` and `cmd/secret/...` suites, focused stack-description/validation regressions, `go build ./...`, and changed-code lint pass on the merged tree. The deterministic Terraform hook race/shuffle reproduction failed before the carried-over isolation fix and passes afterward. The entire `cmd/terraform` package matches the previously validated isolation commit, so its earlier full short-mode race-suite result was reused; that full suite was not rerun for this integration.

The native subprocess review fix passes its focused error-propagation test and the full `pkg/secrets/scoped` suite. Building `pkg/secrets/scoped` and `cmd/secret`, changed-code lint, and fix-log validation also pass. Runtime validation was on macOS; no Windows runtime result is claimed.

## Follow-ups

None.
