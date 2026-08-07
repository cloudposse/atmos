---
slug: introducing-auth-logout
title: 'Why nobody logs out of their cloud identities'
date: 2025-10-21T12:00:00.000Z
sidebar_label: Introducing atmos auth logout
authors:
  - osterman
tags:
  - feature
  - security
release: v1.196.0
---

Most cloud practitioners never log out of their cloud provider identities. Not because they don't want to, but because the tooling doesn't make it easy. There is a command to get credentials and almost never a command to get rid of them, so they accumulate. The `atmos auth logout` command is the missing half: it removes the locally cached credentials for an identity, for a provider, or for everything.

<!--truncate-->

## The Problem

When you authenticate with cloud providers, credentials get scattered across your filesystem:

- **AWS**: `~/.aws/credentials`, `~/.aws/config`, session tokens
- **Azure**: `~/.azure/` directory with multiple authentication artifacts
- **Google Cloud**: `~/.config/gcloud/` with various credential files

Few cloud provider tools ship a single logout command that covers all of it. You are left to manually hunt down and delete credential files across different locations, navigate provider-specific web consoles to revoke tokens, or hope that session expiration handles cleanup for you.

This leads to **credential sprawl**: old, forgotten credentials littering your system, many still valid and exploitable.

## The Fix

The `atmos auth logout` command makes credential cleanup explicit and repeatable. It resolves the full authentication chain behind an identity, removes the cached credentials at every step of that chain, and tells you what it removed. It uses the identity and provider definitions you already have, so there is nothing new to configure.

## How to Use It

### Log out of an identity

```shell
atmos auth logout dev-admin
```

This removes credentials for `dev-admin` and all identities in its authentication chain:

```shell
Logging out from identity: dev-admin

Building authentication chain...
  ✓ Chain: aws-sso → dev-org-admin → dev-admin

Removing credentials...
  ✓ Keyring: aws-sso
  ✓ Keyring: dev-org-admin
  ✓ Keyring: dev-admin
  ✓ Files: ~/.aws/atmos/aws-sso/

Successfully logged out from 3 identities

⚠️  Note: This only removes local credentials. Your browser session
    may still be active. Visit your identity provider to end your
    browser session.
```

### Pick from a list

Run the command without arguments and it asks what to remove:

```shell
atmos auth logout
```

```shell
? Choose what to logout from:
  ❯ Identity: dev-admin
    Identity: prod-admin
    Identity: dev-readonly
    Provider: aws-sso (removes all identities)
    All identities (complete logout)
```

The picker is keyboard-driven and themed like the rest of the Atmos CLI.

### Log out of a provider

```shell
atmos auth logout --provider aws-sso
```

This removes the provider credentials and every identity that authenticates through it:

```shell
Logging out from provider: aws-sso

Removing all credentials for provider...
  ✓ Keyring: aws-sso
  ✓ Keyring: dev-org-admin (via aws-sso)
  ✓ Keyring: dev-admin (via aws-sso)
  ✓ Keyring: prod-admin (via aws-sso)
  ✓ Files: ~/.aws/atmos/aws-sso/

Successfully logged out from 4 identities
```

### Preview first

```shell
atmos auth logout dev-admin --dry-run
```

```shell
Dry run mode: No credentials will be removed

Would remove from identity: dev-admin
  • Keyring: aws-sso
  • Keyring: dev-org-admin
  • Keyring: dev-admin
  • Files: ~/.aws/atmos/aws-sso/credentials
  • Files: ~/.aws/atmos/aws-sso/config

3 identities would be logged out
```

## What Gets Removed

Logging out of one identity is rarely enough, because that identity was reached through others. Atmos resolves the complete authentication chain and removes credentials at each step, so nothing orphaned is left behind:

```shell
aws-sso → dev-org-admin → dev-admin
    ↓           ↓              ↓
Removed     Removed        Removed
```

At each step it clears the system keyring entry (Keychain on macOS, Secret Service on Linux, Credential Manager on Windows), the AWS credential file at `~/.aws/atmos/<provider>/credentials`, the AWS config file at `~/.aws/atmos/<provider>/config`, and the provider directory itself once it is empty.

Cleanup is best effort. If one step fails, the rest still run, and the failures are reported at the end:

```shell
Logging out from identity: dev-admin

Removing credentials...
  ✓ Keyring: aws-sso
  ✗ Keyring: dev-admin (not found - already logged out)
  ✓ Files: ~/.aws/atmos/aws-sso/

Logged out with warnings (2/3 successful)

Errors encountered:
  • dev-admin: credential not found in keyring
```

That way a single missing entry never blocks the rest of the cleanup.

## Browser Sessions

:::warning Important
The `atmos auth logout` command only removes **local credentials**. Your browser session with the identity provider (AWS SSO, Okta, etc.) remains active.
:::

To completely end your session:

1. Run `atmos auth logout` to remove local credentials
2. Visit your identity provider's website (AWS SSO, Okta, etc.)
3. Sign out from the browser session
4. Close all browser windows.

The command displays this warning after every logout so the second half does not get forgotten.

## When to Log Out

At the end of a work session, clear the provider and everything under it:

```shell
atmos auth logout --provider aws-sso
```

When switching between environments, drop the old identity before taking the new one:

```shell
atmos auth logout dev-admin
atmos auth login prod-admin
```

When authentication misbehaves, preview the cleanup, run it, then re-authenticate from a known-empty state:

```shell
atmos auth logout dev-admin --dry-run  # Preview
atmos auth logout dev-admin            # Execute
atmos auth login dev-admin             # Fresh login
```

Check the result on either side with `atmos auth whoami`:

```shell
# Check current status
atmos auth whoami

# Clear and re-authenticate
atmos auth logout dev-admin
atmos auth login dev-admin

# Verify
atmos auth whoami
```

For an audit, the interactive picker doubles as a review step — choose "All identities" to clear everything, and the logs record what went.

## Audit Trail

Logout operations are logged, so the cleanup is reviewable after the fact:

```shell
2025-10-17T10:15:30Z DEBUG Starting logout identity=dev-admin
2025-10-17T10:15:30Z DEBUG Authentication chain built chain=[aws-sso dev-org-admin dev-admin]
2025-10-17T10:15:30Z DEBUG Removing keyring entry alias=aws-sso
2025-10-17T10:15:30Z INFO Logout completed identity=dev-admin removed=3
```

Set `ATMOS_LOGS_LEVEL=Debug` to see this detail.

## Configuration

Logout reads the same `atmos.yaml` authentication configuration that login does:

```yaml
auth:
  providers:
    aws-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://mycompany.awsapps.com/start

  identities:
    dev-admin:
      kind: aws/permission-set
      via:
        provider: aws-sso
      principal:
        name: AdminAccess
        account:
          name: "dev-account"
```

No extra configuration is required.

## What's Next

This first release covers AWS provider logout (SSO, SAML, and user credentials), identity chain resolution, the interactive picker, and dry-run mode. Still to come:

- Azure Entra ID provider logout
- GCP OIDC provider logout
- GitHub Actions OIDC logout
- Selective logout (keep the provider, remove the identity only)
- Automatic cleanup of expired credentials

The command is available in Atmos v1.196.0 and later:

```shell
# Interactive mode
atmos auth logout

# Or logout from specific identity
atmos auth logout <identity-name>

# See all options
atmos auth logout --help
```

Further reading: the [CLI documentation](/cli/commands/auth/logout) for the complete command reference, the [PRD: Auth Logout](https://github.com/cloudposse/atmos/blob/main/docs/prd/auth-logout.md) for the design behind it, and the [authentication overview](/cli/commands/auth/usage) for how logout fits with the rest of the auth commands.

## Get Involved

Tell us which credential locations we still miss on your machine, and which providers you want covered next. Open an issue at [github.com/cloudposse/atmos/issues](https://github.com/cloudposse/atmos/issues).
