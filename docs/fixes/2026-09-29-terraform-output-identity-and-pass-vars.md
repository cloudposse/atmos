# Fix: `terraform output` honors `--identity` with `--format` and works with `init.pass_vars`

**Date:** 2026-09-29

## Summary

Two independent bugs on the `atmos terraform output` / `!terraform.output` path:

- #3232: `atmos terraform output -f json --identity <id>` did not pass the identity's credentials to the `tofu` subprocess, so `tofu` fell back to the host's default AWS credential chain.
- #3231: with `components.terraform.init.pass_vars: true`, every `terraform output` failed with `manual setting of env var "TF_VAR_<name>" detected`.

## Context

**#3232.** `setupTerraformAuth` only sets `info.AuthManager`; it never sets `info.AuthContext`. The main plan/apply path copies `authManager.GetStackInfo().AuthContext` onto `info.AuthContext`, but the `--format` output path did not. `executeOutputWithFormat` therefore passed a nil auth context to the output executor, and `SetupEnvironment` skipped the AWS profile, credentials file and config file variables. The existing `TestExecuteOutputWithFormat_PassesAuthManager` pre-seeded `info.AuthContext`, which hid the gap.

**#3231.** The #1412 fix exports the component's vars as `TF_VAR_*` when `init.pass_vars` is on, then handed the whole environment to `tfexec.Terraform.SetEnv`. terraform-exec rejects any `TF_VAR_*` key there with `ErrManualEnvVar`, and it has no `InitOption` for `-var` or `-var-file`. The existing test used a mock runner whose `SetEnv` accepted anything, so it asserted the broken behavior.

## Changes

- `cmd/terraform/output.go`: new `populateAuthContextFromManager`, called from `prepareOutputContext`. It copies the auth manager's resolved `AuthContext` onto `info.AuthContext`. It does nothing when `info.AuthContext` is already set, the manager is nil, or the manager has no stack info or auth context.
- `pkg/terraform/output/executor.go`: `execute()` gives the runner a copy of the environment without `TF_VAR_*` (`setRunnerEnv`). The full environment still feeds the smart-init fingerprint.
- `pkg/terraform/output/executor_init_vars.go` (new): when `pass_vars` is on and the component has vars, `terraform init` runs directly with the full environment, including `TF_VAR_*`, through a runner wrapper. The init subprocess is overridable with `WithInitWithVars`. Init failures keep the existing `ErrTerraformInit` wrapping and the upgrade/reconfigure recovery. All other commands (`workspace`, `output`) still go through terraform-exec.
- `pkg/terraform/output/executor_runner.go`, `executor_init.go`, `environment.go`: init calls go through `runTerraformInit`; comments updated to say `TF_VAR_*` reaches only the init subprocess.
- Known limitation: a user-defined `TF_VAR_*` in a component `env:` section is now dropped from the non-init runner environment. tfexec used to reject it with an error, and `output` and `workspace` do not consume variables.

## Validation

- Unit tests added or updated first and confirmed failing before the fix: the tfexec-rule mock test failed with `manual setting of env var "TF_VAR_aks_version" detected`, and the positive `populateAuthContextFromManager` case left `info.AuthContext` nil.
- After the fix: `go build ./...` and `go test ./pkg/terraform/... ./cmd/terraform/...` pass.
- End-to-end on a scratch project outside the repo (OpenTofu, local backend, `pass_vars: true`, a component with two vars): a binary built from `origin/main` fails with `manual setting of env var "TF_VAR_stage" detected`. The fixed binary prints `{"greeting": "hello"}`.
- #3232 was not reproduced against real AWS SSO; that needs a live identity. It is covered by the `TestPopulateAuthContextFromManager` unit tests, which call the helper directly. The one-line call inside `prepareOutputContext` has no test of its own, because that function needs a full config to run.
- `golangci-lint` was not run in this session. `gofumpt -l` reports nothing on the changed files.

## Follow-ups

- With `pass_vars: true`, `atmos terraform workspace` passes `-var-file` to `init` without generating the varfile first, so a never-planned component fails with `Failed to read variables file` (reported in #3231). Not fixed here; no tracking issue is opened yet.
