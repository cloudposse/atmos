# PRD: Multi-Registry Support for Toolchain

## Status
- **Status**: Partially Implemented (Phases 1-2 complete, Phases 3-5 pending)
- **Author**: Claude Code
- **Created**: 2025-10-24
- **Updated**: 2025-12-13
- **Related**: `docs/prd/command-registry-pattern.md`, `toolchain/registry/aqua/README.md`

## Overview

Extend Atmos toolchain to support multiple registry sources, enabling private/corporate registries, custom tool distributions, and air-gapped environments while maintaining backward compatibility with the existing single Aqua registry approach.

## Motivation

### Current Limitations

**Single Registry Source**
- Atmos currently hardcodes the Aqua standard registry URL
- No support for private/corporate registries
- Cannot use custom registry sources
- Limited to publicly available tools in Aqua registry

**Enterprise Use Cases Not Supported**
- Companies with internal/proprietary tools cannot use Atmos toolchain
- Air-gapped environments cannot mirror registries
- Organizations cannot enforce tool policies via custom registries
- No support for registry precedence/override

**Aqua Feature Parity**
- Aqua supports multiple registry types (standard, local, github_content)
- Aqua allows registry-specific package resolution
- We're missing this flexibility despite using Aqua's registry format

### Benefits of Multi-Registry Support

1. **Private/Corporate Tools**: Companies can maintain internal tool registries
2. **Air-Gapped Deployments**: Mirror registries for offline environments
3. **Security & Compliance**: Control tool sources and versions
4. **Registry Precedence**: Local/corporate registries override public ones
5. **Flexibility**: Mix public and private tools seamlessly

## Goals

### Primary Goals
- Support multiple registry sources with configurable precedence
- Support different registry types (Aqua standard, local, GitHub content, custom URLs)
- Maintain backward compatibility with existing single-registry configuration
- Enable private/corporate registry scenarios

### Non-Goals
- Not replacing Aqua registry format (we still use their YAML structure)
- Not implementing registry authentication in v1 (use GitHub tokens via environment)
- Not supporting non-Aqua registry formats initially

## Design

### Architecture

Our recently implemented registry interface pattern makes this extension clean:

```
toolchain/
├── registry/
│   ├── registry.go          # ToolRegistry interface (no changes)
│   ├── aqua/                # Aqua implementation
│   ├── composite.go         # NEW: Multi-registry coordinator
│   └── github_content.go    # NEW: GitHub content registry
└── config.go                # NEW: Registry configuration
```

### Registry Types

#### 1. Standard Registry (Current)
```yaml
# atmos.yaml
toolchain:
  registries:
    - type: standard  # Aqua standard registry
      ref: main       # Optional: git ref/tag
```

#### 2. Local Registry
```yaml
toolchain:
  registries:
    - name: corporate
      type: local
      path: .atmos/toolchain-registry.yaml  # Relative to atmos.yaml
```

#### 3. GitHub Content Registry
```yaml
toolchain:
  registries:
    - name: corporate
      type: github_content
      repo_owner: mycompany
      repo_name: toolchain-registry
      ref: v1.2.0
      path: registry.yaml
```

#### 4. Custom URL Registry
```yaml
toolchain:
  registries:
    - name: mirror
      type: url
      base_url: https://registry.example.com/tools
```

### Configuration Schema

```yaml
# atmos.yaml
toolchain:
  # Configuration options
  versions_file: .tool-versions
  install_path: .tools

  # New multi-registry configuration
  registries:
    # First registry has highest precedence
    - name: corporate      # Optional name
      type: local
      path: tools/registry.yaml
      priority: 100        # Optional: explicit priority (higher = checked first)

    - name: github-corp
      type: github_content
      repo_owner: mycompany
      repo_name: tools
      ref: main
      path: aqua-registry.yaml
      priority: 50

    - type: standard      # Aqua standard (fallback)
      ref: main
      priority: 0         # Lowest priority

  # Tool-specific registry override
  tools:
    terraform:
      registry: corporate  # Use specific registry for this tool
```

### Interface Implementation

#### Composite Registry Pattern

```go
// pkg/toolchain/registry/composite.go

// CompositeRegistry coordinates multiple registry sources.
type CompositeRegistry struct {
	registries []PrioritizedRegistry
	local      *LocalConfigManager
}

type PrioritizedRegistry struct {
	Name     string
	Registry ToolRegistry
	Priority int
}

// GetTool tries registries in priority order.
func (cr *CompositeRegistry) GetTool(owner, repo string) (*Tool, error) {
	// Sort by priority (descending)
	sort.Slice(cr.registries, func(i, j int) bool {
		return cr.registries[i].Priority > cr.registries[j].Priority
	})

	// Try local first
	if tool, exists := cr.local.GetTool(owner, repo); exists {
		return cr.local.GetToolWithVersion(owner, repo, "")
	}

	// Try each registry in priority order
	var lastErr error
	for _, pr := range cr.registries {
		tool, err := pr.Registry.GetTool(owner, repo)
		if err == nil {
			return tool, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("%w: %s/%s not found in any registry: %v",
		ErrToolNotFound, owner, repo, lastErr)
}
```

#### GitHub Content Registry

```go
// pkg/toolchain/registry/github_content.go

// GitHubContentRegistry fetches registry from GitHub repository.
type GitHubContentRegistry struct {
	owner  string
	repo   string
	ref    string
	path   string
	client *http.Client
	cache  *registryCache
}

func NewGitHubContentRegistry(owner, repo, ref, path string) *GitHubContentRegistry {
	return &GitHubContentRegistry{
		owner:  owner,
		repo:   repo,
		ref:    ref,
		path:   path,
		client: httpClient.NewHTTPClient(),
		cache:  newRegistryCache(),
	}
}

func (gcr *GitHubContentRegistry) GetTool(owner, repo string) (*Tool, error) {
	// Construct GitHub raw content URL
	url := fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/%s/pkgs/%s/%s/registry.yaml",
		gcr.owner, gcr.repo, gcr.ref, gcr.path, owner, repo,
	)

	// Fetch and parse (similar to Aqua implementation)
	// ...
}
```

### Configuration Loading

```go
// pkg/toolchain/config.go

type RegistryConfig struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"` // standard, local, github_content, url
	Priority int    `yaml:"priority" json:"priority"`

	// Standard registry fields
	Ref string `yaml:"ref" json:"ref,omitempty"`

	// Local registry fields
	Path string `yaml:"path" json:"path,omitempty"`

	// GitHub content registry fields
	RepoOwner string `yaml:"repo_owner" json:"repo_owner,omitempty"`
	RepoName  string `yaml:"repo_name" json:"repo_name,omitempty"`

	// URL registry fields
	BaseURL string `yaml:"base_url" json:"base_url,omitempty"`
}

func LoadRegistries(configs []RegistryConfig) (ToolRegistry, error) {
	var registries []PrioritizedRegistry

	for _, cfg := range configs {
		var reg ToolRegistry

		switch cfg.Type {
		case "standard":
			reg = aqua.NewAquaRegistry() // Existing implementation
		case "local":
			reg = NewLocalRegistry(cfg.Path)
		case "github_content":
			reg = NewGitHubContentRegistry(cfg.RepoOwner, cfg.RepoName, cfg.Ref, cfg.Path)
		case "url":
			reg = NewURLRegistry(cfg.BaseURL)
		default:
			return nil, fmt.Errorf("unsupported registry type: %s", cfg.Type)
		}

		registries = append(registries, PrioritizedRegistry{
			Name:     cfg.Name,
			Registry: reg,
			Priority: cfg.Priority,
		})
	}

	return NewCompositeRegistry(registries), nil
}
```

### Backward Compatibility

**No configuration → Standard Aqua registry (current behavior)**
```yaml
# No toolchain.registries configured
# Defaults to standard Aqua registry
```

**Explicit standard registry**
```yaml
toolchain:
  registries:
    - type: standard  # Explicitly use Aqua standard
```

**Mixed configuration**
```yaml
toolchain:
  registries:
    - type: local
      path: custom-tools.yaml
    - type: standard  # Falls back to Aqua
```

## Implementation Plan

### Phase 1: Foundation ✅ Complete
- [x] Define registry configuration schema in `pkg/schema/schema.go`
- [x] Create `CompositeRegistry` coordinator
- [x] Update `toolchain` package to use composite registry
- [x] Maintain backward compatibility (default to standard registry)
- [x] Unit tests for composite registry

### Phase 2: Registry Types ✅ Complete
- [x] Implement `LocalRegistry` (reads from file path)
- [x] Implement `URLRegistry` (custom base URLs)
- [x] Tests for each registry type
- [ ] Implement `GitHubContentRegistry` (fetches from GitHub repos) - Deferred

### Phase 3: Configuration & Integration (TODO)
- [ ] Add JSON schema validation for toolchain.registries
- [ ] Add integration tests with actual registry configurations
- [ ] Test priority precedence in real scenarios

### Phase 4: CLI & Documentation (TODO)
- [ ] Add `atmos toolchain registry list` command
- [ ] Add `atmos toolchain registry validate` command
- [ ] Update documentation in `website/docs/`
- [ ] Add examples for common scenarios
- [ ] Blog post about multi-registry support

### Phase 5: Advanced Features (Future)
- [ ] Registry authentication/credentials
- [ ] Registry mirroring/caching
- [ ] Registry health checks
- [ ] Metrics on registry usage

**Total Estimate**: Phases 1-2 complete. Phases 3-5 pending.

## Testing Strategy

### Unit Tests
```go
func TestCompositeRegistry_Priority(t *testing.T) {
	// Test that higher priority registries are checked first
}

func TestCompositeRegistry_Fallback(t *testing.T) {
	// Test fallback to lower priority when tool not found
}

func TestGitHubContentRegistry(t *testing.T) {
	// Mock GitHub API responses
}

func TestBackwardCompatibility(t *testing.T) {
	// Test that no config → standard registry
}
```

### Integration Tests
```go
func TestMultiRegistry_CorporateOverride(t *testing.T) {
	// Test that corporate registry overrides standard for same tool
}

func TestMultiRegistry_MixedSources(t *testing.T) {
	// Test tools from different registries in same workflow
}
```

### Manual Testing Scenarios
1. **Corporate Registry**: Private GitHub repo with custom tools
2. **Air-Gapped**: Local registry file only
3. **Mixed**: Local overrides + standard fallback
4. **Migration**: Existing atmos.yaml (no registries config)

## Example Use Cases

### Use Case 1: Corporate Tools (Single Index File)
```yaml
# Company has internal tools in a single registry file
toolchain:
  registries:
    - name: acme-corp
      type: aqua
      source: file://./corporate-registry.yaml  # Single file with all tools
      priority: 100

    - name: aqua-public
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

### Use Case 2: Air-Gapped Environment (Local Index File)
```yaml
# All tools defined locally in single file, no internet access
toolchain:
  registries:
    - name: offline
      type: aqua
      source: file://.atmos/offline-registry.yaml
      priority: 100
```

### Use Case 3: Registry Mirror (Directory Structure)
```yaml
# Use internal mirror with directory structure for performance/compliance
toolchain:
  registries:
    - name: internal-mirror
      type: aqua
      source: https://registry.internal.example.com/pkgs/  # Directory of per-tool files
      priority: 100

    - name: aqua-public
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

### Use Case 4: Development Overrides (Multiple Registries)
```yaml
# Developers can override with local versions using priority
toolchain:
  registries:
    - name: dev-overrides
      type: aqua
      source: file://~/.atmos/dev-tools.yaml  # Personal overrides (highest priority)
      priority: 200

    - name: corporate
      type: aqua
      source: https://github.com/mycompany/tools/tree/main/registry.yaml
      priority: 100

    - name: aqua-public
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

### Use Case 5: Inline Registry (Quick Prototyping) ✅
```yaml
# Define tools directly in atmos.yaml without external files
toolchain:
  registries:
    - name: my-inline-tools
      type: atmos
      priority: 150
      tools:
        stedolan/jq:
          type: github_release
          url: "jq-{{.OS}}-{{.Arch}}"
        example/internal-tool:
          type: http
          url: "https://internal.example.com/{{trimV .Version}}/tool.zip"

    - name: aqua-public
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

## Security Considerations

### Registry Trust
- **Standard registry**: Trusted by default (Aqua project)
- **GitHub content**: Requires explicit configuration (user responsibility)
- **Local registry**: Trusted (under user control)
- **URL registry**: User must ensure HTTPS and trust

### Authentication
- **Phase 1**: Use existing `GITHUB_TOKEN` environment variable
- **Future**: Registry-specific credentials in secure storage

### Policy Validation
- Consider integration with Atmos policy validation
- Validate registry sources against allowlist
- Audit which registries are used

## Documentation Requirements

### User Documentation
1. **Configuration Guide**: How to configure multiple registries
2. **Registry Types Guide**: Details on each registry type
3. **Migration Guide**: Moving from single to multi-registry
4. **Security Best Practices**: Trust and authentication

### Developer Documentation
1. **Architecture**: Registry interface and composite pattern
2. **Adding Registry Types**: How to implement new registry types
3. **Testing**: How to test with multiple registries

### Examples
1. Corporate registry setup
2. Air-gapped deployment
3. Development environment with overrides
4. Registry mirroring

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing configs | High | Maintain backward compatibility; default to standard registry |
| Complex configuration | Medium | Provide clear examples; sensible defaults |
| Registry authentication | Medium | Start with GitHub token; expand later |
| Performance with multiple registries | Low | Cache aggressively; check high-priority first |
| Registry availability | Medium | Implement fallback; clear error messages |

## Success Metrics

1. **Adoption**: Number of users configuring multiple registries
2. **Use Cases**: Corporate vs air-gapped vs mixed
3. **Performance**: No regression in single-registry case
4. **Compatibility**: Zero breaking changes for existing users

## Open Questions

1. **Should we support Aqua's policy file format?** Aqua v2+ requires policies for non-standard registries
2. **Registry health checks?** Should we ping registries to check availability?
3. **Registry caching strategy?** How long to cache registry metadata?
4. **Conflict resolution?** If same tool exists in multiple registries with different metadata?

## Future Enhancements

- **Registry synchronization**: Sync corporate registry with standard
- **Registry analytics**: Track which registries are used most
- **Registry validation**: Validate registry YAML before use
- **Registry templates**: Scaffold new corporate registries
- **Registry versioning**: Pin registry versions for reproducibility

## Implementation Notes

### Completed Work (2025-10-24)

#### Phase 1: Foundation ✅
- ✅ Defined registry configuration schema in `pkg/schema/schema.go`
  - Added `ToolchainRegistry` struct with fields: Name, Type, Source, Priority, Tools
- ✅ Created `CompositeRegistry` coordinator in `toolchain/registry/composite.go`
  - Implements priority-based precedence (higher priority checked first)
  - Checks local config first, then registries by priority
  - Comprehensive test coverage
- ✅ Implemented registry loader in `toolchain/registry/loader.go`
  - Factory pattern with type-based registry creation
  - Aqua package registers itself via `init()` to avoid circular dependencies
- ✅ Updated toolchain package with `NewRegistry()` wrapper
- ✅ Maintained backward compatibility
  - No registries configured → defaults to standard Aqua registry
  - Transparent migration path for existing configurations

#### Phase 2: Registry Types ✅
- ✅ Implemented `URLRegistry` in `toolchain/registry/url.go`
  - Fetches from custom base URLs
  - In-memory caching
  - Follows Aqua registry structure
  - **Two registry patterns** (auto-detected):
    1. **Single index file** (source ends with `.yaml`/`.yml`) - all packages in one file
    2. **Directory structure** (source doesn't end with file extension) - per-tool files at `{source}/{owner}/{repo}/registry.yaml` or `{source}/{repo}/registry.yaml`
  - Single index file pattern matches official Aqua registry structure
  - Pattern detection eliminates need for explicit configuration
- ⏸️ LocalRegistry: Already exists as `LocalConfigManager` in `toolchain/registry/registry.go`
- ⏸️ GitHubContentRegistry: Deferred to future phase

### Configuration Format

The final implemented configuration format differs slightly from the initial proposal to better align with Atmos conventions:

```yaml
# atmos.yaml
toolchain:
  registries:
    - name: corporate        # Optional: registry name for identification
      type: aqua            # aqua, url, atmos
      source: https://...   # Optional: custom source URL
      priority: 100         # Higher = checked first

    - name: internal-mirror
      type: url
      source: https://registry.internal.example.com/pkgs
      priority: 50
```

**Key Design Decisions**:
1. **Simplified type system**: `aqua` (standard or custom source), `url` (generic base URL), `atmos` (inline/future)
2. **Source field**: Combined multiple type-specific fields into single `source` field
3. **Priority-based**: Explicit priority values instead of implicit order
4. **Local config precedence**: Local `tools.yaml` always has highest priority (implicit)

### Architecture

```
toolchain/
├── registry/
│   ├── registry.go          # ToolRegistry interface + LocalConfigManager
│   ├── aqua/
│   │   ├── aqua.go         # Aqua registry implementation + auto-registration
│   │   └── aqua_test.go
│   ├── composite.go         # Multi-registry coordinator with priority
│   ├── composite_test.go    # Comprehensive unit tests
│   ├── url.go               # Custom URL registry
│   └── loader.go            # Registry factory and configuration loader
└── aqua_registry.go         # Backward-compatible wrappers
```

### Testing Coverage

- ✅ CompositeRegistry unit tests (7 tests)
  - Priority-based resolution
  - Fallback to lower priority
  - Local config precedence
  - Error handling
- ✅ Aqua registry tests (26 tests) - All passing
- ✅ Integration with existing toolchain tests - All passing

### Remaining Work

#### Phase 3: Configuration Finalization (TODO)
- [ ] Add JSON schema validation for toolchain.registries
- [ ] Add integration tests with actual registry configurations
- [ ] Test priority precedence in real scenarios

#### Phase 4: Documentation (TODO)
- [ ] Create user documentation in `website/docs/`
- [ ] Add configuration examples
- [ ] Document registry types and use cases
- [ ] Add troubleshooting guide

#### Phase 5: Future Enhancements (Deferred)
- [ ] Implement GitHubContentRegistry for direct GitHub repo access
- [ ] Implement inline AtmosRegistry (tools defined in atmos.yaml)
- [ ] Add `atmos toolchain registry list` command
- [ ] Add `atmos toolchain registry validate` command
- [ ] Registry authentication/credentials management
- [ ] Registry health checks and monitoring

## References

- [Aqua Registry Documentation](https://aquaproj.github.io/docs/reference/registry/)
- [Aqua Develop Registry Guide](https://aquaproj.github.io/docs/develop-registry/)
- [Template Private Aqua Registry](https://github.com/aquaproj/template-private-aqua-registry)
- Atmos Registry Interface: `toolchain/registry/registry.go`
- Atmos Registry PRD: `docs/prd/command-registry-pattern.md`
