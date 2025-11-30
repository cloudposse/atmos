---
slug: introducing-auth-logout
title: 'Introducing atmos auth logout: Secure Credential Cleanup'
sidebar_label: Introducing atmos auth logout
authors:
  - osterman
tags:
  - feature
  - security
release: v1.196.0
---

We're excited to announce a new authentication command: **`atmos auth logout`**. This command provides secure, comprehensive cleanup of locally cached credentials, making it easy to switch between identities, end work sessions, and maintain proper security hygiene.

<!--truncate-->

## Why This Matters

Most cloud practitioners never log out of their cloud provider identities. Not because they don't want to, but because the tooling doesn't make it easy.

When you authenticate with cloud providers, credentials get scattered across your filesystem:

- **AWS**: `~/.aws/credentials`, `~/.aws/config`, session tokens
- **Azure**: `~/.azure/` directory with multiple authentication artifacts
- **Google Cloud**: `~/.config/gcloud/` with various credential files

Most cloud provider tools don't provide a simple, comprehensive logout command. You're left to:

- Manually hunt down and delete credential files across different locations
- Navigate through provider-specific web consoles to revoke tokens
- Hope that session expiration handles cleanup for you

This leads to **credential sprawl**: old, forgotten credentials littering your system, many still valid and exploitable.

The `atmos auth logout` command makes credential cleanup explicit, comprehensive, and easy.

## What's New

### Basic Usage

Logout from a specific identity:

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

### Interactive Mode

Run `atmos auth logout` without arguments for an interactive experience:

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

The interactive mode uses **Charmbracelet Huh** with Atmos theming for a polished experience.

### Provider Logout

Remove all credentials for a specific provider:

```shell
atmos auth logout --provider aws-sso
```

This removes the provider credentials and all identities that authenticate through it:

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

### Dry Run Mode

Preview what would be removed without actually deleting anything:

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

## How It Works

### Authentication Chain Resolution

Atmos intelligently resolves the complete authentication chain for your identity and removes credentials at each step:

```shell
aws-sso → dev-org-admin → dev-admin
   ↓           ↓              ↓
Removed     Removed        Removed
```

This ensures no orphaned credentials are left behind.

### Comprehensive Cleanup

The logout command removes credentials from **all storage locations**:

- ✅ **System keyring entries** - Credentials stored securely by your OS
- ✅ **AWS credential files** - `~/.aws/atmos/<provider>/credentials`
- ✅ **AWS config files** - `~/.aws/atmos/<provider>/config`
- ✅ **Empty directories** - Cleans up provider directories after removal

### Best-Effort Error Handling

The logout command continues even if individual steps fail, ensuring maximum cleanup:

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

This best-effort approach means you always get as much cleanup as possible.

## Security Best Practices

### Browser Sessions

:::warning Important
`atmos auth logout` only removes **local credentials**. Your browser session with the identity provider (AWS SSO, Okta, etc.) remains active.
:::

To completely end your session:

1. Run `atmos auth logout` to remove local credentials
2. Visit your identity provider's website (AWS SSO, Okta, etc.)
3. Sign out from the browser session
4. Close all browser windows.

The command displays this warning after every logout to ensure you don't forget.

### When to Logout

**Logout at the end of your work session:**

```shell
atmos auth logout --provider aws-sso
```

**Logout when switching contexts:**

```shell
atmos auth logout dev-admin
atmos auth login prod-admin
```

**Logout when troubleshooting authentication:**

```shell
atmos auth logout dev-admin --dry-run  # Preview
atmos auth logout dev-admin            # Execute
atmos auth login dev-admin             # Fresh login
```

### Audit Trail

All logout operations are logged for security auditing:

```shell
2025-10-17T10:15:30Z DEBUG Starting logout identity=dev-admin
2025-10-17T10:15:30Z DEBUG Authentication chain built chain=[aws-sso dev-org-admin dev-admin]
2025-10-17T10:15:30Z DEBUG Removing keyring entry alias=aws-sso
2025-10-17T10:15:30Z INFO Logout completed identity=dev-admin removed=3
```

Enable debug logging with `ATMOS_LOGS_LEVEL=Debug` to see detailed audit information.

## Use Cases

### 1. Daily Workflow

Start and end your day with clean credential state:

```shell
# Morning: Login for the day
atmos auth login

# Evening: Logout for security
atmos auth logout
```

### 2. Multi-Identity Switching

Switch between development and production environments:

```shell
# Switch from dev to prod
atmos auth logout dev-admin
atmos auth login prod-admin

# Later: Switch back
atmos auth logout prod-admin
atmos auth login dev-admin
```

### 3. Troubleshooting

Clear credential cache when debugging authentication issues:

```shell
# Check current status
atmos auth whoami

# Clear and re-authenticate
atmos auth logout dev-admin
atmos auth login dev-admin

# Verify
atmos auth whoami
```

### 4. Compliance

Demonstrate credential cleanup for security audits:

```shell
# Interactive review of what to remove
atmos auth logout

# Select "All identities" to clear everything
# Audit logs show complete cleanup
```

## Configuration

The logout command works with your existing `atmos.yaml` authentication configuration:

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

No additional configuration required - logout uses the same identity and provider definitions as login.

## Technical Details

### Cross-Platform Support

The logout command uses native Go libraries for maximum compatibility:

- **File operations**: `os.RemoveAll()` for cross-platform directory removal
- **Keyring access**: `go-keyring` library supporting macOS, Linux, and Windows
- **Path handling**: `filepath` package for platform-specific path separators.

### Error Handling

Following Atmos error handling patterns:

- Uses static error sentinels from `errors/errors.go`
- Wraps errors with `errors.Join` for proper error chains
- Continues cleanup on non-fatal errors
- Reports all errors at completion.

### Interface Extensions

The logout feature extends the auth interfaces:

```go
// Provider interface
type Provider interface {
    // ... existing methods
    Logout(ctx context.Context) error
}

// Identity interface
type Identity interface {
    // ... existing methods
    Logout(ctx context.Context) error
}

// AuthManager interface
type AuthManager interface {
    // ... existing methods
    Logout(ctx context.Context, identityName string) error
    LogoutProvider(ctx context.Context, providerName string) error
    LogoutAll(ctx context.Context) error
}
```

### Telemetry

Like all Atmos commands, logout automatically captures anonymous usage telemetry:

- Command path: `auth logout`
- Error state: Boolean only (no sensitive data)
- No user data, credentials, or identity names captured

## What's Next

This initial release supports:

- ✅ AWS provider logout (SSO, SAML, user credentials)
- ✅ Identity chain resolution
- ✅ Interactive mode
- ✅ Dry run mode

Future enhancements:

- 🔄 Azure Entra ID provider logout
- 🔄 GCP OIDC provider logout
- 🔄 GitHub Actions OIDC logout
- 🔄 Selective logout (keep provider, remove identity only)
- 🔄 Automatic cleanup of expired credentials

## Get Started

The `atmos auth logout` command is available in Atmos v1.x.x and later.

**Try it now:**

```shell
# Interactive mode
atmos auth logout

# Or logout from specific identity
atmos auth logout <identity-name>

# See all options
atmos auth logout --help
```

**Learn more:**

- 📖 [CLI Documentation](/cli/commands/auth/logout) - Complete command reference
- 📋 [PRD: Auth Logout](https://github.com/cloudposse/atmos/blob/main/docs/prd/auth-logout.md) - Technical design document
- 🔐 [Authentication Overview](/cli/commands/auth/usage) - Complete authentication overview

## Feedback Welcome

We'd love to hear how you're using `atmos auth logout`:

- 💬 **Discuss** - Share your thoughts in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- 🐛 **Report Issues** - Found a bug? [Open an issue](https://github.com/cloudposse/atmos/issues)
- 🚀 **Contribute** - Submit PRs for improvements

Secure credential management is critical for infrastructure automation. We're committed to making authentication with Atmos both powerful and secure.

---

**Ready to try it?** Run `atmos auth logout` to get started!
