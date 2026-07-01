# `hooks-tflint`

Demonstrates the **`tflint`** hook kind: an `after.terraform.plan` hook that
runs [tflint](https://github.com/terraform-linters/tflint) against the
component and renders a SARIF findings summary in the terminal.

## What this shows

- `kind: tflint` with **zero configuration** — the kind's defaults supply
  the binary name, args (`--chdir=$ATMOS_COMPONENT_PATH --format=sarif`),
  failure mode (`warn`), and the shared SARIF result handler.
- tflint has no file-output flag — `--format=sarif` writes to **stdout** — so
  the kind opts into the engine's `CaptureStdout` behavior, which redirects
  stdout into `$ATMOS_OUTPUT_FILE`. From there it's identical to
  `trivy`/`checkov`/`kics`.
- Single markdown rendering used in the terminal and on the Pro run page (the
  same `pkg/hooks/sarif` parser serves every scanner kind). In CI, the same
  findings also become a `$GITHUB_STEP_SUMMARY` block, inline PR annotations,
  and a GitHub Code Scanning upload — automatically, gated by your `ci.*`
  config.

## Requirements

- `tofu` (OpenTofu) on PATH.
- `tflint` on PATH (e.g., `brew install tflint`). The builtin `terraform`
  ruleset needs no `tflint --init`; provider rulesets (aws/google/azurerm) do.
- **No real cloud credentials needed** — tflint parses HCL directly.

## Run

```bash
atmos terraform plan bucket -s test
```

Expected: plan runs, then tflint runs and flags the intentionally-unused
`unused` variable (`terraform_unused_declarations`); the plan proceeds
(`on_failure: warn`).

## Files

- `components/terraform/bucket/variables.tf` — declares an unused variable so
  tflint has a deterministic finding.
