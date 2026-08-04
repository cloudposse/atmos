# Fix: `atmos auth login` corrupts the Azure CLI cache for guest (B2B) users

**Date:** 2026-08-03

## Summary

After `atmos auth login` with any Azure provider, the Azure CLI cache write-back created an
MSAL Account entry with `home_account_id` derived as `{oid}.{target-tenant}` and a hardcoded
`account_source: "device_code"`. For guest (B2B) users the home tenant differs from the tenant
being accessed, so `~/.azure/msal_token_cache.json` ended up with two Account entries for the
same username, and every subsequent az command failed with
`Found multiple accounts with the same username`
([azure-cli#20168](https://github.com/Azure/azure-cli/issues/20168)) — including
`az account get-access-token`, which the `azure/cli` provider itself shells out to. One
`atmos auth login` therefore broke both az and the next atmos login for any guest user.
Fixed in [#2861](https://github.com/cloudposse/atmos/pull/2861).

## Context

Reproduced end to end against a real tenant where the operator is a B2B guest
(`user@home-tenant.com` signing in to a different tenant): `az login` →
`atmos auth login` (azure/cli provider) → az broken. Two separate write paths produced the bad
entry: `UpdateAzureCLIFiles` (called from the `azure/subscription` identity's PostAuthenticate)
and the device-code provider's own `updateAzureCLICache`. Both derived the MSAL
`home_account_id` from the *target* tenant, while az records the *home* tenant
(`{home-oid}.{home-tenant}`), producing a same-username duplicate that MSAL refuses to
disambiguate.

Two contributing design gaps:

- Credentials minted **by** the Azure CLI were written **back** into the CLI's own cache —
  pure corruption risk with zero benefit, since az's cache is already authoritative for that
  session. (The write-back is load-bearing only for MSAL-based flows like device code, where
  it's what lets Terraform's `azurerm`/`azuread` providers authenticate via `ARM_USE_CLI`.)
- The `azure/subscription` identity rebuilt `AzureCredentials` field-by-field when wrapping
  provider credentials, silently dropping any newly added field. This initially defeated the
  fix (the new `AuthMethod`/`HomeAccountID` fields never reached the cache writer) and was only
  caught by the end-to-end guest-tenant test.

## Changes

- `pkg/auth/types/azure_credentials.go`: `AzureCredentials` gains `AuthMethod` (which provider
  kind minted the credentials: `cli` / `device_code` / `oidc`) and `HomeAccountID` (MSAL's
  `{home-oid}.{home-tenant-id}`).
- `pkg/auth/cloud/azure/setup.go`: `UpdateAzureCLIFiles` skips the write-back entirely for
  CLI-sourced credentials; `addUserAccountAndTokens` prefers the MSAL home account ID over the
  `{oid}.{target-tenant}` derivation.
- `pkg/auth/providers/azure/device_code.go` / `device_code_cache.go`: the device-code provider
  captures `AuthResult.Account.HomeAccountID` in both silent and interactive flows
  (`captureHomeAccountID`) and threads it through its own cache writer.
- `pkg/auth/identities/azure/subscription.go`: the identity wrap is now a struct copy with
  explicit overrides; a reflection-based regression test fails if any future field is dropped.
- Coverage: `azure/cli` Authenticate is tested end to end via a stubbed `az` on `PATH`;
  the device-code silent/headless paths and the identity PostAuthenticate happy path are
  covered with a sandboxed `HOME`.

## Recovery

A corrupted cache is repaired with `az account clear && az login --tenant <tenant-id>`.
