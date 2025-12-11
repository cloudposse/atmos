# ECR Authentication PRD

## Executive Summary

Add ECR authentication support to Atmos using a **hybrid approach**:
1. **PostAuthenticate hook** - Automatic ECR login as opt-in side effect on existing AWS identities
2. **Standalone command** - `atmos auth ecr-login` for ad-hoc use with current AWS credentials

This approach avoids the management overhead of a separate ECR identity type while providing flexibility for different workflows.

**Key Design Decision:** ECR login is implemented as an opt-in extension to existing AWS identities (via `ecr_login: true` in principal config) rather than a separate identity type. This mirrors the existing workflow where users set `AWS_PROFILE` and then login to ECR.

## Problem Statement

### Current Limitations

Users cannot easily authenticate to AWS ECR using Atmos-managed credentials. To pull or push images, they must:

1. Manually run `aws ecr get-login-password` to retrieve authorization tokens
2. Pipe the output to `docker login` with the correct registry URL
3. Repeat this process every 12 hours (ECR token expiration)
4. Configure `DOCKER_CONFIG` environment variable to use isolated credentials
5. Manage credential refresh across multiple registries and accounts

This creates friction for common workflows:
- Local development requiring ECR image pulls
- GitHub Actions workflows needing ECR access
- Devcontainer builds using private base images

### User Impact

**Current Experience:**
```bash
# User authenticates with Atmos for AWS access
$ atmos auth login aws-dev

# User needs to pull ECR image... must do this manually:
$ aws ecr get-login-password --region us-east-2 | \
  docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-2.amazonaws.com

# Token expires after 12 hours, must repeat
# Must also configure DOCKER_CONFIG for isolation
```

**Desired Experience (Option A - Automatic):**
```bash
# Configure ecr_login: true in identity config
$ atmos auth login aws-dev
✓ Authenticated as arn:aws:sts::123456789012:assumed-role/DevRole/user
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

# Docker commands work automatically
$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
# Success!
```

**Desired Experience (Option B - Explicit):**
```bash
# Get AWS credentials however you want
$ export AWS_PROFILE=dev

# Explicitly login to ECR
$ atmos auth ecr-login --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
# Success!
```

## Design Goals

1. **Minimal Management Overhead**: No separate ECR identity to configure and manage
2. **Single Login Experience**: `atmos auth login` can do both AWS auth and ECR login
3. **Explicit Control**: Standalone command for users who prefer explicit ECR login
4. **Non-Blocking Errors**: ECR failures don't block AWS authentication
5. **Isolated Docker Config**: Use Atmos-managed Docker config file to avoid polluting user's default config
6. **XDG Compliance**: Use `pkg/xdg` for config paths
7. **Multi-Registry Support**: Support multiple ECR registries

## Technical Specification

### 1. Configuration

#### Option A: Identity-Level ECR Login (PostAuthenticate hook)

Add ECR configuration to existing AWS identity's `principal` map:

```yaml
auth:
  identities:
    dev-admin:
      kind: aws/permission-set
      via:
        provider: aws-sso
      principal:
        name: AdministratorAccess
        account:
          name: dev
        # NEW: Opt-in ECR login after AWS authentication
        ecr_login: true
        ecr_registries:                    # Optional: specific registries
          - account_id: "123456789012"
            region: "us-east-2"
          - account_id: "987654321098"
            region: "us-west-2"
```

**Configuration Options:**

| Field | Required | Description |
|-------|----------|-------------|
| `principal.ecr_login` | No | Enable automatic ECR login (default: false) |
| `principal.ecr_registries` | No | List of registries; if omitted, uses current account/region |
| `principal.ecr_registries[].account_id` | Yes* | AWS account ID for registry |
| `principal.ecr_registries[].region` | Yes* | AWS region for registry |

*Required if `ecr_registries` is specified.

#### Option B: Standalone Command (Ad-hoc)

```bash
# Use current AWS credentials (from profile, env, or atmos auth)
atmos auth ecr-login --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com

# Auto-detect from current account/region
atmos auth ecr-login

# Multiple registries
atmos auth ecr-login \
  --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com \
  --registry 987654321098.dkr.ecr.us-west-2.amazonaws.com
```

### 2. Authentication Flow

#### Option A: PostAuthenticate Hook Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. User Executes Command                                        │
│    $ atmos auth login dev-admin                                 │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. Normal AWS Authentication                                    │
│    - SSO login / assume role / IAM user auth                    │
│    - Obtain AWS credentials                                     │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. PostAuthenticate Hook                                        │
│    - SetupFiles() - write AWS credential files                  │
│    - SetAuthContext() - populate in-process auth                │
│    - SetEnvironmentVariables() - configure subprocess env       │
│    - NEW: PerformECRLogin() if ecr_login: true                  │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. ECR Login (if enabled)                                       │
│    - Call ecr:GetAuthorizationToken for each registry           │
│    - Write to Atmos Docker config                               │
│    - Log success/warning (non-fatal errors)                     │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. Return Success                                               │
│    ✓ Authenticated as arn:aws:sts::...                          │
│    ✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com    │
└─────────────────────────────────────────────────────────────────┘
```

#### Option B: Standalone Command Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. User Executes Command                                        │
│    $ atmos auth ecr-login --registry ...                        │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. Load AWS Credentials                                         │
│    - Use default AWS credential chain                           │
│    - AWS_PROFILE, env vars, or atmos auth context               │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. Determine Registries                                         │
│    - Use --registry flags if provided                           │
│    - Otherwise, detect from current account/region              │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. ECR Login                                                    │
│    - Call ecr:GetAuthorizationToken for each registry           │
│    - Write to Atmos Docker config                               │
│    - Set DOCKER_CONFIG environment variable                     │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. Return Success                                               │
│    ✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com    │
└─────────────────────────────────────────────────────────────────┘
```

### 3. Implementation Architecture

#### 3.1 Package Structure

```
pkg/auth/
├── cloud/
│   ├── aws/
│   │   ├── ecr.go              # ECR token fetcher
│   │   └── ecr_login.go        # ECR login helper for PostAuthenticate
│   └── docker/
│       └── config.go           # Docker config.json manager
├── identities/
│   └── aws/
│       ├── user.go             # Modified: add ECR login in PostAuthenticate
│       ├── permission_set.go   # Modified: add ECR login in PostAuthenticate
│       └── assume_role.go      # Modified: add ECR login in PostAuthenticate
└── ecr_login.go                # Standalone command implementation

cmd/auth/
└── ecr_login.go                # atmos auth ecr-login command
```

#### 3.2 Docker Config Manager

**File:** `pkg/auth/cloud/docker/config.go`

```go
import "github.com/cloudposse/atmos/pkg/xdg"

// ConfigManager manages Docker config.json for ECR authentication.
type ConfigManager struct {
    configPath string
}

// NewConfigManager creates a new Docker config manager using XDG paths.
func NewConfigManager() (*ConfigManager, error) {
    // Use pkg/xdg to get the config path, respecting XDG environment variables.
    configDir, err := xdg.GetXDGConfigDir("docker", 0700)
    if err != nil {
        return nil, fmt.Errorf("failed to get docker config directory: %w", err)
    }
    return &ConfigManager{
        configPath: filepath.Join(configDir, "config.json"),
    }, nil
}

// WriteAuth writes ECR authorization to Docker config.
func (m *ConfigManager) WriteAuth(registry string, username string, password string) error

// RemoveAuth removes ECR authorization from Docker config.
func (m *ConfigManager) RemoveAuth(registries ...string) error

// GetConfigDir returns the directory containing the Docker config.
func (m *ConfigManager) GetConfigDir() string

// GetAuthenticatedRegistries returns list of authenticated ECR registries.
func (m *ConfigManager) GetAuthenticatedRegistries() ([]string, error)
```

**Note:** Uses `pkg/xdg.GetXDGConfigDir()` to determine the config path, which respects `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME` environment variables.

**Docker Config Format:**

```json
{
  "auths": {
    "123456789012.dkr.ecr.us-east-2.amazonaws.com": {
      "auth": "QVdTOmV5SjBlWEJsLi4u"
    }
  }
}
```

The `auth` field contains `base64(username:password)` where username is always `AWS` and password is the authorization token from ECR.

#### 3.3 ECR Token Fetcher

**File:** `pkg/auth/cloud/aws/ecr.go`

```go
// ECRAuthResult contains ECR authorization token information.
type ECRAuthResult struct {
    Username   string    // Always "AWS"
    Password   string    // Decoded authorization token
    Registry   string    // e.g., 123456789012.dkr.ecr.us-east-1.amazonaws.com
    ExpiresAt  time.Time // Token expiration time
}

// GetAuthorizationToken retrieves ECR credentials using AWS config.
func GetAuthorizationToken(ctx context.Context, cfg aws.Config, accountID, region string) (*ECRAuthResult, error)

// BuildRegistryURL constructs ECR registry URL from account ID and region.
func BuildRegistryURL(accountID, region string) string

// ParseRegistryURL extracts account ID and region from ECR registry URL.
func ParseRegistryURL(registryURL string) (accountID, region string, err error)
```

#### 3.4 ECR Login Helper (PostAuthenticate)

**File:** `pkg/auth/cloud/aws/ecr_login.go`

```go
// ECRLoginConfig holds configuration for ECR login.
type ECRLoginConfig struct {
    Enabled    bool
    Registries []ECRRegistry
}

// ECRRegistry represents a single ECR registry configuration.
type ECRRegistry struct {
    AccountID string `mapstructure:"account_id"`
    Region    string `mapstructure:"region"`
}

// ParseECRConfig extracts ECR configuration from identity principal map.
func ParseECRConfig(principal map[string]interface{}) *ECRLoginConfig

// PerformECRLogin executes ECR login for configured registries.
// Called from PostAuthenticate hook when ecr_login: true.
// Errors are non-fatal and logged as warnings.
func PerformECRLogin(ctx context.Context, awsCfg aws.Config, ecrConfig *ECRLoginConfig) error
```

#### 3.5 Modify Existing AWS Identities

**Files to modify:**
- `pkg/auth/identities/aws/user.go`
- `pkg/auth/identities/aws/permission_set.go`
- `pkg/auth/identities/aws/assume_role.go`

In each identity's `PostAuthenticate()` method, add ECR login support:

```go
func (i *userIdentity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
    // Existing code...
    if err := awsCloud.SetupFiles(...); err != nil { /* handle */ }
    if err := awsCloud.SetAuthContext(...); err != nil { /* handle */ }
    if err := awsCloud.SetEnvironmentVariables(...); err != nil { /* handle */ }

    // NEW: Handle ECR login if configured
    ecrConfig := awsCloud.ParseECRConfig(i.config.Principal)
    if ecrConfig.Enabled {
        awsCfg, err := buildAWSConfig(ctx, params.Credentials)
        if err != nil {
            log.Warn("Failed to build AWS config for ECR login", "error", err)
        } else if err := awsCloud.PerformECRLogin(ctx, awsCfg, ecrConfig); err != nil {
            log.Warn("ECR login failed", "error", err)
            // Non-fatal - don't block authentication
        }
    }

    return nil
}
```

#### 3.6 Standalone Command

**File:** `cmd/auth/ecr_login.go`

```go
var ecrLoginCmd = &cobra.Command{
    Use:   "ecr-login",
    Short: "Login to AWS ECR registries",
    Long: `Login to AWS ECR registries using current AWS credentials.

Uses AWS credentials from:
- Current AWS_PROFILE environment variable
- Atmos auth context (if authenticated)
- Default AWS credential chain`,
    RunE: func(cmd *cobra.Command, args []string) error {
        registries, _ := cmd.Flags().GetStringArray("registry")
        return auth.ECRLogin(ctx, registries)
    },
}

func init() {
    ecrLoginCmd.Flags().StringArrayP("registry", "r", nil, "ECR registry URL(s)")
    authCmd.AddCommand(ecrLoginCmd)
}
```

**File:** `pkg/auth/ecr_login.go`

```go
// ECRLogin performs ECR authentication using current AWS credentials.
func ECRLogin(ctx context.Context, registries []string) error {
    // 1. Load AWS config from environment/profile
    awsCfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return fmt.Errorf("failed to load AWS config: %w", err)
    }

    // 2. If no registries specified, use current account
    if len(registries) == 0 {
        accountID, region := getCurrentAccountAndRegion(ctx, awsCfg)
        registries = []string{awsCloud.BuildRegistryURL(accountID, region)}
    }

    // 3. Get tokens and write Docker config
    dockerConfig, err := docker.NewConfigManager()
    if err != nil {
        return err
    }

    for _, registry := range registries {
        accountID, region, err := awsCloud.ParseRegistryURL(registry)
        if err != nil {
            return fmt.Errorf("invalid registry URL %s: %w", registry, err)
        }

        result, err := awsCloud.GetAuthorizationToken(ctx, awsCfg, accountID, region)
        if err != nil {
            return fmt.Errorf("ECR login failed for %s: %w", registry, err)
        }

        if err := dockerConfig.WriteAuth(result.Registry, result.Username, result.Password); err != nil {
            return fmt.Errorf("failed to write Docker config: %w", err)
        }

        ui.Success("ECR login: %s (expires in 12h)", result.Registry)
    }

    // 4. Set DOCKER_CONFIG for current session
    os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir())

    return nil
}
```

### 4. Error Handling

Add sentinel errors to `errors/errors.go`:

```go
// ECR authentication errors.
var (
    ErrECRAuthenticationFailed = errors.New("ECR authentication failed")
    ErrECRTokenExpired         = errors.New("ECR authorization token expired")
    ErrECRRegistryNotFound     = errors.New("ECR registry not found")
)
```

**Error Behavior:**

| Context | Error Behavior |
|---------|----------------|
| PostAuthenticate hook | Non-fatal: log warning, continue AWS auth |
| Standalone command | Fatal: return error to user |

### 5. Environment Variables

When ECR login succeeds, set:

| Variable | Value | Purpose |
|----------|-------|---------|
| `DOCKER_CONFIG` | `~/.config/atmos/docker` | Points Docker to Atmos-managed config |

### 6. File Locking

The Docker config manager uses file locking to prevent concurrent modification:

```go
import "github.com/gofrs/flock"

func (m *ConfigManager) WriteAuth(...) error {
    lock := flock.New(m.configPath + ".lock")
    if err := lock.Lock(); err != nil {
        return err
    }
    defer lock.Unlock()
    // ... write config ...
}
```

## User Experience Examples

### Single Login with Automatic ECR

```bash
# Configure ecr_login: true in identity (see Configuration section)
$ atmos auth login dev-admin
✓ Authenticated as arn:aws:sts::123456789012:assumed-role/DevRole/user
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

# Docker commands work automatically
$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
latest: Pulling from my-app
...
Status: Downloaded newer image for my-app:latest
```

### Explicit ECR Login

```bash
# First, get AWS credentials however you want
$ export AWS_PROFILE=dev
# OR
$ atmos auth login dev-admin

# Then explicitly login to ECR
$ atmos auth ecr-login --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
```

### Auto-Detect Registry

```bash
# Login to ECR in current account/region
$ atmos auth ecr-login
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)
```

### Multi-Registry Login

```yaml
# atmos.yaml
auth:
  identities:
    all-envs:
      kind: aws/permission-set
      via:
        provider: aws-sso
      principal:
        name: DevOpsAccess
        ecr_login: true
        ecr_registries:
          - account_id: "123456789012"
            region: "us-east-2"
          - account_id: "987654321098"
            region: "us-west-2"
```

```bash
$ atmos auth login all-envs
✓ Authenticated as arn:aws:sts::123456789012:assumed-role/DevOpsAccess/user
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)
✓ ECR login: 987654321098.dkr.ecr.us-west-2.amazonaws.com (expires in 12h)
```

### GitHub Actions Integration

```yaml
# .github/workflows/build.yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Configure Atmos Auth
        run: |
          atmos auth login aws-ci
          # ECR login happens automatically if ecr_login: true
          # OR explicitly:
          atmos auth ecr-login

      - name: Build and Push
        run: |
          docker build -t 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:${{ github.sha }} .
          docker push 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:${{ github.sha }}
```

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Add Docker config manager (`pkg/auth/cloud/docker/config.go`)
- [ ] Add ECR token fetcher (`pkg/auth/cloud/aws/ecr.go`)
- [ ] Add ECR login helper (`pkg/auth/cloud/aws/ecr_login.go`)
- [ ] Add sentinel errors (`errors/errors.go`)

### Phase 2: PostAuthenticate Integration
- [ ] Modify `pkg/auth/identities/aws/user.go`
- [ ] Modify `pkg/auth/identities/aws/permission_set.go`
- [ ] Modify `pkg/auth/identities/aws/assume_role.go`

### Phase 3: Standalone Command
- [ ] Add `cmd/auth/ecr_login.go`
- [ ] Add `pkg/auth/ecr_login.go`

### Phase 4: Testing
- [ ] Unit tests for Docker config manager
- [ ] Unit tests for ECR token fetcher (mocked)
- [ ] Unit tests for PostAuthenticate integration
- [ ] Integration tests for `atmos auth ecr-login` command

### Phase 5: Documentation
- [ ] Update `website/docs/cli/commands/auth/login.mdx`
- [ ] Add `website/docs/cli/commands/auth/ecr-login.mdx`
- [ ] Add ECR configuration examples to auth docs

## Success Criteria

1. ✅ `atmos auth login <identity>` with `ecr_login: true` performs ECR login automatically
2. ✅ `atmos auth ecr-login` works with current AWS credentials
3. ✅ Docker commands use Atmos-managed credentials via `DOCKER_CONFIG`
4. ✅ Multi-registry support works correctly
5. ✅ ECR login failures don't block AWS authentication (PostAuthenticate)
6. ✅ Tests achieve >80% coverage
7. ✅ Documentation includes usage examples

## Security Considerations

1. **Credential Isolation**: Docker config stored in Atmos XDG config directory (via `pkg/xdg.GetXDGConfigDir("docker", 0700)`), separate from user's default `~/.docker/config.json`. This respects `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME` environment variables.
2. **File Permissions**: Docker config created with `0600` permissions
3. **Token Lifetime**: ECR tokens expire after 12 hours (AWS-enforced)
4. **Non-Fatal Errors**: ECR login failures in PostAuthenticate don't expose errors that could leak information
5. **No Secrets in Logs**: Authorization tokens are never logged
6. **Secret Masking**: ECR tokens follow Atmos secret masking patterns

## References

- [AWS ECR GetAuthorizationToken API](https://docs.aws.amazon.com/AmazonECR/latest/APIReference/API_GetAuthorizationToken.html)
- [Docker Config File Specification](https://docs.docker.com/engine/reference/commandline/cli/#configuration-files)
- [XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)
