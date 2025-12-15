# ECR Authentication PRD

## Executive Summary

Add ECR authentication support to Atmos via a new **`auth.integrations`** section that provides client-only credential materialization for services like ECR and EKS.

**Key Insight:** ECR (and EKS) credentials are fundamentally different from identities:

| Concept                     | IAM User | Docker Login (ECR) | EKS (IAM → kubeconfig) |
|-----------------------------|----------|---------------------|-------------------------|
| Stored identity object      | ✅       | ❌                  | ❌                      |
| Policy attachment           | ✅       | ❌                  | ❌                      |
| Stable subject              | ✅       | ❌                  | ❌                      |
| Server-side lifecycle       | ✅       | ❌                  | ❌                      |
| Client-only materialization | ❌       | ✅                  | ✅                      |

**Design Decision:** ECR login is implemented as an **integration** (not an identity) that references an existing identity. This cleanly separates "who you are" (identity) from "derived credentials for services" (integrations).

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
$ atmos auth login dev-admin

# User needs to pull ECR image... must do this manually:
$ aws ecr get-login-password --region us-east-2 | \
  docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-2.amazonaws.com

# Token expires after 12 hours, must repeat
# Must also configure DOCKER_CONFIG for isolation
```

**Desired Experience (Automatic via integration):**
```bash
# Identity references an integration that auto-logins to ECR
$ atmos auth login dev-admin
✓ Authenticated as arn:aws:sts::123456789012:assumed-role/DevRole/user
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

# Docker commands work automatically
$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
# Success!
```

**Desired Experience (Explicit command with integration name):**
```bash
# Explicitly login to ECR using a named integration
$ atmos auth ecr-login dev/ecr
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
# Success!
```

**Desired Experience (Explicit command with identity flag):**
```bash
# Explicitly login to ECR using an identity's linked integrations
$ atmos auth ecr-login --identity dev-admin
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
# Success!
```

## Design Goals

1. **Clean Separation of Concerns**: Identities for "who you are", integrations for "derived service credentials"
2. **Explicit Configuration**: Each integration is named and configured independently
3. **Identity Linking**: Integrations reference identities for AWS credentials
4. **Automatic Login**: Identities can trigger integrations via PostAuthenticate hook
5. **Explicit Control**: Standalone command for ad-hoc integration login
6. **Non-Blocking Errors**: Integration failures don't block identity authentication
7. **Isolated Docker Config**: Use Atmos-managed Docker config file
8. **XDG Compliance**: Use `pkg/xdg` for config paths
9. **Multi-Registry Support**: Support multiple ECR registries per integration
10. **Future-Proof**: Pattern extends naturally to EKS and other integrations

## Technical Specification

### 1. Configuration Schema

#### New `auth.integrations` Section

```yaml
auth:
  # Identities define WHO you are (server-side, policy-attached)
  identities:
    dev-admin:
      kind: aws/permission-set
      via:
        provider: aws-sso
      principal:
        name: AdministratorAccess
        account:
          name: dev
      # Optional: auto-trigger these integrations on login
      integrations:
        - dev/ecr

  # Integrations define DERIVED credentials (client-only materialization)
  integrations:
    dev/ecr:
      kind: aws/ecr
      identity: dev-admin              # Which identity provides AWS creds
      registries:
        - account_id: "123456789012"
          region: us-east-2
        - account_id: "987654321098"
          region: us-west-2

    # Future: EKS integration (not implemented in this PRD)
    # dev/kubecfg:
    #   kind: aws/eks
    #   identity: dev-admin
    #   clusters:
    #     - name: dev-cluster
    #       region: us-east-2
```

#### Integration Configuration Options

| Field | Required | Description |
|-------|----------|-------------|
| `kind` | Yes | Integration type (e.g., `aws/ecr`, future: `aws/eks`) |
| `identity` | Yes | Name of identity providing AWS credentials |
| `registries` | Yes | List of ECR registries to authenticate |
| `registries[].account_id` | Yes | AWS account ID for registry |
| `registries[].region` | Yes | AWS region for registry |

#### Identity Integration Reference

Identities can optionally list integrations to auto-trigger on login:

| Field | Required | Description |
|-------|----------|-------------|
| `integrations` | No | List of integration names to trigger on login |

### 2. Commands

#### `atmos auth ecr-login`

```bash
# Login using a named integration
atmos auth ecr-login dev/ecr

# Login using an identity's linked integrations
atmos auth ecr-login --identity dev-admin

# Override with explicit registry (uses current AWS credentials)
atmos auth ecr-login --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com

# Multiple explicit registries
atmos auth ecr-login \
  --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com \
  --registry 987654321098.dkr.ecr.us-west-2.amazonaws.com
```

### 3. Authentication Flows

#### Flow A: Auto-trigger via Identity Login

```text
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
│    - TriggerIntegrations() - process identity.integrations list │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. For each integration in identity.integrations:               │
│    - Look up integration config (auth.integrations.dev/ecr)     │
│    - Call integration handler (ECR login)                       │
│    - Log success/warning (non-fatal errors)                     │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. Return Success                                               │
│    ✓ Authenticated as arn:aws:sts::...                          │
│    ✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com    │
└─────────────────────────────────────────────────────────────────┘
```

#### Flow B: Explicit Integration Login

```text
┌─────────────────────────────────────────────────────────────────┐
│ 1. User Executes Command                                        │
│    $ atmos auth ecr-login dev/ecr                               │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. Load Integration Config                                      │
│    - Look up auth.integrations.dev/ecr                          │
│    - Get identity reference (dev-admin)                         │
│    - Get registry list                                          │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. Authenticate Referenced Identity                             │
│    - Authenticate dev-admin identity                            │
│    - Obtain AWS credentials                                     │
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

### 4. Implementation Architecture

#### 4.1 Package Structure

```text
pkg/auth/
├── cloud/
│   ├── aws/
│   │   └── ecr.go              # ECR token fetcher
│   └── docker/
│       └── config.go           # Docker config.json manager
├── integrations/
│   ├── registry.go             # Integration type registry
│   ├── types.go                # Integration interfaces
│   └── aws/
│       └── ecr.go              # ECR integration implementation
└── ecr_login.go                # Standalone command implementation

cmd/auth/
└── ecr_login.go                # atmos auth ecr-login command
```

#### 4.2 Integration Interface

**File:** `pkg/auth/integrations/types.go`

```go
// Integration represents a client-only credential materialization.
type Integration interface {
    // Kind returns the integration type (e.g., "aws/ecr").
    Kind() string

    // Execute performs the integration using the provided AWS credentials.
    Execute(ctx context.Context, creds *types.AWSCredentials) error
}

// IntegrationConfig is the configuration for an integration.
type IntegrationConfig struct {
    Kind     string                 `mapstructure:"kind"`
    Identity string                 `mapstructure:"identity"`
    Config   map[string]interface{} `mapstructure:",remain"`
}

// IntegrationFactory creates integrations from configuration.
type IntegrationFactory func(config *IntegrationConfig) (Integration, error)
```

#### 4.3 Integration Registry

**File:** `pkg/auth/integrations/registry.go`

```go
// Registry holds registered integration factories.
var registry = map[string]IntegrationFactory{}

// Register adds an integration factory for a kind.
func Register(kind string, factory IntegrationFactory) {
    registry[kind] = factory
}

// Create instantiates an integration from config.
func Create(config *IntegrationConfig) (Integration, error) {
    factory, ok := registry[config.Kind]
    if !ok {
        return nil, fmt.Errorf("unknown integration kind: %s", config.Kind)
    }
    return factory(config)
}

// Integration kind constants.
const (
    KindAWSECR = "aws/ecr"
    KindAWSEKS = "aws/eks" // Future
)

func init() {
    Register(KindAWSECR, NewECRIntegration)
}
```

#### 4.4 ECR Integration Implementation

**File:** `pkg/auth/integrations/aws/ecr.go`

```go
// ECRIntegration implements the aws/ecr integration type.
type ECRIntegration struct {
    identity   string
    registries []ECRRegistry
}

// ECRRegistry represents a single ECR registry configuration.
type ECRRegistry struct {
    AccountID string `mapstructure:"account_id"`
    Region    string `mapstructure:"region"`
}

// NewECRIntegration creates an ECR integration from config.
func NewECRIntegration(config *IntegrationConfig) (Integration, error) {
    var registries []ECRRegistry
    if err := mapstructure.Decode(config.Config["registries"], &registries); err != nil {
        return nil, fmt.Errorf("invalid registries config: %w", err)
    }
    return &ECRIntegration{
        identity:   config.Identity,
        registries: registries,
    }, nil
}

// Kind returns "aws/ecr".
func (e *ECRIntegration) Kind() string {
    return integrations.KindAWSECR
}

// Execute performs ECR login for all configured registries.
func (e *ECRIntegration) Execute(ctx context.Context, creds *types.AWSCredentials) error {
    dockerConfig, err := docker.NewConfigManager()
    if err != nil {
        return err
    }

    awsCfg, err := buildAWSConfig(ctx, creds)
    if err != nil {
        return err
    }

    for _, reg := range e.registries {
        result, err := awsCloud.GetAuthorizationToken(ctx, awsCfg, reg.AccountID, reg.Region)
        if err != nil {
            return fmt.Errorf("ECR login failed for %s: %w", reg.AccountID, err)
        }

        if err := dockerConfig.WriteAuth(result.Registry, result.Username, result.Password); err != nil {
            return fmt.Errorf("failed to write Docker config: %w", err)
        }

        ui.Success("ECR login: %s (expires in 12h)", result.Registry)
    }

    os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir())
    return nil
}
```

#### 4.5 Docker Config Manager

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

#### 4.6 ECR Token Fetcher

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

#### 4.7 Modify Identity PostAuthenticate

In each identity's `PostAuthenticate()` method, add integration trigger support:

**File:** `pkg/auth/identities/aws/*.go`

```go
func (i *userIdentity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
    // Existing code...
    if err := awsCloud.SetupFiles(...); err != nil { /* handle */ }
    if err := awsCloud.SetAuthContext(...); err != nil { /* handle */ }
    if err := awsCloud.SetEnvironmentVariables(...); err != nil { /* handle */ }

    // NEW: Trigger linked integrations
    for _, integrationName := range i.config.Integrations {
        integrationConfig, err := params.AuthConfig.GetIntegration(integrationName)
        if err != nil {
            log.Warn("Failed to find integration", "name", integrationName, "error", err)
            continue
        }

        integration, err := integrations.Create(integrationConfig)
        if err != nil {
            log.Warn("Failed to create integration", "name", integrationName, "error", err)
            continue
        }

        if err := integration.Execute(ctx, params.Credentials); err != nil {
            log.Warn("Integration failed", "name", integrationName, "error", err)
            // Non-fatal - don't block authentication
        }
    }

    return nil
}
```

#### 4.8 Standalone Command

**File:** `cmd/auth/ecr_login.go`

```go
var ecrLoginCmd = &cobra.Command{
    Use:   "ecr-login [integration]",
    Short: "Login to AWS ECR registries",
    Long: `Login to AWS ECR registries using a named integration or identity.

Examples:
  # Login using a named integration
  atmos auth ecr-login dev/ecr

  # Login using an identity's linked integrations
  atmos auth ecr-login --identity dev-admin

  # Override with explicit registry URL
  atmos auth ecr-login --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com`,
    Args: cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        var integrationName string
        if len(args) > 0 {
            integrationName = args[0]
        }
        identity, _ := cmd.Flags().GetString("identity")
        registries, _ := cmd.Flags().GetStringArray("registry")
        return auth.ECRLogin(ctx, integrationName, identity, registries)
    },
}

func init() {
    ecrLoginCmd.Flags().StringP("identity", "i", "", "Identity to use (triggers its linked integrations)")
    ecrLoginCmd.Flags().StringArrayP("registry", "r", nil, "ECR registry URL(s) - explicit mode")
    authCmd.AddCommand(ecrLoginCmd)
}
```

**File:** `pkg/auth/ecr_login.go`

```go
// ECRLogin performs ECR authentication.
// Priority: integrationName > identity > registries (explicit mode).
func ECRLogin(ctx context.Context, integrationName, identityName string, registries []string) error {
    authManager, err := createAuthManager()
    if err != nil {
        return err
    }

    // Case 1: Named integration
    if integrationName != "" {
        return executeIntegration(ctx, authManager, integrationName)
    }

    // Case 2: Identity's linked integrations
    if identityName != "" {
        return executeIdentityIntegrations(ctx, authManager, identityName)
    }

    // Case 3: Explicit registries with current AWS credentials
    if len(registries) > 0 {
        return executeExplicitRegistries(ctx, registries)
    }

    return fmt.Errorf("specify an integration name, --identity, or --registry")
}

func executeIntegration(ctx context.Context, authManager *AuthManager, name string) error {
    integrationConfig, err := authManager.GetIntegration(name)
    if err != nil {
        return fmt.Errorf("integration not found: %s", name)
    }

    // Authenticate the referenced identity
    whoami, err := authManager.Authenticate(ctx, integrationConfig.Identity)
    if err != nil {
        return fmt.Errorf("failed to authenticate identity '%s': %w", integrationConfig.Identity, err)
    }

    // Create and execute the integration
    integration, err := integrations.Create(integrationConfig)
    if err != nil {
        return err
    }

    return integration.Execute(ctx, whoami.Credentials)
}

func executeIdentityIntegrations(ctx context.Context, authManager *AuthManager, identityName string) error {
    identityConfig, err := authManager.GetIdentity(identityName)
    if err != nil {
        return fmt.Errorf("identity not found: %s", identityName)
    }

    if len(identityConfig.Integrations) == 0 {
        return fmt.Errorf("identity '%s' has no linked integrations", identityName)
    }

    // Authenticate the identity
    whoami, err := authManager.Authenticate(ctx, identityName)
    if err != nil {
        return fmt.Errorf("failed to authenticate identity '%s': %w", identityName, err)
    }

    // Execute each linked integration
    for _, integrationName := range identityConfig.Integrations {
        integrationConfig, err := authManager.GetIntegration(integrationName)
        if err != nil {
            return fmt.Errorf("integration not found: %s", integrationName)
        }

        integration, err := integrations.Create(integrationConfig)
        if err != nil {
            return err
        }

        if err := integration.Execute(ctx, whoami.Credentials); err != nil {
            return err
        }
    }

    return nil
}

func executeExplicitRegistries(ctx context.Context, registries []string) error {
    // Use default AWS credential chain
    awsCfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return fmt.Errorf("failed to load AWS config: %w", err)
    }

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

    os.Setenv("DOCKER_CONFIG", dockerConfig.GetConfigDir())
    return nil
}
```

### 5. Error Handling

Add sentinel errors to `errors/errors.go`:

```go
// ECR authentication errors.
var (
    ErrECRAuthenticationFailed = errors.New("ECR authentication failed")
    ErrECRTokenExpired         = errors.New("ECR authorization token expired")
    ErrECRRegistryNotFound     = errors.New("ECR registry not found")
    ErrIntegrationNotFound     = errors.New("integration not found")
    ErrUnknownIntegrationKind  = errors.New("unknown integration kind")
)
```

**Error Behavior:**

| Context | Error Behavior |
|---------|----------------|
| PostAuthenticate hook | Non-fatal: log warning, continue identity auth |
| Standalone command | Fatal: return error to user |

### 6. Environment Variables

When ECR login succeeds, set:

| Variable | Value | Purpose |
|----------|-------|---------|
| `DOCKER_CONFIG` | `~/.config/atmos/docker` | Points Docker to Atmos-managed config |

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

## User Experience Examples

### Automatic ECR Login via Identity

```yaml
# atmos.yaml
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
      integrations:
        - dev/ecr

  integrations:
    dev/ecr:
      kind: aws/ecr
      identity: dev-admin
      registries:
        - account_id: "123456789012"
          region: us-east-2
```

```bash
$ atmos auth login dev-admin
✓ Authenticated as arn:aws:sts::123456789012:assumed-role/DevRole/user
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

# Docker commands work automatically
$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
latest: Pulling from my-app
...
Status: Downloaded newer image for my-app:latest
```

### Explicit Integration Login

```bash
# Login using a named integration
$ atmos auth ecr-login dev/ecr
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
```

### Login via Identity Flag

```bash
# Login using an identity's linked integrations
$ atmos auth ecr-login --identity dev-admin
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)

$ docker pull 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:latest
```

### Explicit Registry Override

```bash
# Override with explicit registry URL (uses current AWS credentials)
$ atmos auth ecr-login --registry 123456789012.dkr.ecr.us-east-2.amazonaws.com
✓ ECR login: 123456789012.dkr.ecr.us-east-2.amazonaws.com (expires in 12h)
```

### Multi-Registry Integration

```yaml
# atmos.yaml
auth:
  integrations:
    all-envs/ecr:
      kind: aws/ecr
      identity: devops-admin
      registries:
        - account_id: "123456789012"
          region: us-east-2
        - account_id: "987654321098"
          region: us-west-2
```

```bash
$ atmos auth ecr-login all-envs/ecr
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
          # Option A: Identity login triggers ECR via linked integration
          atmos auth login aws-ci

          # Option B: Explicit integration login
          atmos auth ecr-login ci/ecr

      - name: Build and Push
        run: |
          docker build -t 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:${{ github.sha }} .
          docker push 123456789012.dkr.ecr.us-east-2.amazonaws.com/my-app:${{ github.sha }}
```

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Add integration type system (`pkg/auth/integrations/types.go`)
- [ ] Add integration registry (`pkg/auth/integrations/registry.go`)
- [ ] Add Docker config manager (`pkg/auth/cloud/docker/config.go`)
- [ ] Add ECR token fetcher (`pkg/auth/cloud/aws/ecr.go`)
- [ ] Add sentinel errors (`errors/errors.go`)

### Phase 2: ECR Integration
- [ ] Add ECR integration (`pkg/auth/integrations/aws/ecr.go`)
- [ ] Register ECR integration in registry

### Phase 3: Identity Integration Linking
- [ ] Update schema for `auth.integrations` section
- [ ] Update schema for `identity.integrations` list
- [ ] Modify identity PostAuthenticate to trigger integrations

### Phase 4: Standalone Command
- [ ] Add `cmd/auth/ecr_login.go`
- [ ] Add `pkg/auth/ecr_login.go`

### Phase 5: Testing
- [ ] Unit tests for integration registry
- [ ] Unit tests for Docker config manager
- [ ] Unit tests for ECR token fetcher (mocked)
- [ ] Unit tests for ECR integration
- [ ] Unit tests for PostAuthenticate integration trigger
- [ ] Integration tests for `atmos auth ecr-login` command

### Phase 6: Documentation
- [ ] Update `website/docs/cli/commands/auth/login.mdx`
- [ ] Add `website/docs/cli/commands/auth/ecr-login.mdx`
- [ ] Add integration configuration examples to auth docs

## Success Criteria

1. ✅ `auth.integrations` schema validated and documented
2. ✅ `atmos auth login <identity>` with linked integrations triggers ECR login
3. ✅ `atmos auth ecr-login <integration>` works with named integration
4. ✅ `atmos auth ecr-login --identity <name>` triggers identity's integrations
5. ✅ `atmos auth ecr-login --registry <url>` works with explicit registries
6. ✅ Docker commands use Atmos-managed credentials via `DOCKER_CONFIG`
7. ✅ Multi-registry support works correctly
8. ✅ Integration failures don't block identity authentication
9. ✅ Tests achieve >80% coverage
10. ✅ Documentation includes usage examples

## Security Considerations

1. **Credential Isolation**: Docker config stored in Atmos XDG config directory (via `pkg/xdg.GetXDGConfigDir("docker", 0700)`), separate from user's default `~/.docker/config.json`. This respects `ATMOS_XDG_CONFIG_HOME` and `XDG_CONFIG_HOME` environment variables.
2. **File Permissions**: Docker config created with `0600` permissions.
3. **Token Lifetime**: ECR tokens expire after 12 hours (AWS-enforced).
4. **Non-Fatal Errors**: Integration failures in PostAuthenticate don't expose errors that could leak information.
5. **No Secrets in Logs**: Authorization tokens are never logged.
6. **Secret Masking**: ECR tokens follow Atmos secret masking patterns.

## Future Extensions

The `integrations` pattern naturally extends to other client-only credential materializations:

### EKS Integration (Future)

```yaml
auth:
  integrations:
    dev/kubecfg:
      kind: aws/eks
      identity: dev-admin
      clusters:
        - name: dev-cluster
          region: us-east-2
          alias: dev           # Optional: kubeconfig context name
```

### GCR/GAR Integration (Future)

```yaml
auth:
  integrations:
    dev/gcr:
      kind: gcp/artifact-registry
      identity: gcp-dev
      registries:
        - project: my-project
          location: us-central1
```

## References

- [AWS ECR GetAuthorizationToken API](https://docs.aws.amazon.com/AmazonECR/latest/APIReference/API_GetAuthorizationToken.html)
- [Docker Config File Specification](https://docs.docker.com/engine/reference/commandline/cli/#configuration-files)
- [XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)
