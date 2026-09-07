# `hooks-tflint`

Demonstrates the **`tflint`** hook kind: a `before.terraform.init` hook that
lints a component with [tflint](https://github.com/terraform-linters/tflint)
and renders a SARIF findings summary in the terminal.

## Why lint

`terraform validate` only proves your config parses and is internally
consistent. A linter goes further — it enforces **team conventions** and
**code consistency**, and catches **hygiene** problems that otherwise rot a
codebase: unused variables and outputs, deprecated syntax, invalid instance
types, and missing version constraints. Running it as a hook makes those
standards automatic on every run, locally and in CI, instead of relying on
reviewers to catch them.

## What this shows

- `kind: tflint` with **zero configuration** — the kind's defaults supply the
  binary name, args (`--chdir=$ATMOS_COMPONENT_PATH --format=sarif`), failure
  mode (`warn`), and the shared SARIF result handler.
- Linting on `before.terraform.init` — a linter should **fail fast**, before
  any init/plan work. tflint reads static HCL, so it needs no prior init or
  plan (and no cloud credentials).
- CI jobs that only run `atmos terraform plan`, `apply`, or `deploy` should use
  the matching before event (for example `before.terraform.plan`) so the tflint
  section appears in that job's summary.
- The component is a provider-free `terraform_data` resource, so it lints and
  plans fully offline. The intentionally-unused `unused` variable gives tflint
  a deterministic finding (`terraform_unused_declarations`).

## Requirements

- `tofu` (OpenTofu) on PATH.
- `tflint` on PATH (e.g., `brew install tflint`). The builtin `terraform`
  ruleset needs no `tflint --init`; provider rulesets (aws/google/azurerm) do.
- **No cloud credentials** — nothing here touches a real provider.

## Run

```bash
atmos terraform plan example -s test
```

Expected: tflint runs first (before init) and flags the unused `unused`
variable; the command then proceeds (`on_failure: warn`).

## Files

- `components/terraform/example/variables.tf` — declares an unused variable so
  tflint has a deterministic finding.
