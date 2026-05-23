# `hooks-checkov`

Demonstrates the **`checkov`** hook kind: a `before-terraform-plan` hook
that runs `checkov` against the component and renders a SARIF findings
summary in the terminal.

## What this shows

- `kind: checkov` with **zero configuration** — the kind's defaults supply
  the binary name, args, failure mode, and SARIF result handler.
- Same SARIF parser as `trivy` and `kics` — one body, every consumer.

## Requirements

- `tofu` on PATH.
- `checkov` on PATH (e.g., `atmos toolchain install checkov`).
- **No AWS credentials needed** — checkov parses HCL directly.

## Run

```bash
atmos terraform plan bucket -s test
```

Expected: checkov runs before plan and flags issues on the misconfigured
S3 bucket and security group.
