# Fix: multi-component `terraform deploy` verifies the stored planfile per component

**Date:** 2026-10-16

## Summary

`atmos terraform deploy` with `--affected`, `--all` or `--components` downloaded the stored planfile for
each component but never compared it with a fresh plan and never enforced `planfiles.required`. Drift
and a missing stored plan were applied with exit 0. The single-component command rejected both. Bulk
deploy nodes now run the same verification gate as the single-component path.

## Context

`terraformRunWithOptions` in `cmd/terraform/utils.go` returns from the `--affected`, `--all` and
multi-component branches before it reaches `verifyStoredPlanForDeploy`. The per-node hook
`terraformNodeHooks.BeforeWithWriters` fires the `before.terraform.deploy` CI hook, which only downloads
the stored plan and is warn-only by design (`pkg/ci/plugins/terraform/handlers.go`, `onBeforeDeploy`).
Nothing on the bulk path compared the two plans or checked `required`.

Two more problems surfaced while fixing it. The scheduler copies the node info into the executor input
before the before-hook runs, so a plan selected by the hook never reached apply. The hook also injects
the last aggregate auth context into the node info, so a verification plan started inside the hook would
have run with another node's credentials under concurrency.

## Changes

- `cmd/terraform/utils.go`: `BeforeWithWriters` snapshots the node's own `AuthContext` and `AuthManager`
  before `injectLastAuthContext`, runs the user and CI hooks, then calls `verifyStoredPlanForNode`. That
  helper runs `verifyStoredPlanForDeploy` on a shallow copy carrying the node's own auth and copies back
  only `PlanFile` and `UseTerraformPlan`. A no-op for subcommands other than `deploy`. `e.VerifyPlanfile`
  is reached through a package variable so tests can stub the comparison.
- `pkg/scheduler/adapters/terraform.go`: `carryOverVerifiedPlan` copies `PlanFile` and
  `UseTerraformPlan` into the executor input when the hook set `UseTerraformPlan`, never the auth fields.
  Deploy nodes with `provision.workdir` lock their source component directory before their workdir key
  when planfile storage is configured and the verify mode does not resolve to off
  (`deployVerificationUsesSourceDir`), because verification reads and writes `stored.plan.tfplan` and
  `new.plan` in the source directory. Fixed lock order and unique workdir keys rule out a deadlock.
- `pkg/ci/plugins/terraform/handlers.go`: comment on `onBeforeDeploy` names both gates.

Bulk behaviour now mirrors single-component deploy in every verify, required and storage combination,
including the warnings single-component deploy already logs outside CI.

## Validation

- `cmd/terraform/verify_plan_bulk_test.go`: `TestBeforeWithWriters_BulkDeployVerifiesStoredPlan`
  (drift, missing with `required: true`, missing with `verify: fail` and `required` unset, missing with
  `--verify-plan`, matching selects the fresh plan, no storage, `apply` and `plan` untouched) and
  `TestBeforeWithWriters_BulkDeployVerifiesWithNodeAuth`. Both fail without the fix.
- `pkg/scheduler/adapters/terraform_test.go`:
  `TestExecuteTerraformNodeHooksBeforeVerifiedPlanReachesExecutor`,
  `TestExecuteTerraformLocksSourceDirForDeployWorkdirNodes` (112 subtests across four layouts, seven
  verification settings and four subcommands), `TestTerraformDeploySourceResourceKey`,
  `TestDeployVerificationUsesSourceDir`.
- `gofumpt -l` clean, `go build ./...`, `atmos lint --changed` with 0 issues, `go test -count=1` for
  `cmd/terraform`, `pkg/scheduler`, `pkg/scheduler/adapters`, `pkg/ci/plugins/terraform/...` and
  `internal/exec`, `-race` on `cmd/terraform` and `pkg/scheduler/adapters`.
- `atmos test --full` fails only in `pkg/ci/startup` and `pkg/runner/step` (PTY cast tests); both fail
  identically on a clean `main` checkout at 753f2fed.
- Manual: a fixture with two `terraform_data` components (`cluster` depends on `network`), local
  backends and a `local/dir` planfile store. `deploy --affected --include-dependents --ci --verify-plan`
  exits 1 and leaves state untouched when the stored plan differs or is missing, and applies `network`
  then `cluster` when it matches. Single-component rows unchanged. A fixture with one workdir component
  in two stacks at `--max-concurrency 2` runs in parallel without storage or with verify off, and
  serializes with storage and verify warn.
- Not run against real cloud storage (`aws/s3`) or a real `AuthManager`; both are covered by unit tests
  with stubs.

## Follow-ups

- Bare `deploy -s <stack>` with no component is routed through `ExecuteTerraformQuery` and is not
  rejected with `--planfile` or `--from-plan`, unlike the other multi-component selectors. Pre-existing
  and unrelated to verification.
