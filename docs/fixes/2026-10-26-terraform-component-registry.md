# Fix: Route Terraform execution through the component registry

**Date:** 2026-10-26

## Summary

Terraform and OpenTofu execution now select the registered Terraform component provider. This is the first layer of the Terraform lifecycle extraction stack.

## Context

The component registry existed, but Terraform execution called `internal/exec.ExecuteTerraform` directly. Adding another configuration provider would otherwise require more special cases in the internal pipeline. Execution options also belonged to `internal/exec`, preventing a provider from accepting them without a reverse dependency.

## Changes

- Add `pkg/component/terraform.Provider` with injected lifecycle services and no import of `internal/exec`.
- Make `ExecuteTerraform` a compatibility dispatcher. Preserve the existing private lifecycle behind the host runtime so CLI, scheduler, migration, and plan comparison callers all use the registry.
- Move execution options into `pkg/component`. Retain the internal type alias and compatibility constructors. Preserve streams, capture, cancellation, environment presence, and reporting callbacks.
- Expose the existing configuration-artifact generation step for prepared components. Keep varfile generation and full lifecycle extraction in the subsequent layer.
- Restore original registry providers in the dependency-index test helper. Its previous cleanup left the registry empty and caused the newly registered Terraform provider to disappear in full-package tests.
- Document the stack and removal criteria in [the refactor plan](../prd/terraform-component-registry-refactor.md).

## Validation

- `go build ./...` passed.
- Focused integration tests passed for registry dispatch, scheduled output capture, cancellation, backend generation, policy errors, bulk dry runs, retries, and UI stream options.
- `go test ./cmd/terraform -count=1 -timeout=10m` passed.
- Full tests for `pkg/component`, `pkg/component/terraform`, and `internal/exec` passed with a coverage profile (7,771 passing test/subtest events, 49 skips, zero failures).
- Local added-executable-line coverage against `origin/main`: **164/169 = 97.04%**. The new provider and moved execution-option implementation have 100% coverage of added executable lines. Tests and generated mocks are excluded. Uncovered lines are three existing stderr redirection/capture branches with renamed fields, the missing-provider diagnostic, and an unreachable registration panic.
- This coverage check uses the touched packages' own tests, not full-suite `-coverpkg=./...` breadth. CI's Codecov report remains authoritative.
- `atmos lint --changed` initially identified formatting/profiling requirements. After fixes, its underlying `custom-gcl run --config=.golangci.yml --allow-serial-runners --new-from-rev=origin/main` check passed with zero issues.
- `git diff --check` and the fix-document validator passed.

## Follow-ups

Continue the [refactor stack](../prd/terraform-component-registry-refactor.md). The private legacy runtime is a transition boundary; the remaining Terraform lifecycle must move out of `internal/exec` in subsequent layers. The Terragrunt prototype and external plugin work remain separate.
