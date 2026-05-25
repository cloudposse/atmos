# `hooks-infracost`

Demonstrates the **`infracost`** hook kind: an `after-terraform-plan` hook
that runs `infracost breakdown` against the component and renders a cost
summary in the terminal.

## What this shows

- `kind: infracost` with **zero configuration** — the kind's defaults supply
  the binary name, args, output format, failure mode, and result handler.
- Single markdown rendering used everywhere: when Atmos Pro is connected
  the same body is uploaded; in the terminal it renders via `ui.Markdown()`.

## Requirements

- `tofu` (OpenTofu) on PATH — used in place of HashiCorp Terraform.
- `infracost` on PATH (e.g., `atmos toolchain install infracost`).
- `INFRACOST_API_KEY` env var (free at <https://www.infracost.io/>) —
  infracost needs an API key to access its cloud-hosted pricing database.
  The hook config below forwards it via `env:`; OS environment variables
  also propagate automatically.
- **No AWS credentials needed** — the component uses dummy AWS provider
  config so `tofu plan` succeeds offline.

## Run

```bash
atmos terraform plan nat-gateway -s test
```

Expected: terraform plan succeeds; the `after-terraform-plan` hook fires;
infracost runs and prints a markdown cost summary showing the NAT gateway
and EIP monthly costs (typically ~$32/mo + ~$3.65/mo).

## Files

- `atmos.yaml` — Atmos config; uses `command: tofu` so OpenTofu acts as the
  terraform binary.
- `stacks/deploy/test.yaml` — stack with a single component and one hook.
- `components/terraform/nat-gateway/` — minimal NAT gateway + EIP module.

## Notes

`subnet_id` is a placeholder ID. We don't need a real subnet because we
never apply — infracost prices resources from HCL/plan, not from live state.
