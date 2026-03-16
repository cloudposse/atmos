---
name: atmos-auth
description: "Authentication and identity management: providers (SSO/SAML/OIDC/GCP), identities (AWS/Azure/GCP), keyring, identity chaining, login/exec/shell/console"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Authentication and Identity Management

Atmos Auth provides a unified authentication layer for multiple cloud providers. It consolidates AWS SSO, SAML,
OIDC, GitHub Actions, GCP Workload Identity Federation, Azure, and static credentials into a single configuration
model in `atmos.yaml`. Credentials are managed through providers (upstream authentication systems) and identities
(the roles and accounts obtained from those providers), with support for identity chaining, keyring-based
credential storage, and integrations like ECR.

## Architecture Overview

The auth system has four layers configured under the `auth:` key in `atmos.yaml`:

1. **Providers** -- Upstream systems that issue initial credentials (SSO, SAML, OIDC, GCP ADC/WIF).
2. **Identities** -- Roles, permission sets, or accounts obtained from providers or chained from other identities.
3. **Keyring** -- Secure credential storage backend (system keyring, encrypted file, or in-memory).
4. **Integrations** -- Client-side credential materializations (e.g., ECR Docker login) triggered by identity auth.

```yaml
auth:
  logs:
    level: Info                    # Debug, Info, Warn, Error
    file: /path/to/auth.log       # Optional log file
  keyring:
    type: system                   # system, file, or memory
  providers:
    <name>:
      kind: <provider-kind>
      # Provider-specific fields
  identities:
    <name>:
      kind: <identity-kind>
      # Identity-specific fields
  integrations:
    <name>:
      kind: aws/ecr
      # Integration-specific fields
```

## Provider Types

### AWS IAM Identity Center (SSO)

The most common provider for AWS organizations. Requires `kind`, `region`, and `start_url`.

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start
      auto_provision_identities: true   # Auto-discover accounts and permission sets
      session:
        duration: 4h
      console:
        session_duration: 12h           # Web console session (max 12h)
```

When `auto_provision_identities: true`, Atmos queries `sso:ListAccounts` and `sso:ListAccountRoles` during
login to automatically create identities for all assigned permission sets.

### AWS SAML

For SAML-based IdPs (Okta, Google Apps, ADFS). The next identity in the chain must be `aws/assume-role`.

```yaml
auth:
  providers:
    okta-saml:
      kind: aws/saml
      region: us-east-1
      url: https://company.okta.com/app/amazon_aws/abc123/sso/saml
      driver: Browser              # Browser, GoogleApps, Okta, or ADFS
```

### GitHub Actions OIDC

For CI/CD pipelines in GitHub Actions. Requires `id-token: write` permission in the workflow.

```yaml
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-1
      spec:
        audience: sts.us-east-1.amazonaws.com   # Optional, defaults to STS endpoint
```

### GCP Application Default Credentials

For local development using existing `gcloud` authentication. Requires `gcloud auth application-default login`.

```yaml
auth:
  providers:
    gcp-adc:
      kind: gcp/adc
      spec:
        project_id: my-gcp-project        # Optional, defaults to gcloud config
        region: us-central1               # Optional
        scopes:
          - https://www.googleapis.com/auth/cloud-platform
```

### GCP Workload Identity Federation

For CI/CD using OIDC tokens. In GitHub Actions, `token_source` is auto-detected from environment variables.

```yaml
auth:
  providers:
    gcp-wif:
      kind: gcp/workload-identity-federation
      spec:
        project_id: my-gcp-project
        project_number: "123456789012"
        workload_identity_pool_id: github-pool
        workload_identity_provider_id: github-provider
        service_account_email: ci-sa@my-project.iam.gserviceaccount.com
```

For non-GitHub environments, configure `token_source` explicitly with `type` (`url`, `file`, or `environment`),
the source location, `audience`, and `allowed_hosts`.

## Identity Types

### AWS Permission Set

Maps to an SSO permission set on a specific account. Use `principal.account.name` (resolved via SSO) or
`principal.account.id` (direct).

```yaml
auth:
  identities:
    dev-admin:
      kind: aws/permission-set
      default: true
      via:
        provider: company-sso
      principal:
        name: AdminAccess
        account:
          name: development
```

### AWS Assume Role

Assumes an IAM role, either directly from a provider or chained from another identity.

```yaml
auth:
  identities:
    prod-admin:
      kind: aws/assume-role
      via:
        identity: base-admin       # Chain from another identity
      principal:
        assume_role: arn:aws:iam::999999999999:role/ProductionAdmin
        session_name: atmos-prod   # Optional, for CloudTrail auditing
```

### AWS Assume Root

Centralized root access in AWS Organizations using `sts:AssumeRoot`. Limited to 15-minute sessions.

```yaml
auth:
  identities:
    root-audit:
      kind: aws/assume-root
      via:
        identity: admin-base
      principal:
        target_principal: "123456789012"
        task_policy_arn: arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials
        duration: 15m
```

Supported task policies: `IAMAuditRootUserCredentials`, `IAMCreateRootUserPassword`,
`IAMDeleteRootUserCredentials`, `S3UnlockBucketPolicy`, `SQSUnlockQueuePolicy`.

### AWS User (Break-glass)

Static IAM user credentials for emergency access. Use `!env` to reference environment variables.

```yaml
auth:
  identities:
    emergency:
      kind: aws/user
      credentials:
        access_key_id: !env EMERGENCY_AWS_ACCESS_KEY_ID
        secret_access_key: !env EMERGENCY_AWS_SECRET_ACCESS_KEY
        region: us-east-1
        mfa_arn: arn:aws:iam::123456789012:mfa/username   # Optional MFA
```

### Azure Subscription

Targets a specific Azure subscription. Sets `AZURE_SUBSCRIPTION_ID`, `ARM_SUBSCRIPTION_ID`, etc.

```yaml
auth:
  identities:
    dev-subscription:
      kind: azure/subscription
      via:
        provider: azure-cli
      principal:
        subscription_id: "12345678-1234-1234-1234-123456789012"
        location: eastus
        resource_group: my-rg
```

### GCP Service Account

Impersonates a GCP service account. Requires `roles/iam.serviceAccountTokenCreator` on the base identity.

```yaml
auth:
  identities:
    terraform:
      kind: gcp/service-account
      default: true
      via:
        provider: gcp-adc
      principal:
        service_account_email: terraform@my-project.iam.gserviceaccount.com
        project_id: my-project
        lifetime: 3600s
```

### GCP Project

Sets GCP project context. Sets `GOOGLE_CLOUD_PROJECT`, `CLOUDSDK_CORE_PROJECT`, `GOOGLE_CLOUD_REGION`.

```yaml
auth:
  identities:
    prod-project:
      kind: gcp/project
      via:
        provider: gcp-adc
      principal:
        project_id: production-project
        region: us-central1
        zone: us-central1-a
```

## Identity Chaining

Chains can be arbitrarily deep: `provider -> identity -> identity -> ... -> identity`. Use `via.provider` to
start from a provider or `via.identity` to chain from another identity. They are mutually exclusive. Circular
dependencies are detected and rejected.

```yaml
auth:
  identities:
    base-admin:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: AdminAccess
        account:
          name: core-identity
    prod-admin:
      kind: aws/assume-role
      via:
        identity: base-admin
      principal:
        assume_role: arn:aws:iam::999999999999:role/ProductionAdmin
    prod-readonly:
      kind: aws/assume-role
      via:
        identity: prod-admin
      principal:
        assume_role: arn:aws:iam::999999999999:role/ReadOnlyAccess
```

## Keyring Backends

| Type | Persistence | Security | Use Case |
|------|-------------|----------|----------|
| `system` | Yes | High (OS-managed) | Interactive workstations (Keychain, GNOME Keyring, Windows Credential Manager) |
| `file` | Yes | Medium (AES-256 encrypted) | Headless servers, Docker containers, CI/CD |
| `memory` | No | Low (in-process) | Testing, temporary sessions |

File keyring password resolution: `ATMOS_KEYRING_PASSWORD` env var, then interactive prompt, then error.

## Commands Quick Reference

| Command | Purpose |
|---------|---------|
| `atmos auth login [--identity <name>]` | Authenticate with SSO/SAML/OIDC/static credentials |
| `atmos auth whoami [--identity <name>]` | Show current authentication status |
| `atmos auth validate [--verbose]` | Validate auth configuration for syntax and logic errors |
| `atmos auth shell [--identity <name>]` | Launch interactive shell with credentials pre-configured |
| `atmos auth exec [--identity <name>] -- <cmd>` | Execute a single command with identity credentials |
| `atmos auth env [--format bash\|json\|dotenv]` | Export credentials as environment variables |
| `atmos auth console [--destination <url>]` | Open cloud provider web console in browser |
| `atmos auth list [--format table\|tree\|json\|yaml\|graphviz\|mermaid]` | List providers and identities |
| `atmos auth ecr-login [integration]` | Login to AWS ECR registries |
| `atmos auth logout [identity] [--all] [--provider]` | Clear cached credentials |

All commands accepting `--identity` support three modes: with value (use that identity), without value
(interactive selector), or omitted (use default or prompt). The `-i` alias works for all.

## Disabling Authentication

Disable Atmos-managed auth to use native cloud provider credentials:

```bash
atmos terraform plan mycomponent --stack=dev --identity=false
# or
export ATMOS_IDENTITY=false
```

Recognized disable values: `false`, `0`, `no`, `off` (case-insensitive).

## CI/CD Integration

### GitHub Actions with OIDC

```yaml
jobs:
  deploy:
    permissions:
      id-token: write
      contents: read
    steps:
      - uses: actions/checkout@v4
      - run: atmos terraform apply mycomponent -s prod
```

For AWS OIDC, configure `github/oidc` provider with `aws/assume-role` identity. For GCP WIF, configure
`gcp/workload-identity-federation` provider -- `token_source` is auto-detected in GitHub Actions.

### Disabling Auth in CI

When the CI platform provides credentials natively:

```yaml
env:
  ATMOS_IDENTITY: false
run: atmos terraform apply mycomponent --stack=prod
```

## Profiles for Environment Switching

Use Atmos profiles to swap provider and identity configurations while keeping names consistent:

```bash
atmos --profile developer terraform plan myapp -s dev
ATMOS_PROFILE=ci atmos terraform apply myapp -s prod
```

Each profile is a directory (e.g., `profiles/developer/auth.yaml`) containing auth overrides.

## ECR Integrations

ECR integrations auto-trigger on identity login when `auto_provision: true` (default):

```yaml
auth:
  integrations:
    dev/ecr:
      kind: aws/ecr
      via:
        identity: dev-admin
      spec:
        auto_provision: true
        registry:
          account_id: "123456789012"
          region: us-east-2
```

Integration failures are non-blocking during `atmos auth login`. Use `atmos auth ecr-login` to retry.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `ATMOS_IDENTITY` | Default identity name, or `false` to disable auth |
| `ATMOS_KEYRING_TYPE` | Override keyring backend (`system`, `file`, `memory`) |
| `ATMOS_KEYRING_PASSWORD` | Password for file keyring |
| `ATMOS_XDG_CONFIG_HOME` | Override config directory for AWS files |
| `ATMOS_XDG_DATA_HOME` | Override data directory for file keyring |

## Security Best Practices

- Never commit credentials to version control. Use `!env VAR_NAME` for sensitive values.
- Use shortest practical session durations for high-security environments.
- Validate configurations regularly with `atmos auth validate`.
- Use identity chaining with least-privilege roles rather than broad permissions.
- Logout when switching contexts or ending sessions: `atmos auth logout`.
- Browser sessions with IdPs remain active after local logout -- sign out from the IdP separately.

## Additional Resources

- [references/providers-and-identities.md](references/providers-and-identities.md) -- Detailed provider and identity configuration patterns
- [references/commands-reference.md](references/commands-reference.md) -- Complete command reference for all auth subcommands
