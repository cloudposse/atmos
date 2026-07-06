# `--verify-plan` Requires Planfile Storage

**Date:** 2026-07-03

## Summary

An explicit `--verify-plan` (or `ATMOS_TERRAFORM_VERIFY_PLAN=true`) on `atmos terraform deploy`
silently no-op'd when planfile storage was not configured under `components.terraform.planfiles`
in `atmos.yaml`. The storage gate in `verifyStoredPlanForDeploy` returned before the CLI override
was ever consulted, so deploy ran a plain plan + apply with no stored-plan download, no
verification, no warning, and no error — the user believed their apply was verified when it was not.

Reported via Slack: a user running
`atmos terraform deploy <component> -s <stack> -v --verify-plan=true` in GitHub Actions saw "the
plan is not fetched at all" (note also that `-v` is the `--verbose` shorthand, not `--verify-plan`).

## Changes

### Behavior

| Scenario (no planfile storage configured)      | Before             | After                                        |
|------------------------------------------------|--------------------|----------------------------------------------|
| `--verify-plan` / `ATMOS_TERRAFORM_VERIFY_PLAN=true` | silent no-op | error (`ErrPlanfileStorageNotConfigured`) with a hint pointing at `components.terraform.planfiles` |
| `verify: fail\|warn` set in `atmos.yaml`       | silent no-op       | warning logged, deploy proceeds              |
| Flag unset / `--verify-plan=false`             | silent no-op       | unchanged (silent no-op)                     |

With storage configured, all existing behavior is unchanged.

### Code

| File                                    | Fix                                                                    |
|-----------------------------------------|------------------------------------------------------------------------|
| `cmd/terraform/utils.go`                | `handleUnconfiguredPlanfileStorage` guard inside the storage gate      |
| `errors/errors.go`                      | New sentinel `ErrPlanfileStorageNotConfigured`                         |
| `cmd/terraform/deploy.go`               | Flag help text states the storage requirement                          |

### Documentation

| File                                                          | Fix                                                       |
|---------------------------------------------------------------|-----------------------------------------------------------|
| `website/docs/ci/ci.mdx`                                      | Fixed misleading "opt-in / experimental" passage; storage prerequisite stated; `github-runtime` pointer added to examples |
| `website/docs/ci/planfile-storage.mdx`                        | "Silently skipped" tip now notes the explicit-flag error  |
| `website/docs/cli/commands/terraform/terraform-deploy.mdx`    | `--verify-plan` entry documents the error behavior        |
| `website/docs/components/terraform/planfiles.mdx`             | Per-run overrides passage documents the error behavior    |
| `website/docs/cli/configuration/components/terraform.mdx`     | Added missing `planfiles` config-reference section        |

### Test Coverage Added

- `cmd/terraform/verify_plan_mode_test.go`: explicit flag / env var / `--verify-plan=false` / unset
  flag / config-set `verify:` without storage, plus a regression guard for the explicit flag with
  storage configured (verified red against the old behavior).
