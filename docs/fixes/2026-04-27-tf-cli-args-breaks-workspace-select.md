# Fix: `TF_CLI_ARGS` env var breaks `atmos terraform plan/apply` via internal setup subcommands

**Date:** 2026-04-27

## Issue

A user setting `TF_CLI_ARGS: "-lock-timeout=10m"` at the GitHub
Actions workflow level (so OpenTofu would wait on remote state
locks instead of failing immediately) hit a hard failure on every
`atmos terraform plan` run:

```text
Error parsing command-line flags: flag provided but not defined: -lock-timeout
```

The error is emitted by `tofu`, not Atmos, and aborts the run
before `tofu plan` ever executes. The user worked around it by
switching to `TF_CLI_ARGS_plan` / `TF_CLI_ARGS_apply`, but the
unscoped form is what most CI configurations reach for first and
the failure message points at a flag the user set deliberately on
a Terraform subcommand the user did not know Atmos was running.

## Root cause

`atmos terraform plan` is not a single Terraform invocation. Atmos
runs three classes of `tofu` subprocesses for one `atmos terraform
plan`:

1. **Setup** the user did not write — `tofu workspace select`
   (`runWorkspaceSetup` at
   `internal/exec/terraform_execute_helpers_exec.go:181`), the
   `tofu workspace new` fallback (`createWorkspaceFallback`,
   line 226), and the auto-`tofu init` pre-step
   (`executeTerraformInitPhase` at
   `internal/exec/terraform_execute_helpers.go:528`, only fires
   when SubCommand ≠ "init" and `--skip-init` is not set).
2. **The user's primary subcommand** —
   `executeMainTerraformCommand` (e.g. `tofu plan`).
3. **Data fetches** for `!terraform.output` /
   `atmos.Component(...)` template calls — handled separately
   in `pkg/terraform/output/`.

All three classes go through `ExecuteShellCommand` at
`internal/exec/shell_utils.go:96`, which builds the subprocess
environment from `os.Environ()`. That inherits whatever
`TF_CLI_ARGS` the parent process exported. OpenTofu's documented
behavior is to **prepend `TF_CLI_ARGS` to every subcommand's
argv**, so `tofu workspace select` sees
`tofu -lock-timeout=10m workspace select <ws>` and rejects the
unknown flag.

The user's reported flag (`-lock-timeout`) is one of the few that
*also* happens to be valid for `tofu init`, so init didn't crash
in their case — but `-parallelism`, `-refresh=false`,
`-detailed-exitcode`, `-target`, `-replace`, `-out` (all common in
plan/apply contexts) all crash `tofu init` for exactly the same
reason. The bug class covers every Atmos-internal setup
subprocess, not just `workspace select`.

The third class (data fetches) was already correct:
`pkg/terraform/output/environment.go:23-42` strips `TF_CLI_ARGS`
and `TF_CLI_ARGS_*` from the env before invoking terraform-exec.
The regular plan/apply flow simply did not apply the same hygiene
to its own setup subprocesses.

## How

When Atmos invokes a terraform/tofu subprocess **the user did not
write**, strip the env vars that OpenTofu would inject as flags
into a subcommand that does not accept them.

Four files:

1. **`internal/exec/terraform_env_sanitize.go`** *(new)* — defines
   `terraformWorkspaceEnvBlocklist` (`TF_CLI_ARGS` and
   `TF_CLI_ARGS_workspace`) and the pure helper
   `sanitizeTerraformWorkspaceEnv(env []string) []string` that
   returns a copy of `env` with blocked entries removed.
   Per-subcommand variants (`TF_CLI_ARGS_plan`,
   `TF_CLI_ARGS_apply`, `TF_CLI_ARGS_init`, …) are intentionally
   preserved — OpenTofu only applies them to their named
   subcommand by design, so they cannot affect setup subprocesses.

2. **`internal/exec/shell_utils.go`** — adds
   `WithSanitizedTerraformSetupEnv()` `ShellCommandOption` and a
   `sanitizeTerraformSetupEnv` flag on `shellCommandConfig`. When
   set, `ExecuteShellCommand` filters the **fully merged** subprocess
   env (base process env + `atmosConfig.Env` + per-command `env`
   slice) through `sanitizeTerraformWorkspaceEnv` *after* the merge,
   so blocked vars cannot be reintroduced by `atmosConfig.Env`
   (atmos.yaml top-level `env:`) or by the per-command env slice
   (`info.ComponentEnvList` → stack `env:` + auth hooks). The filter
   runs *before* the `ATMOS_SHLVL` append so atmos's own bookkeeping
   isn't stripped, and it composes cleanly with auth-sanitized envs
   passed via `WithEnvironment` because `processEnv` is applied
   before the merge.

3. **`internal/exec/terraform_execute_helpers_exec.go`** —
   `runWorkspaceSetup` and `createWorkspaceFallback` each append
   `WithSanitizedTerraformSetupEnv()` to their per-call options
   slice before calling `ExecuteShellCommand`.

4. **`internal/exec/terraform_execute_helpers.go`** —
   `executeTerraformInitPhase` does the same. Its existing mutual-
   exclusion contract guarantees this function only runs as the
   auto-init pre-step (never when the user invokes `atmos
   terraform init` directly), so the sanitization cannot affect a
   user-invoked init — that path goes through
   `executeMainTerraformCommand` and is unchanged.

## Tests

- **`internal/exec/terraform_env_sanitize_test.go`** *(new)* —
  table-driven tests for `sanitizeTerraformWorkspaceEnv`: drops
  `TF_CLI_ARGS` and `TF_CLI_ARGS_workspace` (including empty
  values); preserves `TF_CLI_ARGS_plan`, `_apply`, `_init`,
  `TF_VAR_*`, `TF_LOG`, malformed entries with no `=`, values
  containing `=`, and exact-prefix-match edge cases; does not
  mutate the caller's slice.

- **`internal/exec/terraform_workspace_env_sanitize_test.go`**
  *(new)* — three integration tests using the test-binary-as-
  subprocess pattern. A new `_ATMOS_TEST_ENV_DUMP_FILE` sentinel
  in `internal/exec/testmain_test.go` makes the test binary write
  its full environment to a file and exit 0, so each test can
  inspect what the subprocess actually saw:
  - `TestRunWorkspaceSetup_StripsTfCliArgs` (`workspace select`).
  - `TestCreateWorkspaceFallback_StripsTfCliArgs` (`workspace
    new`).
  - `TestExecuteTerraformInitPhase_StripsTfCliArgs` (auto-init);
    uses `TF_CLI_ARGS=-parallelism=4` to guard against the
    user's specific `-lock-timeout` value being one of the few
    that also passes `init`'s flag validation.

  Each test was verified to fail when its corresponding code
  delta is reverted, then pass with the fix in place.

## Status

**Fixed.** Branch `aknysh/update-terraform-args`. All 18 sub-tests
across the four new test functions pass; the full `internal/exec`
package re-runs cleanly (115 s, exit 0).

The user's workaround (switching to `TF_CLI_ARGS_plan` /
`TF_CLI_ARGS_apply`) continues to work; the fix removes the need
for it for the common `-lock-timeout` / `-parallelism` cases.

Sites intentionally **not** sanitized:

- `executeMainTerraformCommand` — the user's primary subcommand.
- `handleVersionSubcommand` — `atmos terraform version` is
  user-invoked.
- `isTerraformCurrentWorkspace` — file read only, no subprocess.
- `pkg/terraform/output/` — already strips `TF_CLI_ARGS` /
  `TF_CLI_ARGS_*` via its own prohibited-vars list.

Open items (separate PRs):

- Update the `warnOnConflictingEnvVars` log message at
  `internal/exec/terraform_execute_helpers.go:341` to acknowledge
  the new setup-step hygiene.
- Add a `TF_CLI_ARGS` vs `TF_CLI_ARGS_<subcommand>` paragraph to
  `website/docs/cli/configuration/terraform.mdx`.
- Blog post under `website/blog/` (PR is `bugfix`-labeled, so the
  CI gate requires one).
