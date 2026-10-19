# Fix: Policies govern Starlark scripts and their nested Atmos commands

**Date:** 2026-10-16

## Summary

A field test checked whether policies govern Starlark scripts the same way they govern the CLI.
Script calls such as `atmos.terraform(...)`, `steps.atmos`, standalone `.star` scripts, and
`components.get` reach the same policy gates as the CLI. The test also found discovery, rule-matching,
hook-ordering, and error-reporting defects. This fix corrects them.

## Context

The field test used the library `command-deny-destroy` and `function-disable-exec` policies, a
`component.command` rule for production, and a mock Terraform component. It found these defects:

- Default `policy.d` discovery used only the working directory. `atmos.yaml` discovery falls back to the
  `--config`/`--config-path` flags, `ATMOS_CLI_CONFIG_PATH`, the git root, or a parent directory. A command
  run from a repository subdirectory, or a script call with `working_directory=`, loaded the
  configuration but no policies.
- `command-deny-destroy` matched only `terraform destroy`. The `apply`, `plan`, and `deploy` commands with
  `-destroy` (in any spelling, also after `--`) did not match.
- A `component.command` denial ran `before.*` hooks, and then `after.*` hooks with `status=failure`,
  before the denial was reported. The `--all` path labelled the denial "per-component hook failed" and
  lost the reason.
- A component hook script used the instance name (`components/terraform/mycomponent`) as its default
  working directory, not the root module (`metadata.component`). Hook failures logged the raw error chain.
- A deny rule without a `reason` printed only "policy denied execution", with no rule ID.
- Non-TTY output (CI logs, captured stderr) changed `` **`!exec` is disabled** `` into
  `!exec** is disabled**`.
- Script DX: a denied nested command was printed twice. `steps.atmos` reported only `exit status 1`. A
  script could not tell a denial from a failure. `--explain` did not reach nested commands.

## Changes

- **Discovery:** a new `pkg/config/projectroot` package holds the project-root logic. Configuration
  loading and policy discovery both use it, so `policy.d` uses the same precedence as `atmos.yaml`: config
  flags, then environment, then working directory, then git root or ancestor. `ATMOS_POLICY_DIR` stays
  the explicit override.
- **Destroy detection:** the `command` and `component.command` inputs have a normalized `destroy`
  boolean (`IsDestroyInvocation` in `pkg/policy/input_schema.go`). It is set for `terraform destroy`, `helmfile destroy`, and any
  Terraform subcommand with a true `-destroy`/`--destroy` flag, also after `--`. False values stay
  false. The library rule is now `input.command == 'terraform destroy' || input.destroy`, in both
  the CEL and the Starlark variants.
- **Denials:** each denial names every denying rule ("Denied by `<rule-id>`." when the rule has no
  reason). Each denial exits with the new `ExitCodePolicyDenied` (77, `EX_NOPERM`).
- **Rendering:** without color, explanations and hints use a readable ASCII style that has no dangling
  emphasis markers (`RenderReadable`).
- **Hooks:** component admission errors stop the terraform pre-run and the multi-component scheduler
  before any hook runs. The scheduler keeps the denial explanation. A hook working directory comes from
  the root module. Hook failure logs show the message and hints.
- **Scripts:** a streamed nested command does not repeat its stderr tail. The `steps.atmos` and
  `atmos.*` errors name the command and exit code, and identify policy denials (exit 77 is propagated).
  `atmos.*` results have `policy_denied`. `--explain` exports `ATMOS_EXPLAIN=true` to child processes.
- **Docs:** the policy configuration page (discovery precedence and a runtime-trust admonition), the
  admission and overview pages (`destroy`, exit code 77), the `atmos policy` command page
  (`ATMOS_EXPLAIN`), and the `atmos.run`, `atmos.terraform`, and `steps.atmos` reference pages.

## Validation

- `go build ./...` passed.
- `go test` passed for `pkg/policy/...`, `pkg/config/...`, `pkg/flags/...`, `pkg/hooks/...`,
  `pkg/script/...`, `pkg/scheduler/...`, `pkg/ui/markdown/...`, `errors/...`, `cmd/internal/...`,
  `cmd/terraform/...`, and `internal/exec/`.
- The new regression tests failed before the fixes and pass after them (discovery, destroy detection,
  hook ordering, hook directory, rule IDs, exit code 77, non-TTY rendering, script failure reporting).
- `go test ./tests -run TestCLICommands -short` passed with no snapshot changes. This run happened before
  the final test-file consolidation and the documentation edits.
- `atmos policy test` passed 6 of 6 cases for the CEL and Starlark variants of `command-deny-destroy`.
- `./custom-gcl run --new-from-rev=origin/osterman/function-policy-security-audit` reported 0 issues on
  the changed packages.
- A live matrix with the built binary and a git-initialized fixture: CLI and script destroy variants were
  denied with exit 77 and the rule ID. `-destroy=false` and `plan` were allowed. Destroy from a
  subdirectory was denied. A prod `apply` (single and `--all`) was denied with no hook output.
  `policy_denied` was `True` in capture mode. The child `--explain` lines appeared.
- `cd website && npm run build` passed. It reported no broken links on the changed pages.
- Five `pkg/runner/step` cast-session tests (`TestCastHandler*`, `TestRunCastStep*`) time out in this
  environment. They also fail on an unmodified checkout of the branch head, so these changes did not
  cause them.

## Follow-ups

- The `**Error:**` header and title still use the original ASCII style. A message title that contains
  inline code inside bold text can still render markers in non-TTY output.
- `helm delete` and `kubernetes delete` do not set `destroy`. Rules can match them by `command`.
