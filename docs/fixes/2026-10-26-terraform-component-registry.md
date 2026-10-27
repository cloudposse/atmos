# Fix: Route Terraform execution through the component registry

**Date:** 2026-10-26

## Summary

Terraform and OpenTofu execution now select the registered Terraform component provider. The same PR also moves Terraform generation, plan comparison and verification, tool detection, variable handling, paths, and workspace cleanup into the component package.

## Context

The component registry existed, but Terraform execution called `internal/exec.ExecuteTerraform` directly. Adding another configuration provider would otherwise require more special cases in the internal pipeline. Execution options also belonged to `internal/exec`, preventing a provider from accepting them without a reverse dependency.

## Changes

- Add `pkg/component/terraform.Provider` with injected lifecycle services and no import of `internal/exec`.
- Make `ExecuteTerraform` a compatibility dispatcher. Preserve the existing private lifecycle behind the host runtime so CLI, scheduler, migration, and plan comparison callers all use the registry.
- Move execution options into `pkg/component`. Retain the internal type alias and compatibility constructors. Preserve streams, capture, cancellation, environment presence, and reporting callbacks.
- Move 15 complete Terraform implementation files into `pkg/component/terraform`. Extract artifact generation, variable protection, paths, and workspace cleanup from mixed files.
- Inject shared stack, template, deferred-resolution, naming, and execution services through `OperationServices`. Preserve the original generation order and error identity. Keep narrow compatibility entry points and remove unused private wrappers.
- Move pure behavioral tests alongside their implementations. Retain host integration tests and use scoped cross-package coverage to measure them.
- Keep the remaining init/apply lifecycle, scheduling, hooks, and state/output resolution in the host runtime for the next extraction.
- Restore original registry providers in the dependency-index test helper. Its previous cleanup left the registry empty and caused the newly registered Terraform provider to disappear in full-package tests.
- Document the stack and removal criteria in [the refactor plan](../prd/terraform-component-registry-refactor.md).

## Validation

Validation covers the component registry, Terraform component implementation, and host adapters. The scoped coverage run instruments all three packages so host integration tests count toward the extracted code. CI's full-suite Codecov report remains authoritative.

The tests verify registry dispatch, scheduled capture, cancellation, secret-safe varfiles, deferred configuration, generated artifacts, plan comparison, independent injected hosts, and stopping after a generation or execution error.

- `go build ./...` passed.
- Full tests for `pkg/component`, `pkg/component/terraform`, and `internal/exec` passed: 7,784 passing test/subtest events, 49 skips, zero failures.
- Local rename-aware patch coverage against `origin/main`: **543/580 added executable lines (93.62%)**. The scoped run uses `-coverpkg=./pkg/component,./pkg/component/terraform,./internal/exec`; it does not measure full-suite coverage. Counting four uninstrumented type-switch labels as misses still gives 92.98%.
- `go test ./cmd/terraform -count=1 -timeout=10m` passed.
- Patch-scoped `custom-gcl` lint passed with zero issues. Existing batch-generator complexity is retained with function-scoped annotations to keep evaluation order unchanged during extraction.
- `git diff --check` and the fix-document validator passed.


## Review corrections

The provider guide now includes every `ExecutionContext` field and distinguishes custom self-registration from the host's Terraform registration. Compatibility helpers have corrected and expanded documentation, the execution-option test uses the required import groups, and the native-CI overview links to the moved plan-diff implementation. The refactor plan now lists the remaining extraction boundaries.

Validation for these documentation and formatting corrections: the application build, component and Terraform-component tests, patch-scoped lint, and offline Markdown link checks passed. No execution behavior changed.

## Follow-ups

Continue the [refactor stack](../prd/terraform-component-registry-refactor.md). The private legacy runtime is a transition boundary; the remaining Terraform lifecycle must move out of `internal/exec` in subsequent layers. The Terragrunt prototype and external plugin work remain separate.
