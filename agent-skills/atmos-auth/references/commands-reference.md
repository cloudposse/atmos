# Auth Commands Reference

Complete reference for all `atmos auth` subcommands, flags, and usage patterns.

## Common Flag: --identity

All auth commands that accept `--identity` (alias `-i`) support three modes:

- **With value** (`--identity admin`): Use the specified identity.
- **Without value** (`--identity`): Force interactive selector, even if a default is configured.
- **Omitted**: Use the default identity, or prompt if no default is set.

Environment variables: `ATMOS_IDENTITY` or `IDENTITY` (checked in that order).

Set `ATMOS_IDENTITY=false` (or `0`, `no`, `off`) to disable Atmos-managed authentication entirely.

---

## atmos auth login

Authenticate with a configured identity using SSO, SAML, OIDC, or static credentials. Credentials are
cached to avoid repeated logins until expiration.

```shell
atmos auth login [--identity <name>] [--provider <name>]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Identity to authenticate (see common flag above) |
| `--provider` | `-p` | Authenticate directly with a provider (bypasses identity selection) |

### Examples

```shell
atmos auth login                        # Default identity
atmos auth login --identity             # Interactive selector
atmos auth login --identity admin       # Specific identity
atmos auth login -i admin               # Short form
atmos auth login --provider company-sso # Direct provider auth (for auto_provision_identities)
```

### Notes

- Prints provider, identity, account, region, and expiration on success.
- For AWS SSO, a device verification code is displayed in the terminal (not an MFA token).
- For Azure device code authentication, a verification code and URL are shown for browser auth.
- Automatically triggers ECR integrations with `auto_provision: true`.
- When no identities are configured and `--provider` is omitted, Atmos auto-selects a single provider
  or prompts for selection if multiple providers exist.

---

## atmos auth whoami

Display the current authentication context: effective identity, credentials, account, region, and expiration.

```shell
atmos auth whoami [--identity <name>] [--output json]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Identity to inspect |
| `--output` | `-o` | Output format: `json` (default is human-readable) |

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `ATMOS_IDENTITY` | Default identity |
| `ATMOS_AUTH_WHOAMI_OUTPUT` | Set to `json` for default JSON output |

### Examples

```shell
atmos auth whoami                       # Default identity, human-readable
atmos auth whoami --identity dev-admin  # Specific identity
atmos auth whoami --output json         # JSON output (redacts home directory from paths)
atmos auth whoami -i                    # Interactive selector
```

---

## atmos auth validate

Validate the `auth` configuration in `atmos.yaml` for syntax errors, missing required fields, circular
dependency chains, and logical inconsistencies.

```shell
atmos auth validate [--verbose]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable verbose validation output |

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `ATMOS_AUTH_VALIDATE_VERBOSE` | Set to `true` to enable verbose by default |

### Examples

```shell
atmos auth validate                     # Basic validation
atmos auth validate --verbose           # Verbose output with details
```

---

## atmos auth shell

Launch an interactive shell with all cloud credentials pre-configured. The shell inherits your `$SHELL`
preference and supports custom arguments. Use for interactive sessions with multiple commands.

```shell
atmos auth shell [--identity <name>] [--shell <path>] [-- <shell-args>...]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Identity to use for authentication |
| `--shell` | | Shell program (defaults to `$SHELL`, then `bash`, then `sh`; `cmd.exe` on Windows) |

### Arguments

| Argument | Description |
|----------|-------------|
| `shell-args...` | Optional shell arguments after `--` (default: `-l` for login shell) |

### Environment Variables Set in Shell

| Variable | Description |
|----------|-------------|
| `ATMOS_IDENTITY` | Name of the authenticated identity |
| `ATMOS_SHLVL` | Shell nesting level (increments for nested shells) |
| `AWS_SHARED_CREDENTIALS_FILE` | Path to Atmos-managed credentials file (AWS) |
| `AWS_CONFIG_FILE` | Path to Atmos-managed config file (AWS) |
| `AWS_PROFILE` | Profile name for the identity (AWS) |

### Examples

```shell
atmos auth shell                                        # Default shell, default identity
atmos auth shell --identity prod-admin                  # Specific identity
atmos auth shell --shell /bin/zsh                       # Override shell
atmos auth shell -- -c "env | grep AWS"                 # Pass shell args
atmos auth shell -- --norc                              # Skip shell config loading
atmos auth shell --identity staging --shell /bin/bash -- -c "terraform plan"
```

### Notes

- Type `exit` or press `Ctrl+D` to leave the shell.
- Credentials are written to managed config files (XDG-compliant), not exposed directly as env vars.
- Environment variables from authentication take precedence over existing values.

---

## atmos auth exec

Run a single command with identity credentials injected into the environment. Use for one-off commands
or automation where shell isolation is not needed.

```shell
atmos auth exec [--identity <name>] -- <command> [args...]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Identity to use for authentication |

### Arguments

| Argument | Description |
|----------|-------------|
| `command` | Program to execute with auth env vars set |
| `args...` | Arguments passed through to the command |

### Examples

```shell
# AWS examples
atmos auth exec -- terraform plan -var-file=env.tfvars
atmos auth exec --identity prod-admin -- aws sts get-caller-identity
atmos auth exec -- env | grep AWS

# Azure examples
atmos auth exec --identity azure-dev -- az group list
atmos auth exec --identity azure-prod -- terraform plan -var-file=azure.tfvars
atmos auth exec -- env | grep -E '^(AZURE_|ARM_)'

# CI/CD examples
atmos auth exec --identity azure-prod -- terraform apply -auto-approve
```

### Notes

- `--` is required to separate Atmos flags from the subcommand.
- The command inherits all Atmos-configured credentials and environment variables.

---

## atmos auth env

Output credential environment variables for shell evaluation. Does not perform authentication by default.

```shell
atmos auth env [--identity <name>] [--format bash|json|dotenv] [--login]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Identity to use |
| `--format` | `-f` | Output format: `bash` (default), `json`, `dotenv` |
| `--login` | | Trigger authentication if credentials are missing or expired |

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `ATMOS_IDENTITY` | Default identity |
| `ATMOS_AUTH_ENV_FORMAT` | Default output format |

### Output Variables (AWS)

```bash
AWS_SHARED_CREDENTIALS_FILE  # Path to Atmos-managed credentials file
AWS_CONFIG_FILE              # Path to Atmos-managed config file
AWS_PROFILE                  # Profile name for the identity
AWS_REGION                   # Default region (if configured)
```

### Output Variables (Azure)

```bash
AZURE_SUBSCRIPTION_ID        # Azure subscription ID
AZURE_TENANT_ID              # Azure AD tenant ID
ARM_SUBSCRIPTION_ID          # Terraform provider subscription ID
ARM_TENANT_ID                # Terraform provider tenant ID
ARM_USE_OIDC                 # "true" for OIDC authentication
ARM_USE_CLI                  # "true" for CLI/device-code authentication
ARM_CLIENT_ID                # Azure AD application (client) ID for OIDC
```

### Examples

```shell
# Set environment in current shell
eval $(atmos auth env)
eval $(atmos auth env --identity prod-admin)

# JSON output for scripting
atmos auth env --identity prod-admin --format json

# Dotenv format
atmos auth env --format dotenv

# Auto-login if credentials missing
atmos auth env --login
eval $(atmos auth env --identity prod-admin --login)

# PowerShell
$envVars = atmos auth env --format json | ConvertFrom-Json
$envVars.PSObject.Properties | ForEach-Object { Set-Item -Path "Env:$($_.Name)" -Value $_.Value }
```

### Shell Profile Integration

Safe to add to `~/.bashrc` or `~/.zshrc` -- does not trigger login prompts:

```bash
eval $(atmos auth env)
```

---

## atmos auth console

Open cloud provider web console in your default browser using authenticated credentials.

```shell
atmos auth console [--identity <name>] [--destination <url-or-alias>] [--duration <duration>]
                   [--issuer <name>] [--print-only] [--no-open]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Identity to use for console access |
| `--destination` | | Console page URL or AWS service alias (e.g., `s3`, `ec2`, `lambda`) |
| `--duration` | | Console session duration (Go format, e.g., `4h`; max 12h for AWS) |
| `--issuer` | | Identifier in console URL (default: `atmos`; AWS only) |
| `--print-only` | | Print URL to stdout without opening browser |
| `--no-open` | | Display URL but do not open browser |

### Provider Support

| Provider | Status |
|----------|--------|
| AWS (IAM Identity Center) | Supported |
| AWS (SAML) | Supported |
| Azure | Supported (opens Azure Portal) |
| GCP | Planned |

### Examples

```shell
atmos auth console                                      # Default identity, main console
atmos auth console --identity prod-admin                # Specific identity
atmos auth console --destination s3                     # AWS S3 console (alias)
atmos auth console --destination ec2 --duration 4h      # EC2 with custom duration
atmos auth console --destination https://console.aws.amazon.com/cloudformation
atmos auth console --print-only | pbcopy                # Copy URL to clipboard (macOS)
atmos auth console --no-open                            # Display URL only
atmos auth console --issuer devops-team --duration 2h   # Custom issuer
```

### Notes

- AWS console signin tokens are valid for 15 minutes (to click the link). Console session duration is separate.
- 100+ AWS service aliases supported: `s3`, `ec2`, `lambda`, `dynamodb`, `rds`, `vpc`, `iam`, `eks`, etc.
- Requires temporary credentials with session token (not permanent IAM user credentials).

---

## atmos auth list

List all configured authentication providers and identities with their relationships and chains.

```shell
atmos auth list [--format <format>] [--providers [names]] [--identities [names]]
```

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--format` | `-f` | Output format: `table` (default), `tree`, `json`, `yaml`, `graphviz`/`dot`, `mermaid`, `markdown`/`md` |
| `--providers` | | Show only providers (optionally filter by comma-separated names) |
| `--identities` | | Show only identities (optionally filter by comma-separated names) |

`--providers` and `--identities` are mutually exclusive.

### Examples

```shell
atmos auth list                                 # Table format (default)
atmos auth list --providers                     # Providers only
atmos auth list --providers=aws-sso,okta        # Specific providers
atmos auth list --identities                    # Identities only
atmos auth list --identities=admin,developer    # Specific identities
atmos auth list --format tree                   # Hierarchical tree view
atmos auth list --format json                   # JSON for programmatic access
atmos auth list --format yaml                   # YAML output
atmos auth list --format graphviz > auth.dot    # Graphviz DOT format
atmos auth list --format mermaid                # Mermaid diagram syntax
atmos auth list --format markdown > auth.md     # Markdown with embedded Mermaid
```

### Visualization

```shell
# Generate PNG diagram
atmos auth list --format graphviz | dot -Tpng > auth.png

# Generate SVG
atmos auth list --format graphviz | dot -Tsvg > auth.svg
```

### Table Columns

**Providers:** NAME, KIND, REGION, START URL/URL, DEFAULT.
**Identities:** NAME, KIND, VIA PROVIDER, VIA IDENTITY, DEFAULT, ALIAS.

---

## atmos auth ecr-login

Login to AWS Elastic Container Registry using integrations or explicit registry URLs. Writes Docker
credentials to `~/.docker/config.json`.

```shell
atmos auth ecr-login [integration] [--identity <name>] [--registry <url>]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `integration` | Named integration from `auth.integrations` (e.g., `dev/ecr/primary`) |

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Execute all ECR integrations linked to this identity |
| `--registry` | `-r` | Explicit registry URL(s) for ad-hoc login (can be repeated) |

### Examples

```shell
# Named integration
atmos auth ecr-login dev/ecr/primary

# All integrations for an identity
atmos auth ecr-login --identity dev-admin

# Explicit registry URL
atmos auth ecr-login --registry 123456789012.dkr.ecr.us-east-1.amazonaws.com

# Multiple explicit registries
atmos auth ecr-login \
  --registry 123456789012.dkr.ecr.us-east-1.amazonaws.com \
  --registry 987654321098.dkr.ecr.us-west-2.amazonaws.com
```

### Notes

- ECR tokens expire after approximately 12 hours (AWS-enforced).
- Requires IAM permission: `ecr:GetAuthorizationToken`.
- Only private ECR registries are supported (not ECR Public or China/GovCloud).
- Respects `DOCKER_CONFIG` environment variable for credential file location.

---

## atmos auth logout

Remove locally cached credentials and session data. Preserves keychain credentials by default for
faster re-authentication.

```shell
atmos auth logout [identity] [--identity <name>] [--all] [--provider <name>]
                  [--keychain] [--force] [--dry-run]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `identity` | Name of identity to logout from (alternative to `--identity` flag) |

### Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--identity` | `-i` | Identity to logout from |
| `--all` | | Logout from all identities and providers |
| `--provider` | | Logout from a specific provider (clears all its identities) |
| `--keychain` | | Also delete credentials from system keychain (destructive) |
| `--force` | | Skip interactive confirmation prompts (for CI/CD with `--keychain`) |
| `--dry-run` | | Preview what would be removed without deleting |

### What Gets Removed

| Command | Keychain | Session Data | Config Files |
|---------|----------|-------------|--------------|
| `logout <identity>` | Preserved | Cleared | Identity profile removed |
| `logout <identity> --keychain` | Deleted | Cleared | Identity profile removed |
| `logout --provider <name>` | Preserved | Cleared | Entire provider directory |
| `logout --all` | Preserved | Cleared | All profiles removed |
| Add `--keychain` to any | Deleted | Cleared | Same as without |

### Examples

```shell
atmos auth logout dev-admin                     # Logout specific identity (safe)
atmos auth logout --all                         # Logout all (preserves keychain)
atmos auth logout --provider aws-sso            # Logout all identities for provider
atmos auth logout dev-admin --keychain          # Delete keychain creds (interactive confirm)
atmos auth logout dev-admin --keychain --force  # Delete keychain creds (no confirm, for CI/CD)
atmos auth logout --all --dry-run               # Preview what would be removed
atmos auth logout                               # Interactive mode (no arguments)
```

### Notes

- Browser sessions with identity providers remain active -- sign out from IdP separately.
- Uses best-effort cleanup: continues even if individual steps fail.
- Exit code 0 as long as at least one credential was removed.
- All logout operations are logged for security auditing (use `ATMOS_LOGS_LEVEL=Debug`).

---

## Debugging Authentication

### Enable Debug Logging

```shell
# Verbose CLI output
atmos auth validate --verbose

# Set log level
ATMOS_LOGS_LEVEL=Debug atmos auth whoami

# Write auth logs to file
# In atmos.yaml:
auth:
  logs:
    level: Debug
    file: /tmp/atmos-auth.log
```

### Common Troubleshooting Commands

```shell
# Validate configuration
atmos auth validate --verbose

# Check current status
atmos auth whoami

# Re-authenticate
atmos auth login --identity <name>

# Verify assumed role
atmos auth exec --identity <name> -- aws sts get-caller-identity

# Check exported environment
atmos auth env --identity <name>
atmos auth exec --identity <name> -- env | grep AWS
```
