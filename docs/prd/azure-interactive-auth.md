# Azure Interactive Browser Authentication (`azure/interactive`)

**Status**: Implemented
**Last Updated**: 2026-08-04
**Owners**: Atmos auth subsystem

**Upstream references** (verified via Microsoft docs):
- MSAL — [Interactive and non-interactive authentication flows](https://learn.microsoft.com/en-us/entra/identity-platform/msal-authentication-flows)
- MSAL Go — `public.Client.AcquireTokenInteractive`: [pkg.go.dev/github.com/AzureAD/microsoft-authentication-library-for-go/apps/public](https://pkg.go.dev/github.com/AzureAD/microsoft-authentication-library-for-go/apps/public)
- Microsoft Entra — [Microsoft-managed Conditional Access policies](https://learn.microsoft.com/en-us/entra/identity/conditional-access/managed-policies) (device code flow blocking)
- Azure CLI — [Sign in with Azure CLI](https://learn.microsoft.com/en-us/cli/azure/authenticate-azure-cli) (the flow this provider mirrors)

**Related Atmos PRDs**:
- [PRD-Atmos-Auth](../../pkg/auth/docs/PRD/PRD-Atmos-Auth.md) (umbrella)
- Fix doc: [Azure CLI cache corruption for guest users](../fixes/2026-08-03-azure-cli-cache-corruption-guest-users.md) (#2861 — provides the `AuthMethod`/`HomeAccountID` plumbing this feature builds on)

---

## 1. Executive Summary

### Problem

Atmos had no one-command human login for Azure that works under modern tenant policy:

- `azure/device-code` performs a self-contained login, but Microsoft-managed Conditional
  Access policies now **block the device code flow** in many tenants (error `AADSTS530035`),
  because device code is a phishing vector. Microsoft is rolling these managed policies out
  broadly, so this failure mode grows over time rather than shrinking.
- `azure/cli` delegates to an existing `az login` session — it can never be one command, and
  it requires the Azure CLI to be installed and logged in first.
- `azure/oidc` is CI-only (workload identity federation).

On AWS, `atmos auth login` is a single command (`aws/iam-identity-center`). Azure users had
no equivalent.

### Solution

A new provider kind **`azure/interactive`** implementing MSAL interactive browser
authentication — authorization code + PKCE on a localhost redirect, the exact flow
`az login` uses (and MSAL's own name for it: `AcquireTokenInteractive`). Conditional Access
allows it because the browser session carries full CA context (MFA, device state, sign-in
risk).

`atmos auth login` opens the browser, the user signs in, and atmos acquires Management,
Graph, and Key Vault tokens, persists them to the realm-scoped MSAL cache (refresh tokens
make repeat logins silent), and writes the Azure CLI-compatible cache files — so Terraform's
`azurerm`/`azuread` providers authenticate via `ARM_USE_CLI`, and the `az` CLI itself works
without ever running `az login`.

## 2. Design

### Provider kind

`azure/interactive` — named for the mechanism (MSAL "interactive" flow), consistent with the
repo convention that kinds name auth mechanisms, not UX (`aws/iam-identity-center`,
`gcp/workload-identity-federation`). The spec shape is identical to `azure/device-code`:
`tenant_id` (required), `subscription_id`, `location`, `client_id` (defaults to the Azure
CLI public client `04b07795-8ddb-461a-bbee-02f9e1bf7b46`, which pre-authorizes localhost
redirects), `cloud_environment` (public | usgovernment | china).

### Implementation shape

`interactiveProvider` embeds `deviceCodeProvider` and reuses its MSAL client construction,
silent token acquisition, Graph/Key Vault token fan-out, and Azure CLI cache write-back.
Only the acquisition step differs (`AcquireTokenInteractive` vs. the device code flow). The
shared machinery is parameterized by auth method: credentials persist
`auth_method: interactive`, and the MSAL cache `account_source` mirrors az's own labels
(`authorization_code` for the browser flow, `device_code` otherwise).

Authentication order:
1. Silent acquisition from the persisted MSAL cache (refresh tokens survive restarts — no
    browser on repeat logins within the refresh window).
2. Interactive browser flow, guarded by a TTY check (headless environments get an error
    directing CI/CD to `azure/oidc` and browser-less human sessions to `azure/device-code`
    where tenant policy allows it).

Guest (B2B) users are handled correctly: the MSAL `AuthResult` supplies the real
`{home-oid}.{home-tenant}` home account ID, which flows into both cache writers (see the
related fix doc).

### Testability

The MSAL interactive acquisition requires a live identity provider and a browser, so the
provider exposes two injection seams (`acquireInteractive`, `checkInteractive`) per the
repo's dependency-injection convention. Tests cover the full success path, acquisition
failure, and headless refusal against a sandboxed `HOME`.

## 3. Non-goals

- Replacing `azure/device-code` (still valid where a browser cannot run and the tenant
  allows the flow) or `azure/cli` (still valid to piggyback on an existing az session).
- Embedded/webview sign-in, brokered auth (WAM), or Entra device registration.
- Tenants requiring a custom app registration: supported via `spec.client_id`, but
  provisioning that registration is out of scope.

## 4. Acceptance

- `atmos auth login` with an `azure/interactive` provider opens the default browser,
  completes SSO (including MFA under Conditional Access), and mints ARM/Graph/Key Vault
  tokens — verified end to end in a tenant where the device code flow is blocked by a
  Microsoft-managed policy and the operator is a B2B guest.
- Repeat logins are silent via the MSAL refresh token.
- After login, `az account show` works without `az login`, and the az MSAL cache contains a
  single, correctly-keyed Account entry.
- Headless environments fail fast with guidance toward `azure/oidc` (CI/CD) or
  `azure/device-code` (browser-less human sessions).

### Verification (manual, 2026-08-04)

Executed end to end in a real Entra tenant where the device code flow is blocked by a
Microsoft-managed Conditional Access policy (`AADSTS530035`), with an operator who is a
guest (B2B) user in that tenant. Starting from a fully clean state
(`az account clear` plus removal of `~/.azure/atmos`, `~/.azure/msal_token_cache.json`, and
`~/.azure/azureProfile.json`):

1. `atmos auth login` — opened the default browser, SSO completed, tokens minted (~1.5h
    expiry). One command, no device code, no `az login`.
2. `atmos auth login` again — succeeded **silently** (same token expiry, no browser),
    confirming refresh-token persistence in the realm-scoped MSAL cache.
3. `atmos auth whoami` — reported provider, identity, subscription principal, tenant, and
    expiry.
4. `az account show` — worked without ever running `az login`, confirming the drop-in
    write-back.
5. Cache forensics: exactly one MSAL Account entry, `account_source: authorization_code`
    (matching az's own label for this flow), with a home account ID whose tenant differs
    from the target tenant (the guest case that previously produced the duplicate-account
    corruption); persisted credentials carry `auth_method: interactive` and the home account
    ID.

Known cosmetic limitation: the azureProfile written by the write-back records the
subscription ID as the subscription's display name (atmos does not query ARM for the
display name during login), so `az account show` shows the ID in the `name` field until the
user runs `az login` themselves. Functionality is unaffected.
