# Fix: azurerm backend init fails ("only one of subscription and tenant") under Atmos-managed CLI auth

**Date:** 2026-08-11

## Summary

With an Atmos profile active, `atmos terraform` against an `azurerm` backend failed at
`terraform init`:

```
Error: Failed to get existing workspaces: error listing blobs:
AzureCLICredential: ERROR: Please specify only one of subscription and tenant, not both
```

For CLI / device-code / interactive Azure auth, Atmos exported **both**
`ARM_SUBSCRIPTION_ID` and `ARM_TENANT_ID` to the Terraform subprocess. OpenTofu's built-in
`azurerm` backend authenticates through the Azure CLI and runs
`az account get-access-token --subscription <id> --tenant <id>`, and the Azure CLI refuses
that flag combination. The failure is at **argument validation** — it happens before the CLI
even checks whether a session exists — so `terraform init` cannot list existing workspaces or
read state at all. The tenant is now exported only for OIDC auth (which needs it and does not
shell out to the CLI); on the CLI path only the subscription is exported.

## Context

Reproduced end to end during a real Azure engagement (per-subscription `azurerm` backends,
`azure/interactive` auth, `use_azuread_auth: true`). Symptoms and the diagnostic path:

- A direct `tofu init` against the same backend **succeeded** (no Atmos env in play), while
  `ATMOS_PROFILE=<profile> atmos terraform …` **failed** — isolating the cause to the
  environment Atmos exports to the subprocess.
- `az account get-access-token` flag tests: `--subscription <id>` alone works, `--tenant <id>`
  alone works, **both together** error. So the trigger is passing both, which the backend's
  Azure CLI credential does when both `ARM_SUBSCRIPTION_ID` and `ARM_TENANT_ID` are set.
- The `azurerm` / `azapi` / `azuread` **providers** were unaffected — they resolve auth
  without this CLI-flag conflict and auto-detect the tenant from the CLI session. Only the
  OpenTofu built-in `azurerm` **backend** hit it.
- It failed regardless of Azure CLI subscription state (single-sub, multi-sub, or even after
  `az account clear`), confirming the flags come from Atmos-exported env, not CLI state.

The tenant is not needed on the CLI path: the MSAL session Atmos seeds (and the active
subscription) already fixes it, and the providers auto-detect it. OIDC (service-principal /
federated) auth is different — it needs the tenant explicitly and does not invoke the Azure
CLI, so there is no conflict there.

## Changes

- `pkg/auth/cloud/azure/env.go`: `PrepareEnvironment` now exports `AZURE_TENANT_ID` /
  `ARM_TENANT_ID` only when `cfg.UseOIDC` is set. The subscription (`AZURE_SUBSCRIPTION_ID` /
  `ARM_SUBSCRIPTION_ID`) is still exported for both paths, so the backend and providers stay
  scoped to the identity's subscription.
- `pkg/auth/providers/azure/oidc.go`: the OIDC provider re-adds `ARM_TENANT_ID` /
  `AZURE_TENANT_ID` in its post-`PrepareEnvironment` override (OIDC needs the tenant and has no
  Azure CLI shell-out, so no conflict).
- Tests updated to assert the tenant is **omitted** on the CLI / device-code / interactive path
  and **present** for OIDC: `pkg/auth/cloud/azure/env_test.go` (adds a `useOIDC` case and moves
  the tenant vars to `expectedMissing` for CLI cases), `pkg/auth/cloud/azure/setup_test.go`,
  `pkg/auth/providers/azure/cli_test.go`, `pkg/auth/providers/azure/device_code_test.go`.

## Recovery (pre-fix versions)

Run `terraform`/`tofu` with the Atmos profile **off** so Atmos does not export the tenant, and
rely on the Azure CLI session that `atmos auth login` seeds:

```bash
atmos auth login --profile <profile> --identity <sub>   # points the CLI at the subscription
unset ATMOS_PROFILE
atmos terraform apply <component> -s <stack>            # backend uses the CLI session cleanly
```
