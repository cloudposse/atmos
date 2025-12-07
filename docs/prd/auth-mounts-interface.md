# PRD: Auth Paths Interface

## Problem Statement

Currently, the `Identity` and `Provider` interfaces return environment variables via the `Environment()` method. However, some providers (like AWS) require **credential files** for proper authentication, and consumers need to know about these paths.

**Current Interface Limitation:**
- ✅ `Environment()` returns environment variables (e.g., `AWS_PROFILE=dev`)
- ❌ **No way to discover what credential paths exist** (e.g., `~/.aws/credentials`, `~/.aws/config`)

**Why Paths Matter:**

Different consumers need path information for different purposes:
1. **Devcontainers**: Mount credential files into containers
2. **Backup/Export**: Package credentials for transfer
3. **Cleanup/Logout**: Know what files to remove
4. **Validation**: Check if required credential files exist
5. **Future use cases**: Sync, migration, auditing, etc.

**Provider Path Requirements:**
- AWS: `~/.aws/credentials`, `~/.aws/config`
- Azure: `~/.azure/` directory
- GCP: `~/.config/gcloud/` directory, service account JSON files
- GitHub: No credential files (token via env var only)

**Current workaround violates provider-agnostic design** - Hardcoding path knowledge in consumer code (e.g., devcontainers).

## Current Workaround (Anti-Pattern)

```go
// WRONG: Hardcoded provider-specific logic in devcontainer code
func translatePathsForContainer(envVars map[string]string, config *devcontainer.Config) {
    // Hardcoded list of AWS-specific paths
    if path, exists := envVars["AWS_SHARED_CREDENTIALS_FILE"]; exists {
        envVars["AWS_SHARED_CREDENTIALS_FILE"] = translatePath(path, ...)
    }

    // Have to add more hardcoded logic for each provider
    if path, exists := envVars["AZURE_CONFIG_DIR"]; exists { ... }
    if path, exists := envVars["GOOGLE_APPLICATION_CREDENTIALS"]; exists { ... }
}
```

**Problems:**
- ❌ Devcontainer code knows about provider-specific env var names
- ❌ Must update devcontainer code when adding new providers
- ❌ Violates provider-agnostic design principle
- ❌ No way to discover mount requirements programmatically

## Proposed Solution

Add a `Paths()` method to `Identity` and `Provider` interfaces that returns credential file/directory paths with metadata. Consumers (like devcontainers) use this generic path information for their specific needs (mounting, copying, etc.).

**Key Insight:** Providers know what **paths** they use. Consumers decide **what to do** with those paths (mount them, copy them, delete them, etc.).

### Interface Changes

```go
// pkg/auth/types/interfaces.go

// PathType indicates what kind of filesystem entity the path represents.
type PathType string

const (
    PathTypeFile      PathType = "file"      // Single file (e.g., ~/.aws/credentials)
    PathTypeDirectory PathType = "directory" // Directory (e.g., ~/.azure/)
)

// Path represents a credential file or directory used by the provider/identity.
type Path struct {
    // Location is the filesystem path (may contain ~ for home directory).
    Location string

    // Type indicates if this is a file or directory.
    Type PathType

    // Required indicates if path must exist for provider to function.
    // If false, missing paths are optional (provider works without them).
    Required bool

    // Purpose describes what this path is used for (helps with debugging/logging).
    // Examples: "AWS credentials file", "Azure config directory", "GCP service account key"
    Purpose string

    // Metadata holds optional provider-specific information.
    // Consumers can use this for advanced features without breaking interface.
    // Examples:
    //   - "selinux_label": "system_u:object_r:container_file_t:s0" (future SELinux support)
    //   - "read_only": "true" (hint that path should be read-only)
    //   - "mount_target": "/workspace/.aws" (suggested container path)
    Metadata map[string]string
}

// Provider interface
type Provider interface {
    // ... existing methods ...

    // Paths returns credential files/directories used by this provider.
    // Returns empty slice if provider doesn't use filesystem credentials (e.g., GitHub tokens).
    // Consumers decide how to use these paths (mount, copy, delete, etc.).
    Paths() ([]Path, error)
}

// Identity interface
type Identity interface {
    // ... existing methods ...

    // Paths returns credential files/directories used by this identity.
    // Returns empty slice if identity doesn't use filesystem credentials.
    // Paths are in addition to provider paths (identities can add more files).
    Paths() ([]Path, error)
}

// WhoamiInfo struct
type WhoamiInfo struct {
    // ... existing fields ...

    // Paths contains combined paths from provider and identity chains.
    // Later paths override earlier ones if Location matches.
    Paths []Path `json:"paths,omitempty"`
}
```

### Provider Implementations

#### AWS Providers (SAML, SSO, User)

```go
// pkg/auth/providers/aws/saml/provider.go

func (p *SAMLProvider) Paths() ([]types.Path, error) {
    basePath := awsCloud.GetFilesBasePath(&p.providerConfig)

    // Use AWSFileManager to get correct provider-namespaced paths.
    fileManager, err := awsCloud.NewAWSFileManager(basePath)
    if err != nil {
        return nil, err
    }

    return []types.Path{
        {
            Location: fileManager.GetCredentialsPath(p.providerName),
            Type:     types.PathTypeFile,
            Required: true,
            Purpose:  "AWS credentials file",
            Metadata: map[string]string{
                "read_only": "true", // Hint for consumers
            },
        },
        {
            Location: fileManager.GetConfigPath(p.providerName),
            Type:     types.PathTypeFile,
            Required: false, // Config file is optional
            Purpose:  "AWS config file",
            Metadata: map[string]string{
                "read_only": "true",
            },
        },
    }, nil
}
```

#### Azure Provider

```go
// pkg/auth/providers/azure/provider.go

func (p *AzureProvider) Paths() ([]types.Path, error) {
    azureDir := os.Getenv("AZURE_CONFIG_DIR")
    if azureDir == "" {
        azureDir = filepath.Join(os.Getenv("HOME"), ".azure")
    }

    return []types.Path{
        {
            Location: azureDir,
            Type:     types.PathTypeDirectory,
            Required: true,
            Purpose:  "Azure config directory",
            Metadata: map[string]string{
                "read_only": "true",
            },
        },
    }, nil
}
```

#### GCP Provider

```go
// pkg/auth/providers/gcp/provider.go

func (p *GCPProvider) Paths() ([]types.Path, error) {
    paths := []types.Path{
        {
            Location: filepath.Join(os.Getenv("HOME"), ".config", "gcloud"),
            Type:     types.PathTypeDirectory,
            Required: false,
            Purpose:  "GCP gcloud config directory",
            Metadata: map[string]string{
                "read_only": "true",
            },
        },
    }

    // If using service account key file, add it
    if p.config.KeyFile != "" {
        paths = append(paths, types.Path{
            Location: p.config.KeyFile,
            Type:     types.PathTypeFile,
            Required: true,
            Purpose:  "GCP service account key file",
            Metadata: map[string]string{
                "read_only": "true",
            },
        })
    }

    return paths, nil
}
```

#### GitHub Provider

```go
// pkg/auth/providers/github/provider.go

func (p *GitHubProvider) Paths() ([]types.Path, error) {
    // GitHub uses token in environment variable, no files needed
    return []types.Path{}, nil
}
```

### AuthManager Integration

Update `AuthManager.Authenticate()` to populate paths in `WhoamiInfo`:

```go
// pkg/auth/manager.go

func (m *AuthManager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
    // ... existing authentication logic ...

    // Collect paths from provider chain
    var allPaths []types.Path

    // Get provider paths
    if m.currentProvider != nil {
        providerPaths, err := m.currentProvider.Paths()
        if err != nil {
            return nil, fmt.Errorf("failed to get provider paths: %w", err)
        }
        allPaths = append(allPaths, providerPaths...)
    }

    // Get identity paths (for identity-specific credential files)
    if identity != nil {
        identityPaths, err := identity.Paths()
        if err != nil {
            return nil, fmt.Errorf("failed to get identity paths: %w", err)
        }
        allPaths = append(allPaths, identityPaths...)
    }

    // Deduplicate paths - later paths override earlier ones if Location matches
    allPaths = deduplicatePaths(allPaths)

    whoami := &types.WhoamiInfo{
        // ... existing fields ...
        Paths: allPaths,
    }

    return whoami, nil
}

// deduplicatePaths removes duplicate paths, keeping the last occurrence.
// This allows identities to override provider paths.
func deduplicatePaths(paths []types.Path) []types.Path {
    seen := make(map[string]int) // Location -> index
    result := make([]types.Path, 0, len(paths))

    for _, path := range paths {
        if idx, exists := seen[path.Location]; exists {
            // Override existing path
            result[idx] = path
        } else {
            // Add new path
            seen[path.Location] = len(result)
            result = append(result, path)
        }
    }

    return result
}
```

### Devcontainer Integration (Provider-Agnostic)

Devcontainers use `Paths()` to create mounts. The conversion from `Path` → container mount is consumer-specific.

```go
// internal/exec/devcontainer_identity.go

func injectIdentityEnvironment(ctx context.Context, config *devcontainer.Config, identityName string) error {
    // ... authenticate identity ...
    whoami, err := authManager.Authenticate(ctx, identityName)

    // 1. Inject environment variables (existing)
    envVars := whoami.Environment
    // ... add XDG vars ...
    for k, v := range envVars {
        config.ContainerEnv[k] = v
    }

    // 2. Convert paths to mounts (NEW - provider-agnostic!)
    hostPath, containerPath := parseMountPaths(config.WorkspaceMount, config.WorkspaceFolder)

    for _, credPath := range whoami.Paths {
        // Check if file exists if required
        if credPath.Required {
            if _, err := os.Stat(credPath.Location); err != nil {
                return fmt.Errorf("required path %s (%s) does not exist: %w",
                    credPath.Location, credPath.Purpose, err)
            }
        } else if _, err := os.Stat(credPath.Location); err != nil {
            // Optional path doesn't exist, skip it
            continue
        }

        // Translate host path to container path
        containerMountPath := translatePathForContainer(credPath.Location, hostPath, containerPath)

        // Check metadata for hints
        readOnly := true // Default to read-only for security
        if ro, ok := credPath.Metadata["read_only"]; ok && ro == "false" {
            readOnly = false
        }

        // Add to devcontainer mounts
        mountSpec := devcontainer.Mount{
            Type:     "bind",
            Source:   credPath.Location,
            Target:   containerMountPath,
            ReadOnly: readOnly,
        }

        config.Mounts = append(config.Mounts, mountSpec)
    }

    return nil
}
```

### Other Potential Consumers

The `Paths()` interface is generic enough for other use cases:

**Backup/Export:**
```go
// Export all credential files for transfer
func ExportCredentials(whoami *types.WhoamiInfo, destDir string) error {
    for _, path := range whoami.Paths {
        if path.Type == types.PathTypeFile {
            // Copy file to backup
        } else {
            // Copy directory recursively
        }
    }
}
```

**Cleanup/Logout:**
```go
// Remove credential files on logout
func CleanupCredentials(whoami *types.WhoamiInfo) error {
    for _, path := range whoami.Paths {
        os.RemoveAll(path.Location)
    }
}
```

## Benefits

✅ **Generic**: `Paths()` is useful beyond just devcontainers (backup, cleanup, validation, etc.)
✅ **Provider-Agnostic**: Consumers don't know about provider-specific logic
✅ **Extensible**: New providers just implement `Paths()` interface
✅ **Discoverable**: Consumers can query what paths are needed
✅ **Flexible**: Metadata allows future extensions (SELinux, permissions, etc.) without breaking interface
✅ **Consistent**: Same pattern as `Environment()` method
✅ **Testable**: Easy to mock path requirements in tests

## Implementation Phases

### Phase 1: Interface Addition
1. Add `Path` type and `PathType` constants to `pkg/auth/types/interfaces.go`
2. Add `Paths()` method to `Provider` and `Identity` interfaces
3. Add `Paths` field to `WhoamiInfo` struct
4. Add default implementations returning empty slice for all existing providers

### Phase 2: AWS Implementation
1. Implement `Paths()` for AWS SAML provider
2. Implement `Paths()` for AWS SSO provider
3. Implement `Paths()` for AWS User provider
4. Update `AuthManager.Authenticate()` to collect paths with deduplication
5. Write tests for AWS path discovery.

### Phase 3: Other Providers
1. Implement `Paths()` for Azure provider (when available)
2. Implement `Paths()` for GCP provider (when available)
3. Implement `Paths()` for GitHub provider (returns empty)
4. Write tests for all provider path discovery

### Phase 4: Devcontainer Integration
1. Update `injectIdentityEnvironment()` to use `whoami.Paths`
2. Convert `Path` objects to container mounts
3. Remove hardcoded path translation logic
4. Add path validation (check Required files exist)
5. Add tests for devcontainer path-to-mount conversion

### Phase 5: Documentation
1. Update auth PRD with `Paths()` method documentation
2. Update devcontainer PRD to reference path injection
3. Add examples to blog post

## Backward Compatibility

✅ **Non-Breaking Change**: Adding method to interface with default implementations
✅ **Existing Providers**: All return empty slice until explicitly implemented
✅ **Existing Devcontainers**: Continue working without path injection
✅ **Gradual Migration**: Can implement `Paths()` per provider over time

## Security Considerations

1. **Read-Only by Default**: Devcontainer mounts should be read-only to prevent container from modifying credentials
2. **Validation**: Check that required paths exist before starting container
3. **Path Sanitization**: Ensure paths are within allowed directories (no arbitrary file system access)
4. **Container Isolation**: Mounts are per-container, not shared across instances
5. **Metadata Extensibility**: `Metadata` field allows future security features (SELinux labels, etc.) without breaking changes

## Design Decisions

### 1. Path Conflicts
**Decision**: Later paths override earlier ones (identity > provider) based on `Location` match.
**Rationale**: Allows identity-specific overrides while keeping provider defaults.

### 2. SELinux Support
**Decision**: Build interface with SELinux in mind via `Metadata` field, don't implement yet.
**Rationale**:
- `Metadata["selinux_label"]` reserves space for future implementation
- Consumers can ignore metadata they don't understand
- No breaking change when SELinux is added later

**Future Example:**
```go
Path{
    Location: "~/.aws/credentials",
    Metadata: map[string]string{
        "selinux_label": "system_u:object_r:container_file_t:s0",
    },
}
```

### 3. Paths vs Mounts
**Decision**: Interface returns `Paths`, consumers convert to mounts/copies/etc.
**Rationale**:
- More generic and reusable (backup, cleanup, validation)
- Devcontainers are one consumer; others may copy files instead of mount
- Provider knows paths; consumer decides what to do with them

## Alternatives Considered

### Alternative 1: Hardcode Path Translation
**Rejected**: Violates provider-agnostic design, not extensible

### Alternative 2: Parse Environment Variables
**Rejected**: Not all providers use env vars for paths, requires hardcoded knowledge

### Alternative 3: Convention-Based Mounting
**Rejected**: Assumes all providers follow same directory structure (they don't)

### Alternative 4: Return Mount Specs Directly
**Rejected**: Too specific to devcontainers; other consumers (backup, cleanup) need paths, not mounts

## Success Criteria

✅ Devcontainer identity injection works without hardcoded provider logic
✅ Adding new provider doesn't require devcontainer code changes
✅ All existing providers implement `Paths()` (even if returning empty slice)
✅ Tests cover path discovery for all providers
✅ Documentation updated with path injection examples
✅ Interface supports future extensions (SELinux) via `Metadata` field
