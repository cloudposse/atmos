# Azure ACR Integration (`azure/acr`)

Atmos logs Docker clients into Azure Container Registry via the `azure/acr` integration and the
`atmos azure acr login` command.

## Command

`atmos azure acr login` supports three modes:

```shell
# Named integration from auth.integrations
atmos azure acr login dev/acr

# All azure/acr integrations linked to an identity
atmos azure acr login --identity azure-dev

# Explicit registry login server using ambient Azure credentials
atmos azure acr login --registry myregistry.azurecr.io

# Sovereign clouds: the login server suffix selects the cloud
atmos azure acr login --registry myregistry.azurecr.us
```

Named-integration and identity modes use Atmos Auth. Explicit `--registry` mode uses ambient Azure
credentials (the Azure SDK default credential chain: environment variables, managed identity,
workload identity, Azure CLI). Every `--registry` value is validated before any login starts, and
the ambient token is requested from the cloud that matches the registry's suffix.

## Configuration

Configure ACR integrations under `auth.integrations` with `kind: azure/acr`. Providers and
identities are the standard Azure Auth building blocks (see the main skill and
[providers-and-identities.md](providers-and-identities.md)):

```yaml
auth:
  providers:
    azure-device-code:
      kind: azure/device-code
      spec:
        tenant_id: 00000000-0000-0000-0000-000000000000

  identities:
    azure-dev:
      kind: azure/subscription
      via:
        provider: azure-device-code
      principal:
        subscription_id: 11111111-1111-1111-1111-111111111111

  integrations:
    dev/acr:
      kind: azure/acr
      via:
        identity: azure-dev
      spec:
        auto_provision: true
        registry:
          name: myregistry
```

`spec.registry` is the same struct used by `aws/ecr` integrations (`account_id`, `region` for AWS;
`name`, `tenant_id` for Azure) — only the fields relevant to the integration's `kind` matter.
The login server is `{name}` plus the suffix of the identity's provider cloud
(`spec.cloud_environment` on the provider):

| `cloud_environment` | Login server |
|---------------------|--------------|
| `public` (default, or unset/unknown) | `{name}.azurecr.io` |
| `usgovernment` | `{name}.azurecr.us` |
| `china` | `{name}.azurecr.cn` |

A registry only accepts an Azure AD token from its own cloud, so the provider's
`cloud_environment` must match the registry. Logout cannot know which cloud was used, so it removes
the Docker login for `{name}.azurecr.io`, `{name}.azurecr.us` and `{name}.azurecr.cn`.

### Registry in another tenant (`spec.registry.tenant_id`)

A registry only accepts an Azure AD token issued by its own Entra ID tenant; the exchange sends
the token and the tenant that issued it. `spec.registry.tenant_id` (a tenant GUID or a domain such
as `partner.onmicrosoft.com`, validated when the integration loads) names the registry's tenant
when it differs from the identity's:

- Same tenant as the identity: the identity's credentials are used unchanged.
- Another tenant: Atmos mints an ARM token in that tenant silently from the realm's MSAL cache
  (the provider's cached refresh token, same client), at the provider cloud's login endpoint.
  This needs an `azure/device-code` or `azure/interactive` provider and an account that is a
  member or guest of the registry's tenant.
- `azure/cli` and `azure/oidc` credentials fail with `ErrACRTenantOverrideUnsupported` (never a
  silent fallback to the identity's tenant). Fix: use an identity whose provider `tenant_id` is the
  registry's tenant and drop `spec.registry.tenant_id`.
- A tenant that demands its own sign-in (Conditional Access, MFA) also fails, with a hint to
  configure a provider for that tenant.
- `--registry` mode has no tenant option.

## Guidance

- Prefer named integrations for stable registries; they make the registry name and identity
  explicit in `atmos.yaml`.
- Use `--identity` when the intent is "log in to every ACR registry attached to this identity."
- Use `--registry` for one-off registry login servers, or when a script intentionally uses ambient
  Azure credentials instead of Atmos Auth.
- ACR credentials are written to Docker's config location, respecting `DOCKER_CONFIG` when set.
  Set `DOCKER_CONFIG` first when the workflow needs isolated credentials.
- `spec.auto_provision: true` triggers ACR login during `atmos auth login`; set it to `false` for
  registries that should only be logged in explicitly.
- `atmos azure acr login` has no `--public` mode (no ECR-Public equivalent) — it always
  authenticates. ACR registries are private by default; grant the principal the built-in
  `AcrPull`/`AcrPush` roles, or the repository-scoped roles when ABAC repository permissions are
  enabled (those supersede the built-in `Acr*` roles). Standard/Premium registries can separately
  enable anonymous pull, which needs no login.
- Installing Docker or related tools for a CI job is out of scope here — route tool installation to
  the `atmos-toolchain` skill.
