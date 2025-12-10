# ECR Authentication PRD

## Executive Summary

Add `aws/ecr` identity type to Atmos auth system to enable Docker authentication with AWS Elastic Container Registry (ECR). This allows users to pull and push container images using Atmos-managed AWS credentials without manual `docker login` commands.

**Key Design Decision:** Implement ECR as an identity type (not a profile) that chains from existing AWS identities. ECR authentication tokens are written to an Atmos-managed Docker config file (`~/.config/atmos/docker/config.json`), isolating Atmos credentials from user's default Docker config.

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

**Desired Experience:**
```bash
# User configures ECR identity in atmos.yaml
# Then authenticates:
$ atmos auth login my-ecr

# Docker commands work automatically
$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
# Success!

# Token refresh is handled automatically
$ atmos auth refresh my-ecr
```

## Design Goals

1. **Identity-Based Architecture**: ECR as a first-class identity type, consistent with existing auth patterns
2. **Credential Chaining**: ECR identity chains from existing AWS identities (SSO, assume-role, IAM user)
3. **Isolated Docker Config**: Use Atmos-managed Docker config file to avoid polluting user's default config
4. **XDG Compliance**: Store Docker config at `~/.config/atmos/docker/config.json`
5. **Multi-Registry Support**: Support multiple ECR registries across different accounts/regions
6. **Explicit Authentication**: User explicitly logs in (no automatic background refresh initially)
7. **Clean Logout**: Remove ECR credentials on `atmos auth logout`

## Technical Specification

### 1. Identity Configuration

ECR is configured as an identity in `atmos.yaml` using the existing `principal` map pattern:

```yaml
auth:
  identities:
    my-ecr:
      kind: aws/ecr
      description: "Development ECR registry"
      principal:
        # Credential source (required): identity that provides AWS credentials
        identity: aws-dev

        # Registry configuration (required): at least one of these
        account_id: "123456789012"     # Explicit account ID
        # OR
        region: "us-east-2"            # If account_id omitted, uses caller identity's account

        # Optional: multiple registries
        registries:
          - account_id: "123456789012"
            region: "us-east-2"
          - account_id: "987654321098"
            region: "us-west-2"
```

**Configuration Options:**

| Field | Required | Description |
|-------|----------|-------------|
| `kind` | Yes | Must be `aws/ecr` |
| `description` | No | Human-readable description |
| `principal.identity` | Yes | Source identity for AWS credentials |
| `principal.account_id` | No* | AWS account ID for registry |
| `principal.region` | No* | AWS region for registry |
| `principal.registries` | No* | List of multiple registries |

*At least one of `account_id`, `region`, or `registries` must be specified.

### 2. Authentication Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. User Executes Command                                        │
│    $ atmos auth login my-ecr                                    │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. Resolve ECR Identity Configuration                           │
│    - Load identity config from atmos.yaml                       │
│    - Validate principal.identity exists                         │
│    - Parse registry configuration                               │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. Authenticate Source Identity                                 │
│    - Call authManager.Authenticate(ctx, "aws-dev")              │
│    - Obtain AWS credentials from source identity                │
│    - Source identity must be already authenticated              │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. Call ECR GetAuthorizationToken API                           │
│    - Use AWS credentials from source identity                   │
│    - Request token for configured registries                    │
│    - Response: base64(username:password), expiration            │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. Write Docker Config                                          │
│    - Create/update ~/.config/atmos/docker/config.json           │
│    - Add auth entry for each registry URL                       │
│    - Format: base64(AWS:password) in "auths" map                │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 6. Set DOCKER_CONFIG Environment Variable                       │
│    - Add to auth context environment                            │
│    - DOCKER_CONFIG=~/.config/atmos/docker                       │
│    - Docker commands use Atmos-managed config                   │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 7. Return Success with Expiration Info                          │
│    - Display authenticated registries                           │
│    - Show token expiration time (12 hours from now)             │
└─────────────────────────────────────────────────────────────────┘
```

### 3. Implementation Architecture

#### 3.1 Package Structure

```
pkg/auth/
├── identities/
│   └── aws/
│       └── ecr.go              # ECR identity implementation
├── cloud/
│   ├── aws/
│   │   └── ecr.go              # ECR API client (GetAuthorizationToken)
│   └── docker/
│       └── config.go           # Docker config.json manager
└── types/
    └── ecr_credentials.go      # ECRCredentials type
```

#### 3.2 ECR Identity Interface Implementation

**File:** `pkg/auth/identities/aws/ecr.go`

```go
// ECRIdentity implements ECR authentication as an identity.
type ECRIdentity struct {
    name           string
    config         *ECRIdentityConfig
    authConfig     *schema.AuthConfiguration
    dockerConfig   *docker.ConfigManager
    authenticating bool
    creds          *types.ECRCredentials
}

// Kind returns the identity kind.
func (e *ECRIdentity) Kind() string {
    return "aws/ecr"
}

// Authenticate retrieves ECR authorization token and writes Docker config.
func (e *ECRIdentity) Authenticate(ctx context.Context, authCtx *schema.AuthContext) (*types.WhoamiInfo, error)

// Validate checks if ECR credentials are still valid.
func (e *ECRIdentity) Validate(ctx context.Context, authCtx *schema.AuthContext) (*types.ValidationInfo, error)

// PostAuthenticate sets DOCKER_CONFIG environment variable.
func (e *ECRIdentity) PostAuthenticate(ctx context.Context, authCtx *schema.AuthContext, whoami *types.WhoamiInfo) error

// Logout removes ECR credentials from Docker config.
func (e *ECRIdentity) Logout(ctx context.Context, authCtx *schema.AuthContext) error
```

#### 3.3 Docker Config Manager

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

// GetAuthenticatedRegistries returns list of authenticated ECR registries.
func (m *ConfigManager) GetAuthenticatedRegistries() ([]string, error)
```

**Note:** Uses `pkg/xdg.GetXDGConfigDir()` to determine the config path, which respects `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME` environment variables and uses platform-appropriate defaults.

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

#### 3.4 ECR API Client

**File:** `pkg/auth/cloud/aws/ecr.go`

```go
// GetECRAuthorizationToken retrieves ECR authorization token.
func GetECRAuthorizationToken(ctx context.Context, cfg aws.Config, registryIDs []string) (*ECRAuthResult, error)

// ECRAuthResult contains ECR authorization token information.
type ECRAuthResult struct {
    Token      string    // Base64-decoded authorization token
    Username   string    // Always "AWS"
    Password   string    // Actual password from token
    Expiration time.Time // Token expiration time
    ProxyURL   string    // Registry proxy endpoint URL
}
```

#### 3.5 ECR Credentials Type

**File:** `pkg/auth/types/ecr_credentials.go`

```go
// ECRCredentials contains ECR-specific credential information.
type ECRCredentials struct {
    Registries   []ECRRegistry `json:"registries"`
    ConfigPath   string        `json:"config_path"`
    Expiration   string        `json:"expiration,omitempty"`
    SourceIdentity string      `json:"source_identity"`
}

// ECRRegistry represents a single ECR registry.
type ECRRegistry struct {
    URL        string `json:"url"`
    AccountID  string `json:"account_id"`
    Region     string `json:"region"`
}

// IsExpired returns true if the ECR token is expired.
func (c *ECRCredentials) IsExpired() bool

// GetExpiration returns the token expiration time.
func (c *ECRCredentials) GetExpiration() (*time.Time, error)

// BuildWhoamiInfo populates whoami information for ECR.
func (c *ECRCredentials) BuildWhoamiInfo(info *types.WhoamiInfo)

// Validate validates ECR credentials by checking Docker config.
func (c *ECRCredentials) Validate(ctx context.Context) (*types.ValidationInfo, error)
```

### 4. Identity Registration

**File:** `pkg/auth/factory/factory.go`

Add ECR identity type to the factory:

```go
// Identity kind constants (add to pkg/auth/types/kinds.go).
const (
    IdentityKindAWSUser          = "aws/user"
    IdentityKindAWSPermissionSet = "aws/permission-set"
    IdentityKindAWSAssumeRole    = "aws/assume-role"
    IdentityKindAWSECR           = "aws/ecr"
    IdentityKindAzureSubscription = "azure/subscription"
)

func (f *Factory) CreateIdentity(name string, config *IdentityConfig, authConfig *schema.AuthConfiguration) (Identity, error) {
    switch config.Kind {
    // ... existing cases ...
    case types.IdentityKindAWSECR:
        return aws.NewECRIdentity(name, config, authConfig)
    default:
        return nil, fmt.Errorf("unknown identity kind: %s", config.Kind)
    }
}
```

**Note:** Identity kind strings should be defined as constants in `pkg/auth/types/kinds.go` for consistency and to avoid typos. This refactoring should be applied to existing identity kinds as well.

### 5. Environment Variables

The ECR identity sets the following environment variables on successful authentication:

| Variable | Value | Purpose |
|----------|-------|---------|
| `DOCKER_CONFIG` | `~/.config/atmos/docker` | Points Docker to Atmos-managed config |

### 6. Error Handling

Add sentinel errors to `errors/errors.go`:

```go
// ECR authentication errors.
var (
    ErrECRAuthenticationFailed = errors.New("ECR authentication failed")
    ErrECRTokenExpired         = errors.New("ECR authorization token expired")
    ErrECRRegistryNotFound     = errors.New("ECR registry not found")
    ErrCleanupCredentials      = errors.New("failed to clean up credentials")
)
```

**Error Scenarios:**

| Scenario | Error | User Message |
|----------|-------|--------------|
| Source identity not authenticated | `ErrAuthenticationFailed` | "Source identity 'aws-dev' is not authenticated. Run: atmos auth login aws-dev" |
| Invalid AWS credentials | `ErrECRAuthenticationFailed` | "Failed to retrieve ECR authorization token: {aws_error}" |
| Registry not accessible | `ErrECRRegistryNotFound` | "ECR registry not found or not accessible: {registry_url}" |
| Docker config write failure | `ErrECRAuthenticationFailed` | "Failed to write Docker config: {error}" |
| Token expired | `ErrECRTokenExpired` | "ECR token expired. Run: atmos auth refresh my-ecr" |

### 7. File Locking

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

### 8. User Experience Examples

#### 8.1 Basic ECR Authentication

```bash
# Configure ECR identity in atmos.yaml
# (see configuration section above)

# First, authenticate source AWS identity
$ atmos auth login aws-dev
✓ Authenticated as arn:aws:sts::123456789012:assumed-role/DevRole/user

# Then authenticate ECR identity
$ atmos auth login my-ecr
✓ Authenticated to ECR registry: 123456789012.dkr.ecr.us-east-2.amazonaws.com
  Token expires: 2024-01-15 23:45:00 (in 12 hours)

# Pull images using Atmos-managed credentials
$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
latest: Pulling from my-app
...
Status: Downloaded newer image for my-app:latest
```

#### 8.2 Multi-Registry Authentication

```yaml
# atmos.yaml
auth:
  identities:
    all-ecr:
      kind: aws/ecr
      description: "All ECR registries"
      principal:
        identity: aws-admin
        registries:
          - account_id: "123456789012"
            region: "us-east-2"
          - account_id: "987654321098"
            region: "us-west-2"
```

```bash
$ atmos auth login all-ecr
✓ Authenticated to ECR registries:
  - 123456789012.dkr.ecr.us-east-2.amazonaws.com
  - 987654321098.dkr.ecr.us-west-2.amazonaws.com
  Token expires: 2024-01-15 23:45:00 (in 12 hours)
```

#### 8.3 Whoami Information

```bash
$ atmos auth whoami my-ecr
Identity: my-ecr
Kind: aws/ecr
Source Identity: aws-dev
Registries:
  - 123456789012.dkr.ecr.us-east-2.amazonaws.com
Docker Config: ~/.config/atmos/docker/config.json
Expiration: 2024-01-15 23:45:00 (in 11h 30m)
```

#### 8.4 Logout

```bash
$ atmos auth logout my-ecr
✓ Removed ECR credentials for: 123456789012.dkr.ecr.us-east-2.amazonaws.com
```

### 9. Integration with Devcontainers

ECR authentication integrates with devcontainer identity support:

```bash
# Authenticate and launch devcontainer with ECR access
$ atmos auth login my-ecr
$ atmos devcontainer shell geodesic --identity my-ecr

# Inside container: Docker commands use Atmos ECR credentials
geodesic:~ $ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
# Success! DOCKER_CONFIG is set automatically
```

### 10. GitHub Actions Integration

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
          atmos auth login my-ecr

      - name: Build and Push
        run: |
          docker build -t 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:${{ github.sha }} .
          docker push 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:${{ github.sha }}
```

## Implementation Checklist

### Phase 1: Core Implementation
- [ ] Add ECRCredentials type (`pkg/auth/types/ecr_credentials.go`)
- [ ] Add Docker config manager (`pkg/auth/cloud/docker/config.go`)
- [ ] Add ECR API client (`pkg/auth/cloud/aws/ecr.go`)
- [ ] Add ECR identity (`pkg/auth/identities/aws/ecr.go`)
- [ ] Register ECR identity in factory (`pkg/auth/factory/factory.go`)
- [ ] Add sentinel errors (`errors/errors.go`)

### Phase 2: Testing
- [ ] Unit tests for Docker config manager
- [ ] Unit tests for ECR API client (mocked)
- [ ] Unit tests for ECR identity
- [ ] Integration tests with LocalStack ECR

### Phase 3: Documentation
- [ ] Update `website/docs/cli/commands/auth/login.mdx`
- [ ] Add ECR configuration examples to auth docs
- [ ] Update schema documentation

### Phase 4: Future Enhancements
- [ ] Automatic token refresh before expiration
- [ ] Support for ECR Public (public.ecr.aws)
- [ ] Credential helper mode (docker-credential-atmos)

## Success Criteria

1. ✅ Users can configure ECR identity in `atmos.yaml`
2. ✅ `atmos auth login <ecr-identity>` retrieves token and writes Docker config
3. ✅ Docker commands use Atmos-managed credentials via `DOCKER_CONFIG`
4. ✅ `atmos auth logout` removes ECR credentials
5. ✅ `atmos auth whoami` shows ECR registry and expiration
6. ✅ Multi-registry support works correctly
7. ✅ Credential chaining from AWS identities works
8. ✅ Tests achieve >80% coverage
9. ✅ Documentation includes usage examples

## Security Considerations

1. **Credential Isolation**: Docker config stored in Atmos XDG config directory (via `pkg/xdg.GetXDGConfigDir("docker", 0700)`), separate from user's default `~/.docker/config.json`. This respects `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME` environment variables.
2. **File Permissions**: Docker config created with `0600` permissions
3. **Token Lifetime**: ECR tokens expire after 12 hours (AWS-enforced)
4. **Credential Cleanup**: `atmos auth logout` removes credentials from config file
5. **No Secrets in Logs**: Authorization tokens are never logged
6. **Secret Masking**: ECR tokens follow Atmos secret masking patterns

## References

- [AWS ECR GetAuthorizationToken API](https://docs.aws.amazon.com/AmazonECR/latest/APIReference/API_GetAuthorizationToken.html)
- [Docker Config File Specification](https://docs.docker.com/engine/reference/commandline/cli/#configuration-files)
- [Auth Context Multi-Identity PRD](./auth-context-multi-identity.md)
- [Auth Mounts Interface PRD](./auth-mounts-interface.md)
- [XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)
