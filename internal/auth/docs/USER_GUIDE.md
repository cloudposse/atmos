# Atmos Auth User Guide

## Overview

Atmos Auth provides a unified authentication system for cloud providers, supporting complex identity chaining and credential management. This guide will help you get started with configuring and using Atmos Auth for your infrastructure projects.

## Quick Start

### 1. Basic Configuration

Add authentication configuration to your `atmos.yaml`:

```yaml
auth:
  providers:
    my-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://mycompany.awsapps.com/start

  identities:
    admin:
      kind: aws/permission-set
      default: true
      via:
        provider: my-sso
      principal:
        name: AdminAccess
        account:
          name: "123456789012"
```

Notes:

- Region is required for the GitHub OIDC provider and is validated at construction time.

### 2. Validate Configuration

```bash
atmos auth validate
```

### 3. Authenticate

```bash
# Use default identity
atmos auth login

# Use specific identity
atmos auth login --identity admin
```

### 4. Check Authentication Status

```bash
atmos auth whoami
```

### 5. Use with Terraform

```bash
# Atmos automatically handles authentication
atmos terraform plan mycomponent -s dev
```

## Authentication Concepts

### Providers

**Providers** are the root authentication sources that obtain initial credentials:

- **AWS SSO**: `aws/iam-identity-center`
- **AWS SAML**: `aws/saml`
- **GitHub OIDC**: `github/oidc`
- **AWS Assume Role**: `aws/assume-role`

### Identities

**Identities** use provider credentials to assume specific roles or permissions:

- **AWS Permission Set**: `aws/permission-set`
- **AWS Assume Role**: `aws/assume-role`
- **AWS User**: `aws/user`

### Identity Chaining

You can chain identities to create complex authentication flows:

```yaml
identities:
  # Base permission set from SSO
  base-admin:
    kind: aws/permission-set
    via:
      provider: my-sso
    principal:
      name: AdminAccess
      account:
        name: "123456789012"

  # Cross-account role using base permissions
  prod-admin:
    kind: aws/assume-role
    via:
      identity: base-admin # Chain through another identity
    principal:
      assume_role: arn:aws:iam::999999999999:role/ProductionAdmin
      session_name: atmos-prod-access
```

## Configuration Examples

### AWS SSO with Permission Sets

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start

  identities:
    dev-admin:
      kind: aws/permission-set
      default: true
      via:
        provider: company-sso
      principal:
        name: AdminAccess
        account:
          name: "123456789012"

    prod-readonly:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: ReadOnlyAccess
        account:
          name: "999999999999"
```

### AWS SAML Authentication

```yaml
auth:
  providers:
    okta-saml:
      kind: aws/saml
      region: us-east-1
      url: https://company.okta.com/app/amazon_aws/abc123/sso/saml

  identities:
    saml-admin:
      kind: aws/assume-role
      default: true
      via:
        provider: okta-saml
      principal:
        assume_role: arn:aws:iam::123456789012:role/AdminRole
        session_name: okta-saml-session
```

### GitHub Actions OIDC

```yaml
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-1 # Required

  identities:
    github-deploy:
      kind: aws/assume-role
      default: true
      via:
        provider: github-oidc
      principal:
        assume_role: arn:aws:iam::123456789012:role/GitHubActionsRole
        session_name: github-actions
```

### AWS User (Break-glass)

```yaml
auth:
  identities:
    emergency-user:
      kind: aws/user
      credentials:
        access_key_id: !env EMERGENCY_AWS_ACCESS_KEY_ID
        secret_access_key: !env EMERGENCY_AWS_SECRET_ACCESS_KEY
        region: us-east-1
```

## CLI Commands

### Authentication Commands

```bash
# Validate auth configuration
atmos auth validate
atmos auth validate --verbose

# Login (authenticate and cache credentials)
atmos auth login
atmos auth login --identity prod-admin

# Check current authentication status
atmos auth whoami
atmos auth whoami --identity dev-admin

# Get environment variables
atmos auth env
atmos auth env --identity prod-admin --format export
atmos auth env --format json
atmos auth env --format dotenv

# Execute command with authentication
atmos auth exec --identity prod-admin -- aws sts get-caller-identity
atmos auth exec -- terraform plan

# Configure AWS user credentials
atmos auth user configure
```

### Help and Information

```bash
# General auth help
atmos auth --help

# Command-specific help
atmos auth login --help
atmos auth env --help
```

## Component-Level Configuration

Override authentication settings for specific components:

```yaml
# In your component configuration
components:
  terraform:
    myapp:
      auth:
        identities:
          custom-role:
            kind: aws/assume-role
            via:
              provider: my-sso
            principal:
              assume_role: arn:aws:iam::123456789012:role/MyAppRole
        default_identity: custom-role
```

## Environment Variable Formats

### Export Format

```bash
atmos auth env --format export
# Output:
export AWS_ACCESS_KEY_ID="AKIA..."
export AWS_SECRET_ACCESS_KEY="..."
export AWS_SESSION_TOKEN="..."
```

### JSON Format

```bash
atmos auth env --format json
# Output:
{
  "AWS_ACCESS_KEY_ID": "AKIA...",
  "AWS_SECRET_ACCESS_KEY": "...",
  "AWS_SESSION_TOKEN": "..."
}
```

### Dotenv Format

```bash
atmos auth env --format dotenv
# Output:
AWS_ACCESS_KEY_ID=AKIA...
AWS_SECRET_ACCESS_KEY=...
AWS_SESSION_TOKEN=...
```

## Default Identity Handling

### Single Default

When you have one default identity, it's used automatically:

```yaml
identities:
  admin:
    kind: aws/permission-set
    default: true # This will be used by default
```

### Multiple Defaults

When multiple defaults exist, Atmos behavior depends on the environment:

**Interactive Mode**: Prompts you to choose

```bash
$ atmos auth whoami
? Multiple default identities found. Please choose one:
  ▸ dev-admin
    prod-admin
    staging-admin
```

**CI Mode**: Returns an error

```bash
$ CI=true atmos auth whoami
Error: multiple default identities found: [dev-admin, prod-admin]
```

### No Defaults

**Interactive Mode**: Shows all available identities

```bash
$ atmos auth whoami
? No default identity configured. Please choose an identity:
  ▸ dev-admin
    prod-admin
    staging-admin
```

**CI Mode**: Returns an error

```bash
$ CI=true atmos auth whoami
Error: no default identity configured
```

## Credential Storage

Atmos securely stores credentials using your operating system's keyring:

- **macOS**: Keychain
- **Linux**: Secret Service (GNOME Keyring, KDE Wallet)
- **Windows**: Windows Credential Manager

### Credential Expiration

- Credentials are automatically refreshed when expired
- You can check expiration with `atmos auth whoami`
- Manual refresh: `atmos auth login --identity <name>`

### Clearing Credentials

```bash
# Clear all cached credentials
atmos auth logout

# Clear specific identity credentials
atmos auth logout --identity prod-admin
```

## AWS File Management

Atmos manages AWS credential files separately from your personal AWS configuration:

### File Locations

- Credentials: `~/.aws/atmos/<provider>/credentials`
- Config: `~/.aws/atmos/<provider>/config`

### Environment Variables

Atmos automatically sets:

- `AWS_SHARED_CREDENTIALS_FILE`: Points to Atmos-managed credentials
- `AWS_CONFIG_FILE`: Points to Atmos-managed config
- `AWS_PROFILE`: Set to the identity name

### Your Files Remain Untouched

- Your existing `~/.aws/credentials` and `~/.aws/config` are never modified
- Atmos uses separate files to avoid conflicts

## Troubleshooting

### Common Issues

**Configuration Validation Errors**

```bash
atmos auth validate --verbose
```

**Authentication Failures**

```bash
# Check current status
atmos auth whoami

# Re-authenticate
atmos auth login --identity <name>

# Check with verbose output
atmos auth login --identity <name> --verbose
```

**Permission Errors**

```bash
# Verify identity configuration
atmos auth validate

# Check assumed role/permissions
atmos auth exec --identity <name> -- aws sts get-caller-identity
```

**Environment Variable Issues**

```bash
# Check what variables are set
atmos auth env --identity <name>

# Test environment
atmos auth exec --identity <name> -- env | grep AWS
```

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
export ATMOS_LOG_LEVEL=Debug
atmos auth login --identity <name>
```

### Logs Configuration

Configure logging in `atmos.yaml`:

```yaml
auth:
  logs:
    level: Debug # Debug, Info, Warn, Error
    file: /tmp/atmos-auth.log
```

## Workflows Integration

Use Atmos Auth in workflows:

```yaml
# atmos.yaml workflows section
workflows:
  deploy:
    description: Deploy with authentication
    steps:
      - name: validate-auth
        command: atmos auth validate
      - name: deploy-dev
        command: atmos terraform apply myapp -s dev
        identity: dev-admin
      - name: deploy-prod
        command: atmos terraform apply myapp -s prod
        identity: prod-admin
```

## CI/CD Integration

### GitHub Actions

```yaml
- name: Configure AWS credentials
  run: |
    # Atmos handles authentication automatically in CI
    atmos auth whoami

- name: Deploy infrastructure
  run: |
    atmos terraform apply myapp -s prod
  env:
    CI: true # Ensures non-interactive mode
```

### GitLab CI

```yaml
deploy:
  script:
    - atmos auth validate
    - atmos terraform apply myapp -s prod
  variables:
    CI: "true"
```

## Security Best Practices

### Credential Management

- Never commit credentials to version control
- Use environment variables for sensitive data: `!env VAR_NAME`
- Regularly rotate credentials
- Use least-privilege access

### Configuration Security

- Validate configurations regularly: `atmos auth validate`
- Use specific account IDs in permission sets
- Implement proper session naming for audit trails
- Monitor authentication logs

### Environment Isolation

- Use different identities for different environments
- Separate providers for different security domains
- Implement proper identity chaining for cross-account access

## Advanced Usage

### Custom Session Names

```yaml
identities:
  admin:
    kind: aws/assume-role
    principal:
      assume_role: arn:aws:iam::123456789012:role/AdminRole
      session_name: "atmos-{{.User}}-{{.Timestamp}}"
```

### Conditional Configuration

Use environment templating for dynamic configuration:

```yaml
identities:
  dynamic-user:
    kind: aws/user
    credentials:
      access_key_id: !env AWS_ACCESS_KEY_ID_${ENVIRONMENT}
      secret_access_key: !env AWS_SECRET_ACCESS_KEY_${ENVIRONMENT}
```

### Identity Inheritance

```yaml
identities:
  base-admin:
    kind: aws/permission-set
    via:
      provider: sso
    principal:
      name: AdminAccess
      account:
        name: "123456789012"

  # Inherits from base-admin but assumes different role
  cross-account-admin:
    kind: aws/assume-role
    via:
      identity: base-admin
    principal:
      assume_role: arn:aws:iam::999999999999:role/CrossAccountAdmin
```

This guide covers the essential aspects of using Atmos Auth. For more advanced scenarios or troubleshooting, refer to the architecture documentation and provider-specific guides.
