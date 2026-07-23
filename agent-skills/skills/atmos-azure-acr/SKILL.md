---
name: atmos-azure-acr
description: "Azure ACR commands in Atmos: atmos azure acr login, ACR auth integrations, Docker credential writes, registry login via identity or explicit registry"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Azure ACR

Use this skill for logging Docker clients into Azure Container Registry through Atmos.
It owns `atmos azure acr login`.

## Command Model

`atmos azure acr login` supports three modes:

```shell
# Named integration from auth.integrations
atmos azure acr login dev/acr

# All azure/acr integrations linked to an identity
atmos azure acr login --identity azure-dev

# Explicit registry login server using ambient Azure credentials
atmos azure acr login --registry myregistry.azurecr.io
```

Named integration and identity modes use Atmos Auth. Explicit `--registry` mode uses ambient Azure
credentials (the Azure SDK default credential chain: environment variables, managed identity,
workload identity, Azure CLI).

## Configuration

Configure ACR integrations under `auth.integrations` with `kind: azure/acr`. Route provider,
identity, device-code, OIDC, and Azure CLI details to `atmos-auth`.

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

`spec.registry` is the same struct used by `aws/ecr` integrations (`account_id`, `region` for
AWS; `name`, `tenant_id` for Azure) — only the fields relevant to the integration's `kind` matter.
Login server = `{name}.azurecr.io`.

## Agent Guidance

- Prefer named integrations for stable registries; they make the registry name and identity
  explicit in `atmos.yaml`.
- Use `--identity` when the intent is "log in to every ACR registry attached to this identity."
- Use `--registry` for one-off registry login servers or when a script intentionally uses ambient
  Azure credentials instead of Atmos Auth.
- ACR credentials are written to Docker's config location, respecting `DOCKER_CONFIG` when set.
  Set `DOCKER_CONFIG` first when the workflow needs isolated credentials.
- `spec.auto_provision: true` triggers ACR login during `atmos auth login`; set it to `false` for
  registries that should only be logged in explicitly.
- There is no ACR equivalent to ECR Public — every registry is private and requires an identity or
  ambient credentials with `AcrPull`/`AcrPush` access.
- If Docker or other tools must be installed for a CI job, route installation to `atmos-toolchain`.

## Routing

| Need | Skill |
|------|-------|
| Azure identity/provider setup, device-code, OIDC, Azure CLI | `atmos-auth` |
| Installing Docker or related tools | `atmos-toolchain` |
| OCI component sources or vendored artifacts stored in registries | `atmos-components`, `atmos-vendoring` |
