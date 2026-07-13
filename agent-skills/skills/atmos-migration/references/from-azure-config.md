# Migrating from az CLI Config

This reference is the agent's decision guide for users coming from the `az` CLI. For a fuller
prose walkthrough of Azure auth itself (not migration-specific), see the
[Azure Authentication Tutorial](https://atmos.tools/tutorials/azure-authentication); for the full
auth configuration schema, see the [atmos-auth](../../atmos-auth/SKILL.md) skill and its
[providers-and-identities.md](../../atmos-auth/references/providers-and-identities.md) reference.

## Identifying the User's Shape

| User has...                                                          | Maps to                                          |
|------------------------------------------------------------------------|--------------------------------------------------|
| `az login` (interactive user), `az account set --subscription`         | [Interactive User Login](#interactive-user-login--azurecli-or-azuredevice-code) |
| CI service principal with a federated credential (no stored secret)    | [CI Service Principal → azure/oidc](#ci-service-principal-with-federated-credential--azureoidc) |
| `az login --service-principal -u <id> -p <secret>` (client secret, not federated) | [No Direct Equivalent: Client-Secret Service Principal](#no-direct-equivalent-client-secret-service-principal) |
| `az login --identity` (VM/AKS Managed Identity)                        | [No Direct Equivalent: Managed Identity](#no-direct-equivalent-managed-identity) |
| `az cloud set --name AzureUSGovernment` / `AzureChinaCloud`            | [Sovereign Clouds](#sovereign-clouds-govchina) |

## Interactive User Login → `azure/cli` or `azure/device-code`

**Before:**
```bash
az login
az account set --subscription 87654321-4321-4321-4321-210987654321
```

**After (`atmos.yaml`) -- reuse the existing `az login` session:**
```yaml
auth:
  providers:
    azure-cli:
      kind: azure/cli
      spec:
        tenant_id: "12345678-1234-1234-1234-123456789012"
        subscription_id: "87654321-4321-4321-4321-210987654321"
        location: eastus

  identities:
    dev-subscription:
      kind: azure/subscription
      default: true
      via:
        provider: azure-cli
      principal:
        subscription_id: "87654321-4321-4321-4321-210987654321"
        location: eastus
        resource_group: my-rg   # optional
```

**Or, if the user wants Atmos-native login without keeping the az CLI installed**, swap
`kind: azure/cli` for `kind: azure/device-code` (same `spec.tenant_id`/`subscription_id`/
`location` fields, plus optional `spec.client_id` which defaults to Azure CLI's own public client
ID). `azure/device-code` prompts a browser-based device code flow instead of requiring a prior
`az login`.

All Azure provider fields (`tenant_id`, `subscription_id`, `location`, `client_id`,
`cloud_environment`) live under `spec:`, unlike AWS and GCP providers where most fields are
top-level -- this is a common transcription mistake when translating from an AWS-config example.

## CI Service Principal with Federated Credential → `azure/oidc`

**Before:** a GitHub Actions job using `azure/login@v2` (or Azure DevOps) with `client-id`,
`tenant-id`, `subscription-id` and **no client secret** -- the app registration has a federated
credential trusting the CI OIDC issuer.

**After (`atmos.yaml`):**
```yaml
auth:
  providers:
    azure-oidc:
      kind: azure/oidc
      spec:
        tenant_id: "12345678-1234-1234-1234-123456789012"
        client_id: "YOUR_APP_CLIENT_ID"
        subscription_id: "87654321-4321-4321-4321-210987654321"
        location: eastus

  identities:
    ci-subscription:
      kind: azure/subscription
      default: true
      via:
        provider: azure-oidc
      principal:
        subscription_id: "87654321-4321-4321-4321-210987654321"
```

The federated credential itself is a one-time Azure AD app registration change (trusting the CI
issuer's OIDC tokens for a given repo/branch/environment) -- not something `atmos.yaml` configures.

## No Direct Equivalent: Client-Secret Service Principal

There is no `azure/service-principal` (or similarly named) provider kind for
`az login --service-principal -u <appId> -p <client-secret-or-cert> --tenant <tenant>`. Do not
invent YAML for this. If the service principal can be converted to a federated credential
(recommended -- no secret to rotate or leak), migrate it to `azure/oidc` above. If federation is
genuinely not possible for the user's scenario, this is a real gap: tell them so rather than
fabricating a mapping.

## No Direct Equivalent: Managed Identity

There is no `azure/managed-identity` provider kind for `az login --identity` (Azure VM, AKS pod,
Container App Managed Identity). Do not confuse this with `azure/emulator`, which is unrelated --
it binds to a locally running Atmos emulator container, not real Azure infrastructure. Users
running Atmos from a Managed-Identity-enabled Azure compute resource currently have no native
provider kind to migrate to -- flag it as a known gap.

## Sovereign Clouds (Gov/China)

**Before:**
```bash
az cloud set --name AzureUSGovernment
az login
```

**After:** add `spec.cloud_environment` to any of the three provider kinds above:
```yaml
auth:
  providers:
    azure-gov:
      kind: azure/device-code
      spec:
        tenant_id: "12345678-1234-1234-1234-123456789012"
        cloud_environment: usgovernment   # or: china
```

`cloud_environment` accepts `public` (default), `usgovernment` (Azure Government / GCC High), or
`china` (Azure China / Mooncake). Atmos switches login endpoints, API scopes, and blob storage
URLs automatically -- it is not inherited from the az CLI's currently active cloud, so it must be
set explicitly even if the user's az CLI is already pointed at the sovereign cloud.

## Common Gotchas

- **`azure/subscription` is never standalone** -- it always needs `via.provider` pointing at one
  of `azure/cli`, `azure/device-code`, or `azure/oidc`.
- **All provider-specific fields live under `spec:`**, not top-level -- easy to get wrong if the
  user is also migrating AWS or GCP profiles in the same session, where most fields are top-level.
- **No client-secret service principal or Managed Identity kind exists yet.** Don't guess at
  field names for these -- confirm the gap and suggest the federated-credential (`azure/oidc`)
  alternative where possible.
- **`cloud_environment` must be set explicitly per provider** -- it doesn't inherit from the az
  CLI's active cloud setting.

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference.
