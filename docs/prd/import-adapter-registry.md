# Import Adapter Registry Pattern

## Overview

This document describes the import adapter registry pattern for Atmos, which provides a modular, extensible architecture for custom import schemes that require **transformation** (like `terragrunt://` for HCL→YAML conversion, or `mock://` for testing). Standard go-getter schemes continue to work through the existing infrastructure.

## Problem Statement

### Current State

Atmos uses [go-getter](https://github.com/hashicorp/go-getter) for downloading remote imports, which supports many schemes:
- `http://`, `https://` - HTTP(S) downloads
- `git::` - Git repositories
- `s3::` - Amazon S3
- `gcs::` - Google Cloud Storage
- `oci://` - OCI registries
- And many more (file://, hg::, scp://, sftp://, etc.)

**However**, the current implementation has two issues:

```go
// pkg/config/imports.go - Current implementation ONLY checks http/https
func isRemoteImport(importPath string) bool {
    return strings.HasPrefix(importPath, "http://") || strings.HasPrefix(importPath, "https://")
}

// downloadRemoteConfig() DOES use go-getter, but isRemoteImport() blocks other schemes!
```

**Issue 1:** The `isRemoteImport()` function only routes `http://` and `https://` to go-getter, even though `downloadRemoteConfig()` already uses go-getter which supports many more schemes.

**Issue 2:** Some use cases need **transformation** before the content becomes valid YAML:
- `terragrunt://` - Parse HCL (terragrunt.hcl) and convert to YAML
- `mock://` - Generate synthetic YAML for testing without external dependencies

### Challenges

1. **Underutilized go-getter** - go-getter supports many schemes but only http/https are routed to it
2. **No transformation support** - Can't handle schemes that need content transformation (HCL→YAML)
3. **No testing support** - No mock scheme for testing imports without external dependencies
4. **Migration friction** - Users migrating from Terragrunt cannot reference existing terragrunt.hcl files

### Key Requirements

1. **Fix go-getter routing** - Route all go-getter schemes (git::, s3::, gcs::, oci://, etc.) properly
2. **Support import adapters** - Enable `terragrunt://`, `mock://` for content transformation
3. **Maintain backward compatibility** - Existing local and HTTP/HTTPS imports must work unchanged
4. **Support testing** - Mock adapter for unit tests without external dependencies
5. **Preserve recursive imports** - Nested imports continue to work across all schemes

## Functional Requirements

### FR-1: Import Adapter Interface

| ID | Requirement |
|----|-------------|
| FR-1.1 | The system SHALL provide an `ImportAdapter` interface with `Schemes()` and `Resolve()` methods |
| FR-1.2 | The `Schemes()` method SHALL return URL schemes/prefixes the adapter handles (e.g., `["http://", "https://"]`) |
| FR-1.3 | The `Resolve()` method SHALL accept context, import path, base path, temp directory, currentDepth, and maxDepth parameters |
| FR-1.4 | The `Resolve()` method SHALL return a list of resolved file paths and any errors |
| FR-1.5 | Import adapters SHALL be able to generate, transform, or fetch content in custom ways |

### FR-2: Import Adapter Registry

| ID | Requirement |
|----|-------------|
| FR-2.1 | The system SHALL provide a global registry for import adapters |
| FR-2.2 | The registry SHALL support registration via `RegisterImportAdapter()` function |
| FR-2.3 | The registry SHALL prevent duplicate registration of the same scheme |
| FR-2.4 | The registry SHALL normalize schemes to lowercase for case-insensitive matching |
| FR-2.5 | The registry SHALL be thread-safe using appropriate synchronization |
| FR-2.6 | The registry SHALL provide `GetImportAdapter()` to retrieve adapters by scheme |
| FR-2.7 | The registry SHALL provide `ResetImportAdapterRegistry()` for testing purposes |
| FR-2.8 | Adapters SHALL self-register via `init()` functions |

### FR-3: Scheme Detection and Routing

| ID | Requirement |
|----|-------------|
| FR-3.1 | The system SHALL check for registered import adapters before go-getter routing |
| FR-3.2 | The system SHALL route all go-getter schemes (http, https, git::, s3::, gcs::, oci://, etc.) to go-getter |
| FR-3.3 | The system SHALL fall back to local filesystem for paths without recognized schemes |
| FR-3.4 | The `extractScheme()` function SHALL extract the scheme from paths containing "://" |
| FR-3.5 | The `extractScheme()` function SHALL return empty string for paths without schemes |

### FR-4: Mock Import Adapter

| ID | Requirement |
|----|-------------|
| FR-4.1 | The system SHALL provide a `MockAdapter` for testing purposes |
| FR-4.2 | The mock adapter SHALL handle the `mock://` scheme |
| FR-4.3 | `mock://empty` SHALL generate an empty YAML configuration |
| FR-4.4 | `mock://error` SHALL return a simulated error for testing error handling |
| FR-4.5 | `mock://nested` SHALL generate a configuration with nested mock:// imports |
| FR-4.6 | `mock://<path>` SHALL generate a configuration with `mock_path` and `mock_source` vars |
| FR-4.7 | The mock adapter SHALL support custom data injection via `MockData` field |
| FR-4.8 | The mock adapter SHALL write generated YAML to the temp directory |
| FR-4.9 | The mock adapter SHALL use unique filenames to prevent collisions |

### FR-5: go-getter Integration

| ID | Requirement |
|----|-------------|
| FR-5.1 | The `isRemoteImport()` function SHALL recognize all go-getter schemes |
| FR-5.2 | Supported go-getter schemes SHALL include: http://, https://, git::, git@, s3::, s3://, gcs::, gcs://, oci://, file://, hg::, scp://, sftp://, github.com/, bitbucket.org/ |
| FR-5.3 | The `isRemoteImport()` function SHALL return false for import adapter schemes |
| FR-5.4 | The existing `downloadRemoteConfig()` function SHALL continue to use go-getter |

### FR-6: Backward Compatibility

| ID | Requirement |
|----|-------------|
| FR-6.1 | Existing local file imports SHALL continue to work unchanged |
| FR-6.2 | Existing HTTP/HTTPS imports SHALL continue to work unchanged |
| FR-6.3 | Nested/recursive imports SHALL continue to work across all schemes |
| FR-6.4 | The `ResolvedPaths` struct SHALL support a new `ADAPTER` import type |

## Solution: Two-Layer Architecture

### Design Philosophy

Rather than replacing go-getter, we **embrace** it for standard downloads and add a **transformation layer** for schemes that need content conversion:

1. **Layer 1: go-getter** - Handles all standard download schemes (http, https, git::, s3::, gcs::, oci://, etc.)
2. **Layer 2: Transformation Adapters** - Handles schemes that need content transformation before becoming valid YAML

### Architecture Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                       Import Resolution                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  importPath                                                  │
│      │                                                       │
│      ▼                                                       │
│  ┌────────────────────────────────────────────┐             │
│  │     Scheme Detection & Routing             │             │
│  └────────────────────────────────────────────┘             │
│      │                              │                        │
│      │ Transformation Schemes       │ Standard Schemes       │
│      │ (terragrunt://, mock://)     │ (everything else)      │
│      ▼                              ▼                        │
│  ┌─────────────────┐    ┌──────────────────────┐           │
│  │ Transformation  │    │    go-getter         │           │
│  │ Adapter Registry│    │    (existing)        │           │
│  │                 │    │                      │           │
│  │ - mock://       │    │ - http(s)://         │           │
│  │ - terragrunt:// │    │ - git::              │           │
│  │ (future)        │    │ - s3::               │           │
│  │                 │    │ - gcs::              │           │
│  │                 │    │ - oci://             │           │
│  │                 │    │ - file://            │           │
│  │                 │    │ - local paths        │           │
│  └─────────────────┘    └──────────────────────┘           │
│      │                              │                        │
│      │ Generate YAML                │ Download YAML          │
│      ▼                              ▼                        │
│  ┌────────────────────────────────────────────┐             │
│  │              Merge Configuration            │             │
│  └────────────────────────────────────────────┘             │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **go-getter first** - Standard schemes continue using go-getter (our existing investment)
2. **Transformation adapters for special cases** - Only schemes needing transformation use adapters
3. **Minimal surface area** - Small, focused adapter interface
4. **Backward compatibility** - Existing imports work without changes
5. **Testability** - Mock adapter enables testing without external dependencies

## Implementation

### 1. Fix isRemoteImport() to Properly Route go-getter Schemes

The first and most important change is fixing the scheme detection to route all go-getter schemes properly:

```go
// pkg/config/imports.go - UPDATED

// isRemoteImport determines if the import path should use go-getter.
// This includes all schemes that go-getter supports.
func isRemoteImport(importPath string) bool {
    // Check for import adapters first (they handle their own schemes).
    if hasImportAdapter(importPath) {
        return false
    }

    // go-getter schemes (not exhaustive, go-getter handles detection).
    goGetterPrefixes := []string{
        "http://",
        "https://",
        "git::",
        "git@",            // SSH git
        "s3::",
        "s3://",
        "gcs::",
        "gcs://",
        "oci://",
        "file://",
        "hg::",
        "scp://",
        "sftp://",
        "github.com/",     // GitHub shorthand
        "bitbucket.org/",  // Bitbucket shorthand
    }

    lowered := strings.ToLower(importPath)
    for _, prefix := range goGetterPrefixes {
        if strings.HasPrefix(lowered, prefix) {
            return true
        }
    }

    return false
}
```

### 2. Import Adapter Interface (for mock://, terragrunt://)

Import adapters handle custom schemes that need special processing (content transformation, synthetic generation, etc.):

```go
// pkg/config/import_adapter.go
package config

import (
    "context"
)

// ImportAdapter handles custom import schemes that need special processing.
// Unlike go-getter schemes (which download YAML directly), import adapters
// can generate, transform, or fetch content in custom ways.
//
// Examples:
//   - mock:// - Generates synthetic YAML for testing
//   - terragrunt:// - Transforms HCL to YAML (future)
type ImportAdapter interface {
    // Schemes returns URL schemes/prefixes this adapter handles.
    // Return empty slice for default adapter (LocalAdapter).
    // Examples: ["http://", "https://", "git::"] or ["mock://"]
    Schemes() []string

    // Resolve processes an import path and returns YAML file paths.
    //
    // Parameters:
    //   - ctx: Context for cancellation and deadlines
    //   - importPath: The full import path (e.g., "mock://component/vpc")
    //   - basePath: The base path for resolving relative references
    //   - tempDir: Temporary directory for generated files
    //   - currentDepth: Current recursion depth for nested imports
    //   - maxDepth: Maximum allowed recursion depth
    //
    // Returns:
    //   - []ResolvedPaths: List of YAML file paths to merge
    //   - error: Any error encountered during resolution
    Resolve(
        ctx context.Context,
        importPath string,
        basePath string,
        tempDir string,
        currentDepth int,
        maxDepth int,
    ) ([]ResolvedPaths, error)
}
```

### 3. Import Adapter Registry

Small registry for custom import adapters:

```go
// pkg/config/import_adapter_registry.go
package config

import (
    "fmt"
    "strings"
    "sync"
)

var (
    importAdapters = &ImportAdapterRegistry{
        adapters: make(map[string]ImportAdapter),
    }
)

// ImportAdapterRegistry manages import adapter registration.
type ImportAdapterRegistry struct {
    mu       sync.RWMutex
    adapters map[string]ImportAdapter
}

// RegisterImportAdapter adds an import adapter to the registry.
func RegisterImportAdapter(adapter ImportAdapter) error {
    importAdapters.mu.Lock()
    defer importAdapters.mu.Unlock()

    if adapter == nil {
        return fmt.Errorf("import adapter cannot be nil")
    }

    scheme := strings.ToLower(adapter.Scheme())
    if scheme == "" {
        return fmt.Errorf("import adapter scheme cannot be empty")
    }

    if _, exists := importAdapters.adapters[scheme]; exists {
        return fmt.Errorf("import adapter for scheme %q already registered", scheme)
    }

    importAdapters.adapters[scheme] = adapter
    return nil
}

// GetImportAdapter returns an import adapter by scheme.
func GetImportAdapter(scheme string) (ImportAdapter, bool) {
    importAdapters.mu.RLock()
    defer importAdapters.mu.RUnlock()

    adapter, ok := importAdapters.adapters[strings.ToLower(scheme)]
    return adapter, ok
}

// hasImportAdapter checks if there's a registered import adapter for the path.
func hasImportAdapter(importPath string) bool {
    scheme := extractScheme(importPath)
    _, ok := GetImportAdapter(scheme)
    return ok
}

// extractScheme extracts the URL scheme from an import path.
func extractScheme(importPath string) string {
    if idx := strings.Index(importPath, "://"); idx > 0 {
        return strings.ToLower(importPath[:idx])
    }
    return ""
}

// ResetImportAdapterRegistry clears the registry (for testing only).
func ResetImportAdapterRegistry() {
    importAdapters.mu.Lock()
    defer importAdapters.mu.Unlock()
    importAdapters.adapters = make(map[string]ImportAdapter)
}
```

### 4. Mock Import Adapter (For Testing)

```go
// pkg/config/adapters/mock_adapter.go
package adapters

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/cloudposse/atmos/pkg/config"
)

// MockAdapter generates synthetic YAML for testing.
// This enables unit tests without external dependencies.
//
// Usage:
//   - mock://empty        → Empty YAML config
//   - mock://error        → Returns error (test error handling)
//   - mock://nested       → Config with nested mock:// imports
//   - mock://custom/path  → Config with mock_path set to "custom/path"
//
// Tests can inject custom data via MockData field.
type MockAdapter struct {
    // MockData allows tests to inject custom YAML content.
    // Key is the path after "mock://", value is YAML content.
    MockData map[string]string
}

func init() {
    if err := config.RegisterImportAdapter(&MockAdapter{}); err != nil {
        panic(fmt.Sprintf("failed to register mock adapter: %v", err))
    }
}

func (m *MockAdapter) Scheme() string {
    return "mock"
}

func (m *MockAdapter) Resolve(
    ctx context.Context,
    importPath string,
    basePath string,
    tempDir string,
) ([]config.ResolvedPaths, error) {
    mockPath := strings.TrimPrefix(importPath, "mock://")

    switch mockPath {
    case "error":
        return nil, fmt.Errorf("mock error: simulated import failure")
    case "empty":
        return m.writeConfig(importPath, tempDir, "# Empty mock configuration\n")
    case "nested":
        return m.writeConfig(importPath, tempDir, `# Nested mock configuration
import:
  - mock://component/base

vars:
  nested: true
  level: 1
`)
    default:
        // Check for injected mock data.
        if m.MockData != nil {
            if content, ok := m.MockData[mockPath]; ok {
                return m.writeConfig(importPath, tempDir, content)
            }
        }

        // Generate default mock config.
        content := fmt.Sprintf(`# Mock configuration for %s
vars:
  mock_path: "%s"
  mock_source: "mock_adapter"
`, mockPath, mockPath)
        return m.writeConfig(importPath, tempDir, content)
    }
}

func (m *MockAdapter) writeConfig(importPath, tempDir, content string) ([]config.ResolvedPaths, error) {
    fileName := fmt.Sprintf("mock-import-%d.yaml", time.Now().UnixNano())
    filePath := filepath.Join(tempDir, fileName)

    if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
        return nil, fmt.Errorf("failed to write mock config: %w", err)
    }

    return []config.ResolvedPaths{
        {
            FilePath:    filePath,
            ImportPaths: importPath,
            ImportType:  config.ADAPTER,
        },
    }, nil
}
```

### 5. Updated imports.go Integration

```go
// pkg/config/imports.go - Key changes

type importTypes int

const (
    LOCAL   importTypes = 0
    REMOTE  importTypes = 1
    ADAPTER importTypes = 2  // For import adapters (mock://, terragrunt://)
)

func processImports(basePath string, importPaths []string, tempDir string, currentDepth, maxDepth int) (resolvedPaths []ResolvedPaths, err error) {
    // ... existing validation ...

    ctx := context.Background()

    for _, importPath := range importPaths {
        if importPath == "" {
            continue
        }

        var paths []ResolvedPaths
        var err error

        // Check for import adapter first.
        if adapter, ok := GetImportAdapter(extractScheme(importPath)); ok {
            paths, err = adapter.Resolve(ctx, importPath, basePath, tempDir)
        } else if isRemoteImport(importPath) {
            // Use go-getter for remote imports.
            paths, err = processRemoteImport(basePath, importPath, tempDir, currentDepth, maxDepth)
        } else {
            // Local filesystem import.
            paths, err = processLocalImport(basePath, importPath, tempDir, currentDepth, maxDepth)
        }

        if err != nil {
            log.Debug("failed to resolve import", "path", importPath, "error", err)
            continue
        }

        resolvedPaths = append(resolvedPaths, paths...)
    }

    return resolvedPaths, nil
}
```

## Use Cases

### Use Case 1: Mock Adapter for Testing

```go
// pkg/config/imports_test.go
func TestImportResolution(t *testing.T) {
    // Inject custom mock data for tests.
    mockAdapter := &adapters.MockAdapter{
        MockData: map[string]string{
            "base/defaults": `vars:
  cidr: "10.0.0.0/16"
  environment: test`,
        },
    }
    config.ResetImportAdapterRegistry()
    config.RegisterImportAdapter(mockAdapter)

    // Test import resolution.
    paths, err := processImports(basePath, []string{"mock://base/defaults"}, tempDir, 1, 25)
    assert.NoError(t, err)
    assert.Len(t, paths, 1)
    assert.Equal(t, config.ADAPTER, paths[0].ImportType)
}
```

### Use Case 2: Nested Mock Imports

```yaml
# Importing "mock://nested" generates config with further imports:
# import:
#   - mock://component/base
# vars:
#   nested: true

# This enables testing complex import chains without external dependencies.
```

### Use Case 3: All go-getter Schemes Now Work

With the fixed `isRemoteImport()`:

```yaml
import:
  # All of these now work (they already were supposed to!)
  - git::https://github.com/acme/configs//stacks/catalog/vpc?ref=v1.0.0
  - s3::https://s3.amazonaws.com/acme-configs/stacks/catalog/vpc.yaml
  - gcs::https://storage.googleapis.com/acme-configs/stacks/catalog/eks.yaml
  - oci://registry.example.com/configs:v1.0.0
  - github.com/acme/configs//stacks/defaults
```

### Use Case 4: Future Terragrunt Adapter (Phase 2)

```yaml
# stacks/migration.yaml - During Terragrunt migration
import:
  # Transform Terragrunt HCL to YAML on-the-fly
  - terragrunt://legacy/prod/vpc/terragrunt.hcl

  # Mix with native Atmos imports
  - _defaults/globals
```

## Testing Strategy

### 1. Import Adapter Registry Tests

```go
// pkg/config/import_adapter_registry_test.go

func TestRegisterImportAdapter(t *testing.T) {
    ResetImportAdapterRegistry()

    adapter := &MockAdapter{}
    err := RegisterImportAdapter(adapter)
    assert.NoError(t, err)
}

func TestExtractScheme(t *testing.T) {
    tests := []struct {
        path   string
        scheme string
    }{
        {"mock://test", "mock"},
        {"terragrunt://path/to/file", "terragrunt"},
        {"http://example.com", "http"},
        {"/absolute/path", ""},       // No scheme
        {"relative/path", ""},         // No scheme
    }

    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            assert.Equal(t, tt.scheme, extractScheme(tt.path))
        })
    }
}
```

### 2. Mock Adapter Tests

```go
// pkg/config/adapters/mock_adapter_test.go

func TestMockAdapterResolve(t *testing.T) {
    adapter := &MockAdapter{}
    tempDir := t.TempDir()

    tests := []struct {
        name       string
        importPath string
        wantErr    bool
    }{
        {"empty config", "mock://empty", false},
        {"error simulation", "mock://error", true},
        {"nested config", "mock://nested", false},
        {"custom path", "mock://component/vpc", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            paths, err := adapter.Resolve(context.Background(), tt.importPath, "/base", tempDir)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Len(t, paths, 1)
            }
        })
    }
}
```

### 3. isRemoteImport Tests

```go
func TestIsRemoteImport(t *testing.T) {
    tests := []struct {
        path     string
        expected bool
    }{
        // go-getter schemes
        {"http://example.com/config.yaml", true},
        {"https://example.com/config.yaml", true},
        {"git::https://github.com/org/repo//path", true},
        {"s3::https://s3.amazonaws.com/bucket/key", true},
        {"gcs::https://storage.googleapis.com/bucket/key", true},
        {"oci://registry.example.com/image:tag", true},
        {"github.com/org/repo//path", true},

        // Local paths
        {"/absolute/path", false},
        {"relative/path", false},
        {"./current/path", false},

        // Import adapters (not go-getter)
        {"mock://test", false},
        {"terragrunt://path", false},
    }

    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            assert.Equal(t, tt.expected, isRemoteImport(tt.path))
        })
    }
}
```

## Implementation Plan

### Phase 1: Fix go-getter Routing + Mock Adapter

**Goal:** Route all go-getter schemes properly and add mock:// for testing.

**Tasks:**
1. Update `isRemoteImport()` to recognize all go-getter schemes
2. Create `pkg/config/import_adapter.go` with `TransformationAdapter` interface
3. Create `pkg/config/import_adapter_registry.go` with registry
4. Implement `MockAdapter` in `pkg/config/adapters/mock_adapter.go`
5. Update `processImports()` to check transformation adapters first
6. Add comprehensive tests

**Files to modify:**
- `pkg/config/imports.go` - Fix `isRemoteImport()`, add transformation adapter check
- `pkg/config/import_adapter.go` - New: TransformationAdapter interface
- `pkg/config/import_adapter_registry.go` - New: Registry functions
- `pkg/config/adapters/mock_adapter.go` - New: Mock adapter implementation
- `pkg/config/adapters/mock_adapter_test.go` - New: Mock adapter tests
- `pkg/config/imports_test.go` - Add isRemoteImport and integration tests

**Success Criteria:**
- ✅ All go-getter schemes (git::, s3::, gcs::, oci://, etc.) work for imports
- ✅ `mock://` adapter generates synthetic YAML for testing
- ✅ Existing local/http imports unchanged
- ✅ 90%+ test coverage on new code

### Phase 2: Terragrunt Adapter (Future)

**Goal:** Transform terragrunt.hcl to YAML for migration support.

**Tasks:**
1. Create `TerragruntAdapter` that parses HCL
2. Transform Terragrunt `inputs` → Atmos `vars`
3. Transform `include` → `import`
4. Handle `dependency` blocks

### Phase 3: Additional Import Adapters (Future)

Potential future adapters:
- `consul://` - Fetch and transform Consul KV to YAML
- `vault://` - Fetch and transform Vault secrets to YAML
- Custom format converters

**Note:** s3://, git::, gcs::, oci:// already work via go-getter - no adapters needed.

## Benefits

### Immediate Benefits (Phase 1)

1. ✅ **Full go-getter support** - All documented schemes now work (git::, s3::, oci://, etc.)
2. ✅ **Testability** - Mock adapter enables testing without external dependencies
3. ✅ **Minimal changes** - Small, focused transformation adapter layer
4. ✅ **Backward compatibility** - Leverages existing go-getter investment

### Future Benefits (Phase 2+)

5. ✅ **Migration support** - Terragrunt users can reference existing HCL files
6. ✅ **Format flexibility** - Can add converters for other config formats

## Risks & Mitigation

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing imports | Low | High | Comprehensive backward compatibility tests |
| go-getter scheme detection wrong | Low | Medium | Test all documented schemes |
| Mock adapter generates invalid YAML | Low | Low | Validate generated YAML in tests |

## Success Criteria

### Phase 1
- ✅ `isRemoteImport()` returns true for all go-getter schemes
- ✅ `mock://` adapter generates valid YAML
- ✅ Nested mock imports work (mock://nested)
- ✅ All existing tests pass
- ✅ 90%+ test coverage on new code

## FAQ

### Q: Why not replace go-getter with adapters for everything?

**A:** go-getter is battle-tested and supports many schemes (git::, s3::, gcs::, oci://, etc.). We have significant investment in it. Import adapters are only for schemes that need special processing (content transformation, synthetic generation), not for standard downloads.

### Q: Why was s3://, git::, etc. not working before?

**A:** Bug in `isRemoteImport()` - it only checked for `http://` and `https://` prefixes, even though `downloadRemoteConfig()` uses go-getter which supports many more schemes.

### Q: How does the mock adapter help testing?

**A:** It generates synthetic YAML without external files or network. Tests can inject custom content via `MockData` field. Special paths like `mock://error` simulate failures.

### Q: What's the routing order?

**A:**
1. Check for import adapter (mock://, terragrunt://)
2. Check if go-getter scheme (http, https, git::, s3::, oci://, etc.)
3. Fall back to local filesystem

## References

- [Command Registry Pattern PRD](command-registry-pattern.md)
- [Component Registry Pattern PRD](component-registry-pattern.md)
- [Store Registry Implementation](../../pkg/store/registry.go)
- [Current imports.go](../../pkg/config/imports.go)
- [Terragrunt Migration Guide](../../website/docs/migration/terragrunt.mdx)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-18 | Initial PRD with mock adapter implementation |
