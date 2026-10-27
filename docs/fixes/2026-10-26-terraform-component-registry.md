# Fix: Give Terraform a component-owned runtime

**Date:** 2026-10-26

## Summary

Terraform and OpenTofu now select a registered component provider whose runtime lives in `pkg/component/terraform`. The same PR reduces `internal/exec` from 34 `terraform*.go` production files to one host adapter, `terraform.go`.

## Context

Terraform execution bypassed the component registry and owned its lifecycle in `internal/exec`. Other configuration providers could not reuse the lifecycle without adding special cases to the shared execution package. Execution options and shared data contracts also belonged to that package.

## Changes

- Register `pkg/component/terraform.Provider` with a component-owned `Executor`. Keep registry dispatch and shared-service composition in one host adapter.
- Move initialization, workspaces, authentication orchestration, hooks, arguments, retries, policy, UI routing, shell sessions, bulk/query/affected execution, generation, plans, and state/output lookup into the component package.
- Bind shared host services per executor. `NewTerraformExecutor` creates an independent executor without changing the registry. The component package does not import `internal/exec`.
- Move execution options and shared data contracts into `pkg/component`. Retain compatibility entry points and type aliases. Preserve streams, capture, cancellation, explicit empty environments, credentials, cache identity, and reporting callbacks.
- Move unit tests with the implementation. Bind tests that need real stack/process services through an external host fixture. Keep host integration tests and test-only compatibility helpers in `internal/exec`.
- Add portable affected-dispatch tests using temporary Git repositories and tests for execution-service failures, scheduler selection, authentication, and wrapper forwarding. Preserve the existing assertions when moving tests.
- Update the provider guide and the [refactor requirements](../prd/terraform-component-registry-refactor.md) to describe the completed lifecycle boundary.

## Validation

- `go build ./...` passed.
- Full tests for `pkg/component/terraform`, `internal/exec`, `pkg/component`, and `cmd/terraform` passed: **8,572 passing test/subtest events, 51 skips, zero failures**. No existing test declarations were lost.
- Local rename-aware patch coverage: **1,659/1,801 added executable lines (92.12%)**. Counting all 40 uninstrumented syntax/label lines as misses gives **90.11%**. The run instruments the component, Terraform component, and host packages across all four test packages. This is scoped coverage; CI Codecov remains authoritative.
- Patch-scoped `custom-gcl` lint passed with zero issues. Existing lookup and shell control flow retains narrow function annotations to preserve behavior during extraction.
- Mock generation, offline links in the changed guides, `git diff --check`, and the fix-document validator passed.

## Follow-ups

Define independent discovery and preparation capabilities, then adapt them to the external plugin protocol and Terragrunt integration. These remain separate work in the [refactor plan](../prd/terraform-component-registry-refactor.md).
