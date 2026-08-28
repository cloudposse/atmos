# Fix: Seeded Azure CLI cache misses the legacy ARM audience and carries no refresh token

**Date:** 2026-08-06

## Summary

After `atmos auth login` (device-code or interactive), the Azure CLI cache write-back
seeded access tokens only under the modern ARM scope
(`https://management.azure.com/.default`) and seeded no refresh token. Two field
failures followed: `azapi`-based Terraform modules (all modern Azure Verified Modules)
died mid-apply with `AzureCLICredential: ERROR: Can't find token from MSAL cache`,
because azidentity/az request the **legacy** ARM audience
(`https://management.core.windows.net/`) by default; and after ~1 hour every az-side
lookup failed the same way, because az had no refresh token to self-mint replacements.
The management token entry now carries the legacy audience scope forms in its MSAL
`target`, and the refresh token MSAL persists in the Atmos realm cache is copied into
the az cache.

## Context

Reproduced end to end during a real engagement: `azurerm` provider resources applied
fine (its az shell-out requests the modern scope) while the `azapi` resource in the
same apply failed — `az account get-access-token` (default → legacy resource) missed
the cache while `--scope https://management.azure.com/.default` hit it. The failure is
cache-*lookup*, not token validity: ARM accepts both audience forms interchangeably,
and MSAL matches requested scopes as a subset of an entry's space-separated `target`.
So one seeded management token entry listing all ARM scope forms
(`management.azure.com/.default`, plus the legacy single- and double-slash forms az
derives from the trailing-slash resource) satisfies every lookup.

The refresh-token gap is fixable because Atmos authenticates with the Azure CLI public
client: the refresh token MSAL persists in `~/.azure/atmos/<realm>/msal_token_cache.json`
is exactly the credential az's own login would have stored.

## Changes

- `pkg/auth/cloud/azure/cloud_environments.go`: `CloudEnvironment` gains
  `LegacyManagementScopes` (public/usgovernment/china variants, single- and
  double-slash forms).
- `pkg/auth/providers/azure/device_code_cache.go` and
  `pkg/auth/cloud/azure/setup.go`: the seeded management token's `target` now joins
  the modern scope with the legacy forms.
- `pkg/auth/cloud/azure/refresh_token.go` (new): `CopyAtmosRefreshTokensInto` copies
  the account's refresh-token entries from the Atmos realm cache into the az cache
  (skipped for service principals, empty realms, or unknown home account IDs).
  Both cache writers invoke it; `UpdateAzureCLIFiles` gains a `realm` parameter.
- Regression tests (`token_audience_test.go`) reproduce both field failures and pin
  the fix; the sovereign-cloud test now asserts the multi-audience `target`.

## Recovery (pre-fix versions)

`az login --tenant <tenant-id>` gives az its own refresh token; keep such a session
alongside `atmos auth login` when applying azapi/AVM-based components.
