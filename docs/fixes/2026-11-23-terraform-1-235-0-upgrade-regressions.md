# Fix: Terraform upgrade regressions found before 1.235.0

**Date:** 2026-11-23

## Summary

Three behavior changes found by the 1.235.0 regression run against v1.234.1 are fixed:

1. An after-plan hook reported success when `terraform plan` failed with any exit code `2` before
   Terraform ran (for example an SSO identity that "needs an interactive terminal"). Only the plan
   subprocess's own exit `2` with `-detailed-exitcode` now counts as a successful plan with changes.
2. `atmos terraform plan <c> -s <s> -- --out=my.tfplan` failed with "plan verification failed: stored
   plan differs from current state: multiple plan output flags". `--out`, `--out=X` and `--out X` are now
   recognized exactly like `-out`, so Atmos no longer adds its own planfile next to the user's.
3. A plain (non-Terragrunt) component whose free-form `settings` contained `terragrunt.exclude` was
   silently skipped by `plan --all` and `apply --all` (exit 0, `if: false` ignored). The exclude block is
   now honored only on units the Terragrunt configuration provider owns.

## Context

All three were introduced after v1.234.1 and reproduced with the regression run's fixtures
(`review/bin/hookexit2.sh`, `review/bin/dashout.sh`, `terragrunt/bin/nb.sh tg-exclude`).

1. `terraformHookOutcome` (cmd/terraform/utils.go, #3562) mapped every after-plan error with exit code
   `2` to `RunSuccess` unless it was a `PrerequisiteError` or a policy error. Its comment said exit `2`
   counts only when the plan subcommand produced it, but nothing checked that. An auth failure in the
   command layer carries exit `2`, so the success hook ran and the failure hook did not; v1.234.1 ran the
   failure hook. The bulk scheduler (`terraformPlanChangedError`) and the operation-policy status
   (`policyOperationStatus`) used the same "any exit 2" mapping, so a node failing with exit `2` was
   reported as changed and its dependents ran, and policy input said `success`. Those two mappings already
   existed in v1.234.1; they are fixed here so every consumer agrees with the hook outcome. The CI exit code is
   derived from the hook outcome (`terraformCIExitCode`), so it reported "changes" for the auth failure.
   The executor already marked the real plan status internally (`successfulPlanChanges`, set only when
   the plan command returned `2` with `-detailed-exitcode` in effect), but the marker was unexported.
2. `buildPlanSubcommandArgs` (pkg/component/terraform/execute_helpers_args.go, #3576) checked only for
   `-out` and `-out=`, so with `--out=my.tfplan` it also appended `-out <atmos planfile>`.
   `validatePlanOutputFlags` (generate_planfile.go), which already knows both dash forms, then rejected the
   two destinations. The TUI plan path (`extractOutFlag`) and the saved-plan policy guard
   (`planSelection`) also recognized only `-out`, so with `--out` the TUI planned into a temporary file
   instead of the user's, and the policy guard reported "plan policies require a saved plan" (both behaved
   the same in v1.234.1 and are fixed for consistency). Terraform
   parses flags with Go's flag package, which accepts one or two dashes.
3. `terraformExcludeBlock` (pkg/scheduler/adapters/terraform_exclude.go, #3598) read
   `settings.terragrunt.exclude` from every graph node. The Terragrunt provider writes that block for its
   units after evaluating `if`; on a plain component it is free-form user data, and `if` is never evaluated.

The intended behavior recorded in `docs/fixes/2026-10-28-terraform-detailed-plan-hooks.md` is kept: a real
plan that returns `2` with `-detailed-exitcode` runs success hooks once, does not run failure hooks, and
keeps exit `2`.

## Changes

- `errors/exit_code.go`: new `DetailedPlanChangesError` marker with `AsDetailedPlanChanges` and
  `IsDetailedPlanChanges`. Message, chain and exit code are unchanged.
- `pkg/component/terraform/execute_hooks.go`, `execute_recovery.go`: the internal `successfulPlanChanges`
  is now an alias of the exported marker, so the plan phase's existing classification (plan command,
  exit `2`, `-detailed-exitcode` in arguments or `TF_CLI_ARGS*`, not a hook failure) is visible to callers.
- `pkg/component/terraform/execute_terragrunt.go`: executable-mode `terragrunt run -- plan` results are
  classified the same way, so a Terragrunt plan with changes keeps its success hooks.
- `cmd/terraform/utils.go` (`terraformHookOutcome`): exit `2` is a successful plan only when the error
  carries the marker; prerequisite and policy errors still fail. `terraformCIExitCode` follows the outcome,
  so CI reports exit `1` with errors for an exit `2` that is not the plan's own.
- `pkg/scheduler/adapters/terraform.go` (`terraformPlanChangedError`) and `pkg/hooks/policy.go`
  (`policyOperationStatus`): the same marker requirement.
- New `pkg/terraform/planout` package recognizing `-out`, `-out=X`, `--out`, `--out=X`; used by
  `buildPlanSubcommandArgs`, `validatePlanOutputFlags`, `containsOutputFlag`, `plannedOutputPath`, the TUI
  `extractOutFlag`, and the policy guard's `planSelection` (which keeps the flag spelling it replaces).
- `pkg/scheduler/adapters/terraform_exclude.go`: `terraformExcludeBlock` returns a block only when
  `schema.ConfigurationOwned(settings)` holds and `settings.configuration.provider` is `terragrunt`.
- Docs: `website/docs/automation/lifecycle-hooks.mdx` (which exit `2` counts as success),
  `website/docs/cli/commands/terraform/terraform-plan.mdx` (`--out` forms),
  `website/docs/migration/terragrunt.mdx` (exclude applies only to provider-supplied units).
- Tests: `cmd/terraform/plan_hook_outcome_test.go`, `errors/exit_code_test.go`,
  `pkg/component/terraform/plan_changes_marker_test.go`, `pkg/component/terraform/plan_output_flags_test.go`,
  `pkg/hooks/policy_test.go`, `pkg/policy/planfile/arguments_test.go`,
  `pkg/scheduler/adapters/terraform_exclude_test.go`, `pkg/scheduler/adapters/terraform_plan_changes_test.go`,
  `pkg/scheduler/adapters/terraform_test.go`, `pkg/terraform/planout/planout_test.go`,
  `pkg/terraform/ui/executor_test.go`. Existing tests that used a bare exit `2` as "plan with changes" now
  use the marker the executor returns; the end-to-end `planCmd` test sets `TF_CLI_ARGS_plan=-detailed-exitcode`
  for its child process, and a new negative test runs the same child without it.

## Validation

- Each regression test was written first and failed on the unfixed code: `TestTerraformHookOutcomePreservesDetailedPlanStatus/exit_two_before_terraform_ran`,
  the matching `TestRunCIHooksForNodePreservesDetailedPlanStatus` and `TestRunHooksOnErrorPreservesDetailedPlanStatus` cases,
  `TestPolicyPlanDetailedExitCode`, `TestExecuteTerraformPlanExitTwoOutsideThePlanIsAFailure`,
  `TestExecuteTerragruntUnitMarksDetailedPlanChanges/with_-detailed-exitcode`, `TestBuildPlanArgsHonorsEveryUserPlanOutputForm/--out*`,
  `TestExtractOutFlag/double-dash_*`, `TestSelectArtifact/plan_double-dash_*`, `TestRejectUnsafeArguments/out_duplicate_mixed_dashes`,
  `TestExecuteTerraformIgnoresExcludeBlockOnComponentsTheProviderDoesNotOwn` and `TestTerraformExcludeBlockRequiresTerragruntOwnership`.
  All pass after the fix.
- `go build ./...` and `go vet` on the changed packages passed.
- `go test` passed for `./errors/`, `./cmd/terraform/...`, `./pkg/component/terraform/...`, `./pkg/ci/...`, `./pkg/hooks/...`,
  `./pkg/policy/...`, `./pkg/scheduler/...`, `./pkg/terraform/ui/...` and `./pkg/terraform/planout/...`.
- `go test -timeout 40m ./cmd/... ./pkg/... ./errors/... ./internal/...`: every package passed except `pkg/config/schema`,
  whose generator failed with `lstat pkg/config/testdata-cwd-fallback: no such file or directory`. That directory is created and
  removed by a `pkg/config` test running at the same time; `go test ./pkg/config/schema/` passed on its own, and this change does
  not touch either package.
- Patch coverage: a merged profile for the eight changed packages (`-coverpkg` over the same packages) covers 71 of 71
  instrumented added or changed non-test lines (100%), computed from `git diff -U0 origin/main` against the profile.
- Manual reproduction with a binary built from this branch, using copies of the regression run's fixtures and Terraform 1.11.0
  (the candidate binary from the regression run as the control):
  - SSO identity failing before Terraform ran: candidate ran the success hook (`success status=success code=2`), the fixed binary runs
    the failure hook (`failure status=failure code=2`), matching v1.234.1; exit stays `2`. The control plan with `-- -detailed-exitcode`
    still runs the success hook with exit `2` on both.
  - `-- --out=my-dd.tfplan` and `-- --out my-ds.tfplan`: candidate exit `1` with "multiple plan output flags"; fixed exit `0` and the
    named planfile is written. `-- -out=my-sd.tfplan` works on both.
  - `terraform plan --all` with free-form `settings.terragrunt.exclude` (`if: false`) on the plain `db` component: candidate skipped `db`
    ("excluded by the Terragrunt exclude block"); fixed plans both `app` and `db`.
- Not run: golangci-lint (the tool invocation was denied in this environment, so lint was not checked locally; CI lint is the first
  lint pass), the `./tests` acceptance package, and the website build (`website/node_modules` is not installed in this worktree).

## Follow-ups

None.
