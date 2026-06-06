# Providers and Identities Reference

Detailed configuration reference for all authentication providers and identity types supported by Atmos Auth.

## Provider Configuration

### AWS IAM Identity Center (SSO)

```yaml
auth:
  providers:
    <name>:
      kind: aws/iam-identity-center        # Required
      region: us-east-1                     # Required: AWS region for Identity Center instance
      start_url: https://company.awsapps.com/start  # Required: SSO start URL
      auto_provision_identities: true       # Optional: auto-discover accounts/permission sets (default: false)
      session:
        duration: 4h                        # Optional: credential lifetime
      console:
        session_duration: 12h               # Optional: web console session (max 12h for AWS)
      spec:
        files:
          base_path: ~/.config/atmos/aws/   # Optional: custom credential file storage path
```

**Auto-provisioning IAM permissions required:**
- `sso:ListAccounts` -- Enumerates all accessible AWS accounts.
- `sso:ListAccountRoles` -- Lists available permission sets per account.

Without these permissions, auto-provisioning fails gracefully and falls back to manually configured identities.

### AWS SAML

```yaml
auth:
  providers:
    <name>:
      kind: aws/saml                        # Required
      region: us-east-1                     # Required: AWS region
      url: https://company.okta.com/app/amazon_aws/abc123/sso/saml  # Required: SAML SSO URL
      driver: Browser                       # Optional: Browser (default, needs Playwright), GoogleApps, Okta, ADFS
```

The `aws/saml` provider requires the next identity in the chain to be `aws/assume-role`, as the SAML
flow requires selecting a role to assume.

### GitHub Actions OIDC

```yaml
auth:
  providers:
    <name>:
      kind: github/oidc                     # Required
      region: us-east-1                     # Required: AWS region for STS endpoint
      spec:
        audience: sts.us-east-1.amazonaws.com  # Optional: defaults to STS endpoint for region
```

GitHub Actions workflow must have `id-token: write` permission.

### GCP Application Default Credentials

```yaml
auth:
  providers:
    <name>:
      kind: gcp/adc                         # Required
      project_id: my-gcp-project            # Optional: override gcloud config default
      region: us-central1                   # Optional: default region
      scopes:                               # Optional: OAuth scopes
        - https://www.googleapis.com/auth/cloud-platform
```

Requires existing ADC. Run `gcloud auth application-default login` first.

### GCP Workload Identity Federation

```yaml
auth:
  providers:
    <name>:
      kind: gcp/workload-identity-federation  # Required
      project_id: my-gcp-project              # Optional: GCP project ID
      project_number: "123456789012"          # Required: GCP project number (numeric)
      workload_identity_pool_id: github-pool  # Required: WIF pool ID
      workload_identity_provider_id: github-provider  # Required: WIF provider ID
      service_account_email: ci-sa@my-project.iam.gserviceaccount.com  # Optional: SA to impersonate
      scopes:                                 # Optional: OAuth scopes
        - https://www.googleapis.com/auth/cloud-platform
      token_source:                           # Auto-detected in GitHub Actions
        type: url                             # url, file, or environment
        url: https://my-oidc-provider.example.com/token
        request_token: <bearer-token>         # For type: url
        audience: //iam.googleapis.com/projects/...
        allowed_hosts:
          - my-oidc-provider.example.com
        environment_variable: OIDC_TOKEN      # For type: environment
        file_path: /path/to/token             # For type: file
```

**GitHub Actions auto-detection:**
- Sets `token_source.type` to `url`.
- Uses `ACTIONS_ID_TOKEN_REQUEST_URL` as the token endpoint.
- Uses `ACTIONS_ID_TOKEN_REQUEST_TOKEN` as the bearer token.
- Constructs `audience` from `project_number`, `workload_identity_pool_id`, and `workload_identity_provider_id`.
- Validates token URL against known GitHub Actions OIDC hosts.

## Identity Configuration

### AWS Permission Set

```yaml
auth:
  identities:
    <name>:
      kind: aws/permission-set              # Required
      default: true                         # Optional: use when no identity specified
      via:
        provider: <provider-name>           # Required: SSO provider reference
      principal:
        name: AdminAccess                   # Required: permission set name
        account:
          name: development                 # Account name (resolved via SSO ListAccounts)
          id: "123456789012"                # OR account ID directly (no lookup needed)
      session:
        duration: 4h                        # Optional: override provider session duration
```

### AWS Assume Role

```yaml
auth:
  identities:
    <name>:
      kind: aws/assume-role                 # Required
      default: false                        # Optional
      via:
        identity: <identity-name>           # Chain from another identity
        # OR
        provider: <provider-name>           # Direct from provider (mutually exclusive)
      principal:
        assume_role: arn:aws:iam::999999999999:role/RoleName  # Required: role ARN
        session_name: atmos-session         # Optional: for CloudTrail auditing
```

### AWS Assume Root

```yaml
auth:
  identities:
    <name>:
      kind: aws/assume-root                 # Required
      via:
        identity: <identity-name>           # Required: must chain from an existing identity
      principal:
        target_principal: "123456789012"     # Required: 12-digit member account ID
        task_policy_arn: arn:aws:iam::aws:policy/root-task/<PolicyName>  # Required
        duration: 15m                       # Optional: max 15 minutes for AssumeRoot
```

**Supported task policies:**
- `arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials`
- `arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword`
- `arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials`
- `arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy`
- `arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy`

Requires AWS Organizations with centralized root access enabled.

### AWS User (Break-glass)

```yaml
auth:
  identities:
    <name>:
      kind: aws/user                        # Required
      credentials:
        access_key_id: !env AWS_ACCESS_KEY_ID        # Use !env for env var references
        secret_access_key: !env AWS_SECRET_ACCESS_KEY  # Use !env for env var references
        region: us-east-1                            # AWS region
        mfa_arn: arn:aws:iam::123456789012:mfa/user  # Optional: prompts for TOTP
      session:
        duration: 1h                        # Optional: 15m-12h (no MFA) or 15m-36h (with MFA)
```

Store credentials securely with `atmos auth user configure --identity <name>` instead of in config files.

### Azure Subscription

```yaml
auth:
  identities:
    <name>:
      kind: azure/subscription              # Required
      via:
        provider: <azure-provider-name>     # Required: Azure provider reference
      principal:
        subscription_id: "12345678-1234-1234-1234-123456789012"  # Required
        location: eastus                    # Optional: default Azure region
        resource_group: my-rg               # Optional: default resource group
```

Sets environment variables: `AZURE_SUBSCRIPTION_ID`, `ARM_SUBSCRIPTION_ID`, `AZURE_LOCATION`,
`ARM_LOCATION`, etc.

### GCP Service Account

```yaml
auth:
  identities:
    <name>:
      kind: gcp/service-account             # Required
      default: true                         # Optional
      via:
        provider: <gcp-provider-name>       # Required: gcp/adc or gcp/workload-identity-federation
      principal:
        service_account_email: tf@my-project.iam.gserviceaccount.com  # Required
        project_id: my-project              # Optional: extracted from email if not set
        scopes:                             # Optional: defaults to cloud-platform
          - https://www.googleapis.com/auth/cloud-platform
        lifetime: 3600s                     # Optional: default 1h, max 12h
        delegates:                          # Optional: multi-hop impersonation chain
          - sa1@project.iam.gserviceaccount.com
```

Requires the base identity to have `roles/iam.serviceAccountTokenCreator` on the target service account.

### GCP Project

```yaml
auth:
  identities:
    <name>:
      kind: gcp/project                     # Required
      via:
        provider: <gcp-provider-name>       # Optional
      principal:
        project_id: production-project      # Required: GCP project ID
        region: us-central1                 # Optional: default region
        zone: us-central1-a                 # Optional: default zone
```

Sets environment variables: `GOOGLE_CLOUD_PROJECT`, `CLOUDSDK_CORE_PROJECT`, `GOOGLE_CLOUD_REGION`,
`CLOUDSDK_COMPUTE_REGION`, `GOOGLE_CLOUD_ZONE`, `CLOUDSDK_COMPUTE_ZONE`.

## Identity Chaining Patterns

### SSO to Cross-Account Role

The most common pattern: authenticate via SSO, then assume a role in another account.

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start

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
```

### Multi-Hop Role Chain

Chain through multiple roles for progressive access control.

```yaml
auth:
  identities:
    base:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: AdminAccess
        account:
          name: core-identity

    cross-account:
      kind: aws/assume-role
      via:
        identity: base
      principal:
        assume_role: arn:aws:iam::111111111111:role/CrossAccountRole

    restricted:
      kind: aws/assume-role
      via:
        identity: cross-account
      principal:
        assume_role: arn:aws:iam::111111111111:role/RestrictedRole
```

### SAML to Assume Role

SAML provider always requires an `aws/assume-role` as the next identity.

```yaml
auth:
  providers:
    okta:
      kind: aws/saml
      region: us-east-1
      url: https://company.okta.com/app/amazon_aws/abc123/sso/saml

  identities:
    admin:
      kind: aws/assume-role
      via:
        provider: okta
      principal:
        assume_role: arn:aws:iam::123456789012:role/AdminRole
```

### GitHub OIDC for CI/CD

Authenticate GitHub Actions runners without static credentials.

```yaml
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-1

  identities:
    deploy:
      kind: aws/assume-role
      default: true
      via:
        provider: github-oidc
      principal:
        assume_role: arn:aws:iam::123456789012:role/GitHubActionsRole
```

### GCP WIF with Service Account Impersonation

Federate from GitHub Actions OIDC into GCP, then impersonate a service account.

```yaml
auth:
  providers:
    gcp-wif:
      kind: gcp/workload-identity-federation
      project_number: "123456789012"
      workload_identity_pool_id: github-pool
      workload_identity_provider_id: github-provider

  identities:
    terraform:
      kind: gcp/service-account
      default: true
      via:
        provider: gcp-wif
      principal:
        service_account_email: terraform@my-project.iam.gserviceaccount.com
```

## Chain Rules

- Chains can be arbitrarily deep: `provider -> identity -> identity -> ... -> identity`.
- `via.provider` and `via.identity` are mutually exclusive on any given identity.
- `aws/user` identities do not require a `via` field (they have inline credentials).
- Circular dependencies are detected at validation time and rejected with an error.
- Only one identity should be marked `default: true`. Multiple defaults trigger interactive selection.

## Session Configuration

Session durations can be configured at the provider level and overridden at the identity level.

| Identity Kind | Duration Range | Notes |
|---------------|---------------|-------|
| AWS Permission Set | Provider default | Controlled by SSO admin |
| AWS Assume Role | 15m-12h | Standard STS limits |
| AWS Assume Root | Max 15m | AWS-enforced hard limit |
| AWS User (no MFA) | 15m-12h | Standard STS limits |
| AWS User (with MFA) | 15m-36h | Extended with MFA |
| GCP Service Account | Up to 12h | Default 1h |

## Component-Level Overrides

Override authentication at the component level in stack configuration. Component auth is deep-merged with
global auth. Component identities override global identities with the same name.

```yaml
components:
  terraform:
    myapp:
      auth:
        identities:
          custom-role:
            kind: aws/assume-role
            via:
              provider: company-sso
            principal:
              assume_role: arn:aws:iam::123456789012:role/MyAppRole
```

## Profiles for Multi-Environment Auth

Use Atmos profiles to swap provider implementations while keeping the same provider name. Identity
configurations reference a consistent provider name that behaves differently per profile.

```text
profiles/
  developer/auth.yaml    # SSO with standard sessions
  ci/auth.yaml           # GitHub OIDC for pipelines
  platform/auth.yaml     # SSO with extended sessions
```

Activate with `--profile` flag or `ATMOS_PROFILE` environment variable:

```bash
atmos --profile developer auth login
ATMOS_PROFILE=ci atmos terraform apply myapp -s prod
```

## ECR Integration Configuration

```yaml
auth:
  integrations:
    <name>:
      kind: aws/ecr                        # Required
      via:
        identity: <identity-name>           # Required: identity providing AWS credentials
      spec:
        auto_provision: true                # Optional: auto-trigger on identity login (default: true)
        registry:
          account_id: "123456789012"        # Required: AWS account ID for ECR registry
          region: us-east-2                 # Required: AWS region for ECR registry
```

ECR tokens expire after approximately 12 hours (AWS-enforced). Credentials are written to
`~/.docker/config.json`. Integration failures during `atmos auth login` are non-blocking.

## Keyring Configuration

### System Keyring (Default)

```yaml
auth:
  keyring:
    type: system
```

Uses OS-native secure storage: macOS Keychain, Linux Secret Service (GNOME Keyring, KDE Wallet),
Windows Credential Manager.

### File Keyring

```yaml
auth:
  keyring:
    type: file
    spec:
      path: ~/.atmos/keyring                # Optional: custom path (default: XDG data directory)
      password_env: ATMOS_KEYRING_PASSWORD   # Optional: env var for password
```

AES-256 encrypted. Password resolution: env var, then interactive prompt, then error.

### Memory Keyring

```yaml
auth:
  keyring:
    type: memory
```

No persistence. Credentials lost on exit. Best for testing only.

## Logging Configuration

```yaml
auth:
  logs:
    level: Info                             # Debug, Info, Warn, Error
    file: /var/log/atmos-auth.log           # Optional: path to log file
```

Auth logs are separate from main Atmos logs. Debug level includes provider initialization, token refresh,
credential resolution steps, and API call details (without secrets).
