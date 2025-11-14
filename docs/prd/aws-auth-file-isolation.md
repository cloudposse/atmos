# AWS Authentication File Isolation PRD

## Executive Summary

This document describes how AWS authentication implements the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md). The AWS implementation serves as the reference implementation for all other cloud providers.

**Status:** ✅ **Implemented** - This PRD documents the existing AWS implementation.

**Key Achievement:** AWS successfully implements XDG-compliant credential isolation with physically separate configuration files, making it **provably impossible** to accidentally use the wrong customer's credentials when working with multiple client environments.

## Critical Use Case: Multi-Customer Cloud Operations

When working with cloud infrastructure at scale (especially as cloud practitioners managing multiple customer environments), you need **absolute certainty** that you never accidentally use the wrong customer's credentials. This is not just a convenience feature—it's a **critical security and operational requirement**.

**The Problem:**
- Cloud Posse manages infrastructure for multiple customers simultaneously
- Each customer has separate AWS accounts, Azure subscriptions, etc.
- Accidentally using Customer A's credentials when working on Customer B's infrastructure can cause:
  - **Security incidents**: Deploying resources to the wrong account
  - **Compliance violations**: Cross-contamination of customer data
  - **Operational disasters**: Destroying the wrong customer's infrastructure
  - **Financial liability**: Billing the wrong customer or unexpected costs

**Why Standard AWS CLI Configuration Isn't Enough:**
Most cloud tooling (AWS CLI, Terraform, etc.) uses a single shared configuration file (`~/.aws/credentials`) with multiple profiles. This creates risks:
- ❌ Easy to forget to switch profiles between customers
- ❌ Hard to verify which customer's credentials are active
- ❌ Credentials for all customers exist in one file (potential for misuse)
- ❌ No physical separation—just logical profiles

**Atmos Solution: Physical File Isolation**
Atmos provides **physically separate credential files** for each identity/customer:
```
~/.config/atmos/aws/customer-a-prod/credentials   # Customer A production
~/.config/atmos/aws/customer-b-prod/credentials   # Customer B production
~/.config/atmos/aws/internal-dev/credentials      # Internal development
```

**Benefits:**
- ✅ **Provably impossible** to use wrong credentials—files are physically separate
- ✅ **Clean logout** deletes entire directory—no traces left behind
- ✅ **Shell session scoping**—each terminal uses different credentials
- ✅ **Audit trail**—easy to see which customer credentials exist: `ls ~/.config/atmos/aws/`
- ✅ **Zero risk of cross-contamination**—customer environments completely isolated

**This is the level of segmentation required when doing cloud configuration at scale.** Atmos brings this best practice to all cloud providers.

## Implementation Overview

### Directory Structure

```
~/.config/atmos/aws/           # XDG_CONFIG_HOME/atmos/aws (base directory)
├── aws-sso/                   # Provider name (from atmos.yaml)
│   ├── credentials            # AWS credentials file (INI format)
│   └── config                 # AWS config file (INI format)
└── aws-user/                  # Different provider (multiple providers can coexist)
    ├── credentials
    └── config
```

**Platform-specific paths:**
- Linux/macOS: `~/.config/atmos/aws/` (XDG default)
- Windows: `%APPDATA%\atmos\aws\`

### File Formats

**Credentials file (`credentials`):**
```ini
[my-sso-identity]
aws_access_key_id = ASIA...
aws_secret_access_key = ...
aws_session_token = ...
```

**Config file (`config`):**
```ini
[profile my-sso-identity]
region = us-east-1
output = json
```

## Environment Variable Strategy

### Primary Isolation Variables

**`AWS_SHARED_CREDENTIALS_FILE`** - Absolute path to Atmos-managed credentials file
- Example: `/home/user/.config/atmos/aws/aws-sso/credentials`
- Purpose: Overrides default `~/.aws/credentials`
- Precedence: Highest in AWS SDK credential resolution chain

**`AWS_CONFIG_FILE`** - Absolute path to Atmos-managed config file
- Example: `/home/user/.config/atmos/aws/aws-sso/config`
- Purpose: Overrides default `~/.aws/config`
- Precedence: Highest in AWS SDK config resolution chain

### Configuration Variables

**`AWS_PROFILE`** - Identity name as profile name
- Example: `my-sso-identity`
- Purpose: Selects which profile section to use within the credentials/config files
- Note: Profile name matches identity name for consistency

**`AWS_REGION`** - Region override (optional)
- Example: `us-east-1`
- Purpose: Overrides region from config file
- Supports component-level overrides via stack inheritance

### Conflicting Variables Cleared

The following variables are **cleared** to prevent conflicts:

- `AWS_ACCESS_KEY_ID` - Would override file-based credentials
- `AWS_SECRET_ACCESS_KEY` - Would override file-based credentials
- `AWS_SESSION_TOKEN` - Would override file-based session token

**Why clear these?** These environment variables have higher precedence than credential files in AWS SDK. If present, they would cause SDK to ignore Atmos-managed files.

### Security Enhancement

**`AWS_EC2_METADATA_DISABLED=true`**
- Purpose: Prevents fallback to EC2 instance metadata service (IMDS)
- Important: Ensures AWS SDK only uses Atmos-managed credentials, not ambient environment
- Security: Prevents credential leakage from EC2 instance roles when running inside EC2

## Code Architecture

### File Manager (`pkg/auth/cloud/aws/files.go`)

**Responsibilities:**
1. Construct file paths for credentials and config files
2. **Read and write INI files** preserving profile structure and comments
3. **Manage multiple identities** within provider-specific INI files
4. Clean up on logout (remove specific identity or entire provider)
5. **Prevent concurrent modification conflicts** via file locking

**Key Type:**
```go
type AWSFileManager struct {
    baseDir string  // e.g., ~/.config/atmos/aws
}
```

**Key Methods:**
- `NewAWSFileManager(basePath string)` - Creates manager with XDG default or custom path
- `GetCredentialsPath(providerName)` - Returns `~/.config/atmos/aws/{provider}/credentials`
- `GetConfigPath(providerName)` - Returns `~/.config/atmos/aws/{provider}/config`
- `WriteCredentials()` - **Writes/updates identity profile in credentials INI file** with file locking
- `WriteConfig()` - **Writes/updates identity profile in config INI file** with file locking
- `DeleteIdentity()` - **Removes specific identity profile from INI files** (preserves other identities)
- `Cleanup()` - **Removes entire provider directory** (all identities for that provider)
- `LoadINIFile()` - Loads INI file preserving comments (critical for expiration metadata)

**INI File Structure:**

Each provider has its own credentials and config files that can contain multiple identity profiles:

```ini
# ~/.config/atmos/aws/aws-sso/credentials
[customer-a-prod]
aws_access_key_id = ASIA...
aws_secret_access_key = ...
aws_session_token = ...
# Expiration: 2025-01-15T10:30:00Z

[customer-b-prod]
aws_access_key_id = ASIA...
aws_secret_access_key = ...
aws_session_token = ...
# Expiration: 2025-01-15T11:00:00Z
```

```ini
# ~/.config/atmos/aws/aws-sso/config
[profile customer-a-prod]
region = us-east-1
output = json

[profile customer-b-prod]
region = us-west-2
output = json
```

**File Locking:**
Uses `github.com/gofrs/flock` to prevent concurrent modification conflicts:
- Acquires exclusive lock before reading/writing INI files
- Timeout: 10 seconds with 50ms retry interval
- Lock file: `{credentials|config}.lock` adjacent to actual file
- Critical for multi-process safety when multiple terminals authenticate simultaneously

**Comment Preservation:**
Uses `gopkg.in/ini.v1` with `IgnoreInlineComment: false` to preserve:
- Expiration timestamps in credentials file (used for token refresh logic)
- User-added comments and metadata
- Profile documentation

### Setup Functions (`pkg/auth/cloud/aws/setup.go`)

**Responsibilities:**
1. Write credential files during authentication (`SetupFiles`)
2. Populate AuthContext with file paths (`SetAuthContext`)
3. Derive environment variables from AuthContext (`SetEnvironmentVariables`)

**`SetupFiles` Flow:**
1. Create file manager with configured or default base path
2. Ensure provider directory exists
3. Write credentials to AWS INI format
4. Write config with region setting

**`SetAuthContext` Flow:**
1. Create file manager
2. Get file paths from manager
3. Populate `AuthContext.AWS` with:
   - `CredentialsFile` - Absolute path to credentials file
   - `ConfigFile` - Absolute path to config file
   - `Profile` - Identity name
   - `Region` - From credentials or component override

**`SetEnvironmentVariables` Flow:**
1. Extract AWS auth context
2. Call `PrepareEnvironment` to build env var map
3. Replace `ComponentEnvSection` with prepared environment

### Environment Preparation (`pkg/auth/cloud/aws/env.go`)

**`PrepareEnvironment` Function:**

Logic flow:
1. Create copy of input environment (doesn't mutate input)
2. Clear conflicting AWS credential env vars
3. Set `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, `AWS_PROFILE`
4. Set `AWS_REGION` if provided
5. Set `AWS_EC2_METADATA_DISABLED=true`
6. Return new map

**Key Feature:** Returns NEW map instead of mutating input for safety and testability.

### Auth Context Schema (`pkg/schema/schema.go`)

```go
type AWSAuthContext struct {
    // CredentialsFile is the absolute path to the Atmos-managed AWS credentials file.
    // Maps to AWS_SHARED_CREDENTIALS_FILE environment variable.
    // Example: /home/user/.config/atmos/aws/aws-sso/credentials
    CredentialsFile string `json:"credentials_file" yaml:"credentials_file"`

    // ConfigFile is the absolute path to the Atmos-managed AWS config file.
    // Maps to AWS_CONFIG_FILE environment variable.
    // Example: /home/user/.config/atmos/aws/aws-sso/config
    ConfigFile string `json:"config_file" yaml:"config_file"`

    // Profile is the AWS profile name (identity name).
    // Maps to AWS_PROFILE environment variable.
    Profile string `json:"profile" yaml:"profile"`

    // Region is the AWS region.
    // Maps to AWS_REGION environment variable (optional override).
    Region string `json:"region,omitempty" yaml:"region,omitempty"`
}
```

## Why AWS Implementation Works

### 1. Physical File Separation = Provable Security

**Physically separate credential files per provider:**
```
~/.config/atmos/aws/
├── customer-a-prod/
│   ├── credentials    # ONLY Customer A credentials
│   └── config         # ONLY Customer A config
├── customer-b-prod/
│   ├── credentials    # ONLY Customer B credentials
│   └── config         # ONLY Customer B config
└── internal-dev/
    ├── credentials    # ONLY internal credentials
    └── config         # ONLY internal config
```

**Why this matters:**
- ✅ **Impossible to accidentally use wrong customer's credentials**—files are physically separate
- ✅ **Operating system enforces isolation**—can't read Customer A's file when using Customer B
- ✅ **Clear visual audit**—`ls ~/.config/atmos/aws/` shows exactly which customers you have access to
- ✅ **Granular cleanup**—delete specific customer directory to remove all traces
- ✅ **No shared state**—zero risk of credential cross-contamination

**Contrast with standard AWS CLI approach:**
```
~/.aws/credentials    # ALL customers in ONE file (risky!)
[customer-a-prod]
aws_access_key_id = ASIA...

[customer-b-prod]
aws_access_key_id = ASIA...

[internal-dev]
aws_access_key_id = ASIA...
```

Problems with single-file approach:
- ❌ All customer credentials in one file (potential for accidental misuse)
- ❌ Must remember to set `AWS_PROFILE` correctly (easy to forget)
- ❌ Hard to audit which customer's credentials are being used
- ❌ Deleting one customer's credentials requires parsing INI file

### 2. Complete Isolation from Developer's Personal Accounts

**Protecting Developer's Hobby Accounts:**

Most developers have personal AWS accounts for hobby projects, learning, or experimentation. These are manually configured with `aws configure` and stored in `~/.aws/`. Atmos must never interfere with these personal accounts.

**Separation:**
- **Personal hobby accounts**: `~/.aws/credentials` - Developer's manually configured credentials
- **Atmos-managed work accounts**: `~/.config/atmos/aws/` - Enterprise/customer credentials provisioned by Atmos
- **Complete isolation**: Atmos operations never touch `~/.aws/` directory

**Why This Matters:**
- ✅ Developer can use personal AWS for side projects without affecting work
- ✅ `atmos auth logout` never deletes personal hobby account credentials
- ✅ Personal `aws configure` settings remain intact
- ✅ No risk of Atmos overwriting manually configured profiles

**Example:**
```bash
# Developer's personal AWS hobby account (stays untouched)
$ aws configure
AWS Access Key ID: PERSONAL_KEY_FOR_HOBBY_PROJECT
AWS Secret Access Key: PERSONAL_SECRET
Default region name: us-west-2
# Stored in: ~/.aws/credentials (never modified by Atmos)

# Work: Atmos enterprise/customer authentication
$ atmos auth login customer-a-sso
# Stored in: ~/.config/atmos/aws/customer-a-sso/ (completely isolated)

# Personal hobby project still works
$ aws s3 ls  # Uses ~/.aws/credentials (personal account)

# Work project uses Atmos credentials
$ atmos terraform plan  # Uses ~/.config/atmos/aws/customer-a-sso/
```

### 2. Clean Logout

**Single directory removal:**
```bash
# Logout removes entire provider directory
$ atmos auth logout aws-sso

# Internally: rm -rf ~/.config/atmos/aws/aws-sso/
# Result: All Atmos-managed AWS SSO credentials removed
# Impact: User's ~/.aws/ directory completely untouched
```

**No INI file parsing required:**
- Don't need to parse INI files to remove specific sections
- Don't risk corrupting user's personal AWS configuration
- Simple, atomic operation

### 3. Environment Variable Precedence

**AWS SDK credential resolution order:**
1. **Environment variables** (`AWS_ACCESS_KEY_ID`, etc.) - Cleared by Atmos
2. **Credentials file** via `AWS_SHARED_CREDENTIALS_FILE` - **Set by Atmos** ✅
3. **Config file** via `AWS_CONFIG_FILE` - **Set by Atmos** ✅
4. Default credentials file (`~/.aws/credentials`) - Not used
5. Default config file (`~/.aws/config`) - Not used
6. EC2 instance metadata - Disabled by `AWS_EC2_METADATA_DISABLED=true`

**Result:** SDK always uses Atmos-managed credentials, never user's manual config or ambient environment.

### 4. Multi-Provider Support

**Multiple AWS providers can coexist:**
```
~/.config/atmos/aws/
├── aws-sso/          # SSO provider for production
├── aws-user/         # IAM user provider for development
└── aws-cross-account/ # Cross-account role provider
```

**Each provider:**
- Has independent credentials and config files
- Can use different authentication methods (SSO, IAM user, etc.)
- No conflicts between providers
- Can be active in different shell sessions simultaneously

### 5. Shell Session Scoping

**Environment variables set per shell session:**
```bash
# Terminal 1
$ atmos auth login aws-sso
$ atmos terraform plan  # Uses aws-sso credentials

# Terminal 2 (different shell session)
$ atmos auth login aws-user
$ atmos terraform plan  # Uses aws-user credentials

# No conflict - each shell has independent environment
```

**Benefits:**
- Different identities in different terminals
- No global state or file system locks
- Clean, predictable behavior

## Provider Configuration

```yaml
# atmos.yaml
auth:
  providers:
    aws-sso:
      kind: aws/sso
      spec:
        sso_start_url: "https://example.awsapps.com/start"
        sso_region: "us-east-1"
        account_id: "123456789012"
        role_name: "PowerUserAccess"

        files:
          # Optional: Override default XDG config directory
          # If not specified, uses ~/.config/atmos/aws
          base_path: ""  # Empty = use XDG default (recommended)
```

## Testing

### Unit Tests

**File Manager Tests (`pkg/auth/cloud/aws/files_test.go`):**
- ✅ Test XDG default path resolution
- ✅ Test custom base_path configuration
- ✅ Test credentials file writing with INI format
- ✅ Test config file writing with INI format
- ✅ Test file locking for concurrent access
- ✅ Test cleanup operations

**Setup Tests (`pkg/auth/cloud/aws/setup_test.go`):**
- ✅ Test AuthContext population
- ✅ Test file creation during setup
- ✅ Test environment variable derivation
- ✅ Test component-level region overrides

**Environment Tests (`pkg/auth/cloud/aws/env_test.go`):**
- ✅ Test primary isolation variables are set
- ✅ Test conflicting variables are cleared
- ✅ Test IMDS is disabled
- ✅ Test input is not mutated

### Integration Tests

- ✅ Full auth flow (SSO login → file writing → environment setup)
- ✅ Multi-provider coexistence
- ✅ Cleanup removes all files
- ✅ No modification of user's `~/.aws/` directory

## Security Considerations

### File Permissions

**Directory permissions:** `0o700` (owner-only access)
```bash
$ ls -ld ~/.config/atmos/aws/aws-sso
drwx------ 2 user user 4096 ... ~/.config/atmos/aws/aws-sso/
```

**File permissions:** `0o600` (owner-only read/write)
```bash
$ ls -l ~/.config/atmos/aws/aws-sso/
-rw------- 1 user user ... credentials
-rw------- 1 user user ... config
```

### Attack Surface Reduction

**Before (without isolation):**
- ❌ Mixed Atmos + user credentials in `~/.aws/credentials`
- ❌ Hard to audit credential source
- ❌ Logout affects user's setup

**After (with isolation):**
- ✅ Clear separation: Atmos credentials in `~/.config/atmos/aws/`
- ✅ Easy audit: `ls ~/.config/atmos/aws/` shows all Atmos providers
- ✅ Clean logout: Delete provider directory

### Credential Isolation Benefits

1. **No cross-contamination**: User's `aws configure` doesn't affect Atmos
2. **Clear audit trail**: All Atmos-managed credentials in one tree
3. **Secure deletion**: Remove entire directory for complete logout
4. **Shell session scoping**: Environment isolation prevents identity leakage

## Migration from Legacy Paths

AWS authentication has always used XDG paths since initial implementation, so no migration is needed.

**Historical note:** Early versions used `~/.aws/atmos/` but were updated to `~/.config/atmos/aws/` for XDG compliance before stable release.

## Adherence to Universal Pattern

This implementation follows the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md):

| Requirement | AWS Implementation | Status |
|-------------|-------------------|--------|
| XDG Compliance | `~/.config/atmos/aws/` | ✅ |
| Provider Scoping | `{provider-name}/` subdirectories | ✅ |
| File Permissions | `0o700` dirs, `0o600` files | ✅ |
| Primary Isolation Vars | `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE` | ✅ |
| Credential Clearing | Clears `AWS_ACCESS_KEY_ID`, etc. | ✅ |
| File Manager | `pkg/auth/cloud/aws/files.go` | ✅ |
| Setup Functions | `pkg/auth/cloud/aws/setup.go` | ✅ |
| Environment Prep | `pkg/auth/cloud/aws/env.go` | ✅ |
| Auth Context | `AWSAuthContext` in `pkg/schema/schema.go` | ✅ |
| Test Coverage | >80% coverage | ✅ |

## Related Documents

1. **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** - Canonical pattern all providers follow
2. **[XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)** - XDG compliance patterns
3. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)** - AuthContext design and usage
4. **[Azure Authentication File Isolation](./azure-auth-file-isolation.md)** - Azure implementation of pattern

## Success Metrics

AWS authentication successfully implements the universal pattern:

1. ✅ **XDG Compliance**: Uses `~/.config/atmos/aws` on all platforms
2. ✅ **User Isolation**: User's `~/.aws/` directory never modified
3. ✅ **Clean Logout**: Deleting provider directory removes all traces
4. ✅ **Environment Control**: SDK uses only Atmos credentials via environment variables
5. ✅ **Multi-Provider**: Multiple AWS providers coexist without conflicts
6. ✅ **Test Coverage**: >80% coverage for all AWS auth code
7. ✅ **Documentation**: Complete PRD documenting implementation

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2025-01-XX | 1.0 | Initial AWS implementation PRD created to document existing implementation |
