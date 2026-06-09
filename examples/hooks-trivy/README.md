# `hooks-trivy`

Demonstrates the **`trivy`** hook kind: a `before-terraform-plan` hook that
runs `trivy config` against the component and renders a SARIF findings
summary in the terminal.

## What this shows

- `kind: trivy` with **zero configuration** — the kind's defaults supply
  the binary name, args (`config --format sarif --output $ATMOS_OUTPUT_FILE
  $ATMOS_COMPONENT_PATH`), failure mode (`warn`), and SARIF result handler.
- Static-analysis scan runs **before plan**, so security issues surface
  before any infrastructure action.
- Single markdown rendering used in the terminal and on the Pro run page.
  The same `pkg/hooks/sarif` parser also serves `checkov` and `kics`.

## Requirements

- `tofu` (OpenTofu) on PATH.
- `trivy` on PATH (e.g., `brew install trivy`).
- **No real cloud credentials needed** — trivy parses HCL directly.

## Run

```bash
atmos terraform plan bucket -s test
```

Expected: trivy runs first; flags a handful of issues (S3 ACL,
unencrypted bucket, wildcard security group ingress); plan proceeds and
shows the dummy resources.

## Files

- `components/terraform/bucket/main.tf` — intentionally insecure HCL so
  trivy has something to find.
