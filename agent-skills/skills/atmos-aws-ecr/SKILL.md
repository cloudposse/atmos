---
name: atmos-aws-ecr
description: "AWS ECR commands in Atmos: atmos aws ecr login, ECR auth integrations, Docker credential writes, registry login via identity or explicit registry"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos AWS ECR

Use this skill for logging Docker clients into AWS Elastic Container Registry through Atmos.
It owns `atmos aws ecr login`.

## Command Model

`atmos aws ecr login` supports three modes:

```shell
# Named integration from auth.integrations
atmos aws ecr login dev/ecr/primary

# All aws/ecr integrations linked to an identity
atmos aws ecr login --identity dev-admin

# Explicit registry URLs using current AWS credentials
atmos aws ecr login --registry 123456789012.dkr.ecr.us-east-1.amazonaws.com
```

Named integration and identity modes use Atmos Auth. Explicit `--registry` mode uses current AWS
credentials from the environment.

## Configuration

Configure ECR integrations under `auth.integrations` with `kind: aws/ecr`. Route provider,
identity, AWS SSO, SAML, OIDC, assume role, and assume root details to `atmos-auth`.

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start/

  identities:
    dev-admin:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: AdministratorAccess
        account: dev

  integrations:
    dev/ecr/primary:
      kind: aws/ecr
      via:
        identity: dev-admin
      spec:
        auto_provision: true
        registry:
          account_id: "123456789012"
          region: us-east-2
```

## Agent Guidance

- Prefer named integrations for stable registries; they make the account, region, and identity
  explicit in `atmos.yaml`.
- Use `--identity` when the intent is "log in to every ECR registry attached to this identity."
- Use `--registry` for one-off registry URLs or when a script intentionally uses ambient AWS
  credentials instead of Atmos Auth.
- ECR credentials are written to Docker's config location, respecting `DOCKER_CONFIG` when set.
  Set `DOCKER_CONFIG` first when the workflow needs isolated credentials.
- `spec.auto_provision: true` triggers ECR login during `atmos auth login`; set it to `false` for
  registries that should only be logged in explicitly.
- If Docker, AWS CLI, or other tools must be installed for a CI job, route installation to
  `atmos-toolchain`.

## Routing

| Need | Skill |
|------|-------|
| AWS identity/provider setup, SSO, SAML, OIDC, assume role/root | `atmos-auth` |
| Installing Docker, AWS CLI, or related tools | `atmos-toolchain` |
| OCI component sources or vendored artifacts stored in registries | `atmos-components`, `atmos-vendoring` |
