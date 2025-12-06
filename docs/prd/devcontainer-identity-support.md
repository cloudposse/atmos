# Devcontainer Identity Support PRD

## Executive Summary

Add `--identity` flag support to `atmos devcontainer` commands to enable launching containers with authenticated cloud provider credentials. This allows users to run infrastructure tools inside devcontainers using Atmos-managed authentication without manual credential configuration.

**Key Design Decision:** Use the existing `AuthContext` pattern to inject provider-agnostic credentials into devcontainers via environment variables. The container runtime (Docker/Podman) receives environment variables from the authenticated identity's `Environment()` method, making this work with any auth provider (AWS, Azure, GitHub, GCP, etc.) without provider-specific logic in the devcontainer code.

## Problem Statement

### Current Limitations

Users cannot easily use authenticated identities inside devcontainers. To work with cloud resources from a devcontainer, they must:

1. Manually copy credentials into the container
2. Mount credential files as volumes
3. Set up credential forwarding
4. Maintain synchronization between host and container credentials

This defeats the purpose of Atmos auth management and creates security and usability issues.

### User Impact

**Current Experience:**
```bash
# User authenticates with Atmos
$ atmos auth login aws-dev

# User launches devcontainer
$ atmos devcontainer shell geodesic

# Inside container: NO credentials available
geodesic:~ $ aws sts get-caller-identity
# Error: Unable to locate credentials

# User must manually configure credentials inside container
```

**Desired Experience:**
```bash
# User authenticates and launches devcontainer with identity
$ atmos devcontainer shell geodesic --identity aws-dev

# Inside container: credentials automatically available
geodesic:~ $ aws sts get-caller-identity
{
  "UserId": "...",
  "Account": "123456789012",
  "Arn": "arn:aws:sts::123456789012:assumed-role/DevRole/..."
}

# Works with ANY authenticated provider
$ atmos devcontainer shell terraform --identity github-token
terraform:~ $ gh auth status
# ✓ Logged in to github.com as user via atmos
```

## Design Goals

1. **Provider-Agnostic Implementation**: Use interface methods (`Identity.Environment()`) rather than hardcoded provider-specific logic
2. **Seamless Integration**: Leverage existing `AuthContext` pattern for consistency with other Atmos auth features
3. **XDG Base Directory Support**: Properly configure Atmos paths inside containers for cache, config, and data directories
4. **Container Path Translation**: Translate host paths to container paths for mounted credential files
5. **Multiple Identity Support** (Future): Foundation for supporting multiple concurrent identities (e.g., `--identity aws-dev --identity github-api`)
6. **Security**: Credentials are injected via environment variables, never hardcoded in container images
7. **Backward Compatibility**: `--identity` flag is optional; devcontainers work without it as before

## Prerequisites

This feature depends on the **Auth Paths Interface** (see `docs/prd/auth-mounts-interface.md`), which extends the `Identity` and `Provider` interfaces to return credential path information via a `Paths()` method.

**Why This Matters:**
- **Environment variables alone are insufficient** for providers like AWS that require credential files (`~/.aws/credentials`, `~/.aws/config`)
- **Devcontainer code must remain provider-agnostic** - no hardcoded knowledge of AWS, Azure, GCP file locations
- **Providers know what paths they use** - they tell us via interface; we decide what to do with them (mount, copy, etc.)

**Key Design Principle:** Providers return **paths** (generic), consumers convert to **mounts** (specific to devcontainers).

**Example:**
```go
// Provider-agnostic devcontainer code (CORRECT)
whoami, err := authManager.Authenticate(ctx, identityName)
envVars := whoami.Environment  // Get env vars from provider
paths := whoami.Paths          // Get credential paths from provider

// Convert paths to mounts (consumer-specific logic)
for _, credPath := range paths {
    mount := convertPathToMount(credPath, containerConfig)
    config.Mounts = append(config.Mounts, mount)
}
```

## Technical Specification

### 1. Command Interface

Add `--identity` flag to all devcontainer lifecycle commands:

```bash
atmos devcontainer shell <name> [--identity <identity-name>]
atmos devcontainer start <name> [--identity <identity-name>]
atmos devcontainer exec <name> [--identity <identity-name>] -- <command>
atmos devcontainer rebuild <name> [--identity <identity-name>]
```

**Flag Behavior:**
- **Optional**: If not specified, container launches without auth environment variables
- **Auto-complete**: Tab completion suggests available authenticated identities
- **Validation**: Error if specified identity doesn't exist or isn't authenticated

### 2. Authentication Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. User Executes Command                                        │
│    $ atmos devcontainer shell geodesic --identity aws-dev       │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. Authenticate Identity                                        │
│    authContext := &schema.AuthContext{}                         │
│    whoami, err := authManager.Authenticate(ctx, "aws-dev")      │
│    // whoami.Environment contains provider env vars             │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. Get Environment Variables (Provider-Agnostic)                │
│    envVars := whoami.Environment  // map[string]string          │
│    // envVars contains provider-specific variables:             │
│    // - AWS: AWS_PROFILE, AWS_SHARED_CREDENTIALS_FILE, etc.     │
│    // - GitHub: GITHUB_TOKEN, etc.                              │
│    // - Azure: AZURE_CONFIG_DIR, etc.                           │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. Add Atmos XDG Environment Variables                          │
│    atmosEnvVars := map[string]string{                           │
│      "XDG_CONFIG_HOME": "/localhost/.atmos",  // container path │
│      "XDG_DATA_HOME":   "/localhost/.atmos",                    │
│      "XDG_CACHE_HOME":  "/localhost/.atmos",                    │
│      "ATMOS_BASE_PATH": "/localhost",                           │
│    }                                                             │
│    envVars = merge(envVars, atmosEnvVars)                       │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. Translate Host Paths to Container Paths                      │
│    // For credential files that reference host paths            │
│    translatePathsForContainer(envVars, containerConfig)         │
│    // Example transformations:                                  │
│    // Before: AWS_SHARED_CREDENTIALS_FILE=/home/user/.aws/...   │
│    // After:  AWS_SHARED_CREDENTIALS_FILE=/localhost/.aws/...   │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 6. Inject Environment Variables into Container                  │
│    createConfig := devcontainer.ToCreateConfig(...)             │
│    createConfig.Env = append(createConfig.Env, envVars...)      │
│    containerID := runtime.Create(ctx, createConfig)             │
└─────────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│ 7. Container Launched with Credentials Available                │
│    $ aws sts get-caller-identity  # Works!                      │
│    $ terraform plan               # Uses correct AWS creds      │
│    $ atmos describe stacks        # Uses correct XDG paths      │
└─────────────────────────────────────────────────────────────────┘
```

### 3. Implementation Details

#### 3.1 Provider-Agnostic Environment Variable Injection

**File:** `pkg/devcontainer/lifecycle.go`

```go
// injectIdentityEnvironment injects authenticated identity environment variables into container config.
// This is provider-agnostic - it works with AWS, Azure, GitHub, GCP, or any auth provider.
func injectIdentityEnvironment(ctx context.Context, config *devcontainer.Config, identityName string) error {
    // 1. Load Atmos configuration
    atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
    if err != nil {
        return fmt.Errorf("failed to load atmos config: %w", err)
    }

    // 2. Create auth manager
    authManager, err := createAuthManager(&atmosConfig.Auth)
    if err != nil {
        return fmt.Errorf("failed to create auth manager: %w", err)
    }

    // 3. Authenticate identity (provider-agnostic!)
    whoami, err := authManager.Authenticate(ctx, identityName)
    if err != nil {
        return fmt.Errorf("failed to authenticate identity '%s': %w", identityName, err)
    }

    // 4. Get environment variables from authenticated identity
    // This uses Identity.Environment() method - works with ANY provider!
    envVars := whoami.Environment
    if envVars == nil {
        envVars = make(map[string]string)
    }

    // 5. Add Atmos XDG environment variables for container paths
    atmosXDGVars := getAtmosXDGEnvironment(config)
    for k, v := range atmosXDGVars {
        envVars[k] = v
    }

    // 6. Translate host paths to container paths for credential files
    translatePathsForContainer(envVars, config)

    // 7. Inject environment variables into container config
    if config.ContainerEnv == nil {
        config.ContainerEnv = make(map[string]string)
    }
    for k, v := range envVars {
        config.ContainerEnv[k] = v
    }

    return nil
}
```

**Key Design Points:**
- **No provider-specific code**: Uses `whoami.Environment` from interface method
- **Works with any auth provider**: AWS, Azure, GitHub, GCP, custom providers
- **Single source of truth**: `Identity.Environment()` returns all provider-specific env vars

#### 3.2 Atmos XDG Environment Variables

**File:** `pkg/devcontainer/identity.go`

```go
// getAtmosXDGEnvironment returns Atmos-specific XDG environment variables for the container.
// These ensure Atmos inside the container uses the correct paths for config, cache, and data.
func getAtmosXDGEnvironment(config *devcontainer.Config) map[string]string {
    // Determine container base path (where workspace is mounted)
    containerBasePath := config.WorkspaceFolder
    if containerBasePath == "" {
        containerBasePath = "/workspace" // Default fallback
    }

    // Calculate container-relative .atmos path
    atmosPath := filepath.Join(containerBasePath, ".atmos")

    return map[string]string{
        // XDG Base Directory Specification paths
        "XDG_CONFIG_HOME": atmosPath,
        "XDG_DATA_HOME":   atmosPath,
        "XDG_CACHE_HOME":  atmosPath,

        // Atmos-specific paths
        "ATMOS_BASE_PATH": containerBasePath,

        // Optional: Override Atmos config directory explicitly
        // "ATMOS_CONFIG_DIR": filepath.Join(atmosPath, "config"),
    }
}
```

**Rationale:**
- **XDG compliance**: Follows XDG Base Directory Specification
- **Isolated storage**: Container-local .atmos directory for caches, configs, auth state
- **Path consistency**: Atmos inside container uses container-relative paths, not host paths
- **Mount-aware**: Paths are relative to `workspaceFolder` (the mounted workspace)

#### 3.3 Path Translation for Credential Files

**File:** `pkg/devcontainer/identity.go`

```go
// translatePathsForContainer translates host filesystem paths to container filesystem paths.
// This is critical for credential files that may reference host paths.
func translatePathsForContainer(envVars map[string]string, config *devcontainer.Config) {
    // Get host workspace path and container workspace path
    hostWorkspace := config.WorkspaceMount // e.g., "type=bind,source=/home/user/project,target=/workspace"
    containerWorkspace := config.WorkspaceFolder // e.g., "/workspace"

    // Extract source and target from mount string
    hostPath, containerPath := parseMountPaths(hostWorkspace, containerWorkspace)

    // Environment variables that commonly contain host paths to translate
    pathVars := []string{
        // AWS
        "AWS_SHARED_CREDENTIALS_FILE",
        "AWS_CONFIG_FILE",
        "AWS_CA_BUNDLE",

        // Azure
        "AZURE_CONFIG_DIR",

        // Google Cloud
        "CLOUDSDK_CONFIG",
        "GOOGLE_APPLICATION_CREDENTIALS",

        // Kubernetes
        "KUBECONFIG",

        // Generic credential paths
        "CREDENTIALS_FILE",
        "CONFIG_FILE",
    }

    // Translate each path variable
    for _, varName := range pathVars {
        if hostFilePath, exists := envVars[varName]; exists {
            // If path starts with host workspace, translate to container path
            if strings.HasPrefix(hostFilePath, hostPath) {
                relPath := strings.TrimPrefix(hostFilePath, hostPath)
                containerFilePath := filepath.Join(containerPath, relPath)
                envVars[varName] = containerFilePath
            }

            // If path is under user home, translate to container workspace
            // Example: ~/.aws/config → /workspace/.aws/config
            if strings.HasPrefix(hostFilePath, os.Getenv("HOME")) {
                relPath := strings.TrimPrefix(hostFilePath, os.Getenv("HOME"))
                containerFilePath := filepath.Join(containerPath, relPath)
                envVars[varName] = containerFilePath
            }
        }
    }
}

// parseMountPaths extracts source and target paths from workspace mount string.
func parseMountPaths(workspaceMount, workspaceFolder string) (hostPath, containerPath string) {
    // Parse mount string: "type=bind,source=/host/path,target=/container/path"
    parts := strings.Split(workspaceMount, ",")
    for _, part := range parts {
        if strings.HasPrefix(part, "source=") {
            hostPath = strings.TrimPrefix(part, "source=")
        }
    }

    containerPath = workspaceFolder
    if containerPath == "" {
        containerPath = "/workspace"
    }

    return hostPath, containerPath
}
```

**Why Path Translation is Critical:**

When Atmos auth writes credential files, it uses host paths:
```
AWS_SHARED_CREDENTIALS_FILE=/home/user/.aws/atmos/aws-sso/credentials
```

But inside the container, these paths don't exist! We need:
```
AWS_SHARED_CREDENTIALS_FILE=/workspace/.aws/atmos/aws-sso/credentials
```

Assuming the workspace is mounted as:
```
type=bind,source=/home/user/project,target=/workspace
```

#### 3.4 Command Integration

**File:** `cmd/devcontainer/shell.go`, `start.go`, `exec.go`, `rebuild.go`

Add `--identity` flag and call injection:

```go
var shellCmd = &cobra.Command{
    Use:   "shell [name]",
    Short: "Launch a shell in a devcontainer",
    Long: `Launch a shell in a devcontainer.

If --identity is specified, the container is launched with authenticated credentials.`,
    Args:              cobra.MaximumNArgs(1),
    ValidArgsFunction: devcontainerNameCompletion,
    RunE: func(cmd *cobra.Command, args []string) error {
        // ... existing code ...

        // Get identity flag
        identityName, _ := cmd.Flags().GetString("identity")

        // Call devcontainer shell with identity
        return e.ExecuteDevcontainerShell(atmosConfigPtr, name, shellInstance, identityName)
    },
}

func init() {
    shellCmd.Flags().StringP("identity", "i", "", "Authenticate with specified identity")
    shellCmd.Flags().String("instance", "default", "Instance name for the devcontainer")

    // Add identity autocomplete
    // (Implementation similar to auth commands)

    devcontainerCmd.AddCommand(shellCmd)
}
```

**Updated Function Signatures:**

```go
// File: pkg/devcontainer/lifecycle.go

// ExecuteDevcontainerShell launches an interactive shell with optional identity.
func ExecuteDevcontainerShell(
    atmosConfig *schema.AtmosConfiguration,
    name string,
    instance string,
    identityName string,  // NEW PARAMETER
) error

// ExecuteDevcontainerStart starts a devcontainer with optional identity.
func ExecuteDevcontainerStart(
    atmosConfig *schema.AtmosConfiguration,
    name string,
    instance string,
    attach bool,
    identityName string,  // NEW PARAMETER
) error

// ExecuteDevcontainerExec executes command with optional identity.
func ExecuteDevcontainerExec(
    atmosConfig *schema.AtmosConfiguration,
    name string,
    instance string,
    command []string,
    identityName string,  // NEW PARAMETER
) error
```

### 4. Container Path Mapping Strategy

#### 4.1 Recommended Workspace Mount

For identity support to work seamlessly, the devcontainer should mount the user's home directory or project directory in a predictable location:

**Option 1: Mount Home Directory** (Geodesic pattern)
```yaml
# devcontainer.json
{
  "workspaceMount": "type=bind,source=${localEnv:HOME},target=/localhost",
  "workspaceFolder": "/localhost"
}
```

**Result:**
- Host: `/home/user/.aws/atmos/aws-sso/credentials`
- Container: `/localhost/.aws/atmos/aws-sso/credentials`

**Option 2: Mount Project Directory**
```yaml
# devcontainer.json
{
  "workspaceMount": "type=bind,source=${localWorkspaceFolder},target=/workspace",
  "workspaceFolder": "/workspace"
}
```

**Result:**
- Host: `/home/user/project/.aws/atmos/aws-sso/credentials`
- Container: `/workspace/.aws/atmos/aws-sso/credentials`

#### 4.2 Credential File Strategy

**Best Practice: Store credentials under workspace**

Configure Atmos auth to write credentials under the workspace directory (not global `~/.aws/`):

```yaml
# atmos.yaml
auth:
  providers:
    aws-sso:
      type: aws-sso
      # Write credentials under project directory for container access
      base_path: ".atmos/auth"  # Relative to workspace
```

This ensures credentials are available inside the container via the workspace mount.

### 5. Security Considerations

#### 5.1 Environment Variable Injection

**Security:** Environment variables are the MOST secure way to pass credentials to containers:
- No credential files in container images (immutable, can't leak via `docker inspect`)
- No bind mounts of system credential directories (isolates container from host)
- Credentials exist only in container runtime memory
- Container removal destroys credentials

#### 5.2 Credential File Bind Mounts

**Alternative:** Some providers require credential files (e.g., AWS SSO session files). In these cases:

1. **Explicit mounts only:** Never mount entire `~/.aws/` or `~/.config/` directories
2. **Read-only mounts:** Mount credential files as read-only when possible
3. **Scoped mounts:** Mount only specific identity credential directories

**Example:**
```go
// Add bind mount for AWS SSO credentials if needed
if authContext.AWS != nil {
    credDir := filepath.Dir(authContext.AWS.CredentialsFile)
    containerCredDir := "/localhost/.aws/atmos/aws-sso"

    mount := container.Mount{
        Type:     "bind",
        Source:   credDir,
        Target:   containerCredDir,
        ReadOnly: true,
    }

    createConfig.Mounts = append(createConfig.Mounts, mount)
}
```

#### 5.3 Credential Lifecycle

**Container Lifetime = Credential Lifetime:**
- Credentials are injected when container is created
- Credentials remain valid while container runs
- Credentials are destroyed when container is removed
- No persistent credential storage in container images

**Credential Refresh:**
- If credentials expire, user must restart container with `--identity` flag
- Future enhancement: Support credential refresh without container restart

### 6. Provider Support Matrix

| Provider | Environment Variables | Credential Files | Path Translation |
|----------|----------------------|------------------|------------------|
| **AWS SSO** | `AWS_PROFILE`, `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE`, `AWS_REGION` | Yes - session files | Required |
| **AWS Static** | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` | Optional | Not required |
| **Azure** | `AZURE_CONFIG_DIR`, `AZURE_CLIENT_ID`, etc. | Yes - config directory | Required |
| **GCP** | `GOOGLE_APPLICATION_CREDENTIALS`, `CLOUDSDK_CONFIG` | Yes - service account JSON | Required |
| **GitHub OIDC** | `GITHUB_TOKEN` | No | Not required |
| **Custom** | Provider-defined via `Identity.Environment()` | Provider-defined | Provider-defined |

**Key Insight:** The implementation doesn't need to know about these specifics! Each provider's `Identity.Environment()` method returns the correct environment variables. Path translation handles credential file paths generically.

### 7. User Experience Examples

#### 7.1 AWS SSO Identity

```bash
# Login with AWS SSO identity
$ atmos auth login aws-dev

# Launch devcontainer with AWS credentials
$ atmos devcontainer shell geodesic --identity aws-dev

# Inside container: AWS SDK automatically uses Atmos credentials
geodesic:~ $ aws sts get-caller-identity
{
  "UserId": "AIDAI...",
  "Account": "123456789012",
  "Arn": "arn:aws:sts::123456789012:assumed-role/DevRole/..."
}

geodesic:~ $ terraform plan
# Terraform uses Atmos AWS credentials automatically

geodesic:~ $ atmos describe stacks
# Atmos inside container uses correct XDG paths
```

#### 7.2 Multiple Identities (Future)

```bash
# Authenticate with AWS and GitHub
$ atmos auth login aws-prod
$ atmos auth login github-api

# Launch devcontainer with both identities
$ atmos devcontainer shell geodesic --identity aws-prod --identity github-api

# Inside container: both credentials available
geodesic:~ $ aws sts get-caller-identity  # Works
geodesic:~ $ gh auth status              # Works
geodesic:~ $ atmos vendor pull           # Uses GitHub credentials
```

#### 7.3 Interactive Identity Selection

```bash
# Launch devcontainer with identity prompt
$ atmos devcontainer shell geodesic --identity

? Select an identity:
❯ aws-dev
  aws-prod
  github-api
  azure-subscription

# User selects identity interactively
```

### 8. Testing Strategy

#### 8.1 Unit Tests

Test identity injection logic with mocked auth system:

```go
func TestInjectIdentityEnvironment(t *testing.T) {
    tests := []struct {
        name         string
        identity     string
        mockWhoami   *types.WhoamiInfo
        expectedEnvs map[string]string
    }{
        {
            name:     "AWS SSO identity",
            identity: "aws-dev",
            mockWhoami: &types.WhoamiInfo{
                Environment: map[string]string{
                    "AWS_PROFILE": "aws-dev",
                    "AWS_SHARED_CREDENTIALS_FILE": "/home/user/.aws/atmos/aws-sso/credentials",
                },
            },
            expectedEnvs: map[string]string{
                "AWS_PROFILE": "aws-dev",
                "AWS_SHARED_CREDENTIALS_FILE": "/workspace/.aws/atmos/aws-sso/credentials",
                "XDG_CONFIG_HOME": "/workspace/.atmos",
            },
        },
    }
}
```

#### 8.2 Integration Tests

Test actual container launch with identity (requires Docker/Podman):

```bash
# Test script
atmos auth login test-identity
atmos devcontainer shell test-container --identity test-identity -- env | grep AWS_PROFILE
```

#### 8.3 Provider Compatibility Tests

Verify identity support works with each auth provider:
- AWS SSO
- AWS Static Credentials
- Azure (future)
- GCP (future)
- GitHub OIDC

### 9. Documentation Requirements

#### 9.1 User Documentation

**File:** `website/docs/cli/commands/devcontainer/shell.mdx`

Add section:
```markdown
## Using Authenticated Identities

Launch a devcontainer with Atmos-managed credentials:

\`\`\`bash
atmos devcontainer shell <name> --identity <identity-name>
\`\`\`

Inside the container, cloud provider SDKs automatically use the authenticated identity.

### Example: AWS SSO

\`\`\`bash
# Authenticate with AWS SSO
atmos auth login aws-dev

# Launch Geodesic with AWS credentials
atmos devcontainer shell geodesic --identity aws-dev

# Inside container: AWS CLI works automatically
$ aws sts get-caller-identity
\`\`\`
```

#### 9.2 Configuration Examples

**File:** `examples/devcontainer/atmos.yaml`

Add example with identity:
```yaml
components:
  devcontainer:
    geodesic:
      spec:
        image: "cloudposse/geodesic:latest"
        # Mount home directory for credential access
        workspaceMount: "type=bind,source=${localEnv:HOME},target=/localhost"
        workspaceFolder: "/localhost"
        containerEnv:
          # XDG paths are automatically configured by --identity flag
          ATMOS_BASE_PATH: "/localhost"
```

### 10. Future Enhancements

#### 10.1 Multiple Concurrent Identities

Support multiple `--identity` flags for multi-cloud workflows:

```bash
atmos devcontainer shell geodesic \
  --identity aws-prod \
  --identity github-api \
  --identity azure-subscription
```

**Implementation:** Merge environment variables from multiple `whoami.Environment` maps.

#### 10.2 Credential Refresh Without Restart

Support credential refresh inside running container:

```bash
# Inside container
$ atmos auth refresh aws-dev
# Credentials updated in container environment
```

#### 10.3 Credential Isolation Per Instance

Support different identities for different container instances:

```bash
atmos devcontainer shell geodesic --identity aws-dev --instance dev
atmos devcontainer shell geodesic --identity aws-prod --instance prod
```

Each instance runs with different credentials.

#### 10.4 Identity Inheritance from Stack Config

Automatically use identity specified in stack configuration:

```yaml
# stack config
components:
  terraform:
    vpc:
      settings:
        identity: aws-prod  # Devcontainer inherits this identity
```

```bash
atmos devcontainer shell geodesic -s ue2-prod
# Automatically uses aws-prod identity from stack config
```

## Implementation Checklist

### Phase 1: Core Identity Support
- [ ] Add `--identity` flag to devcontainer commands
- [ ] Implement provider-agnostic `injectIdentityEnvironment()`
- [ ] Implement `getAtmosXDGEnvironment()`
- [ ] Implement `translatePathsForContainer()`
- [ ] Update command signatures to accept `identityName`
- [ ] Add identity autocomplete to devcontainer commands

### Phase 2: Testing
- [ ] Unit tests for identity injection logic
- [ ] Unit tests for path translation logic
- [ ] Integration tests with Docker runtime
- [ ] Integration tests with Podman runtime
- [ ] Provider compatibility tests (AWS, GitHub)

### Phase 3: Documentation
- [ ] Update command documentation with `--identity` flag
- [ ] Add "Using Authenticated Identities" section to docs
- [ ] Add configuration examples with identity
- [ ] Update PRD with implementation details
- [ ] Update blog post with identity support

### Phase 4: Future Enhancements
- [ ] Multiple concurrent identities support
- [ ] Credential refresh without restart
- [ ] Identity inheritance from stack config
- [ ] Interactive identity selection prompt

## Success Criteria

1. ✅ Users can launch devcontainers with `--identity` flag
2. ✅ Cloud provider SDKs inside container use Atmos-managed credentials
3. ✅ Implementation is provider-agnostic (no AWS-specific code)
4. ✅ XDG paths are correctly configured inside container
5. ✅ Credential file paths are translated from host to container paths
6. ✅ Works with Docker and Podman runtimes
7. ✅ Tests achieve >80% coverage for identity injection code
8. ✅ Documentation includes usage examples and configuration guidance

## References

- [Auth Context Multi-Identity PRD](./auth-context-multi-identity.md)
- [Devcontainer Command PRD](./devcontainer-command.md)
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
- [Development Containers Specification](https://containers.dev/)
