# PRD: Toolchain Registry Search and Discovery

## Overview

Users need the ability to discover available tools across configured registries without manually browsing registry URLs. This PRD defines commands for searching and listing tools in toolchain registries.

**Key Architecture Decisions:**
- Commands organized under `atmos toolchain registry` subcommand group
- Functionality implemented as `ToolRegistry` interface methods (`Search`, `ListAll`, `GetMetadata`)
- Cache layer as transparent wrapper around any registry implementation
- `atmos toolchain search` provided as convenience alias to `registry search`

## Problem Statement

Currently, users must:
- Know the exact `owner/repo` format to install tools
- Manually browse registry URLs (e.g., aquaproj/aqua-registry) to discover available tools
- Guess tool names or rely on external documentation

This creates friction in the tool discovery workflow and reduces the value of the multi-registry architecture.

## Goals

1. **Search**: Enable fuzzy search across all configured registries
2. **List**: Display all available tools from a specific registry
3. **Info**: Show detailed metadata for a specific tool before installation
4. **Performance**: Cache registry data for fast local searches
5. **UX**: Present results in a clean, scannable format with relevant metadata

## Non-Goals

- Web-based registry browser UI (CLI only)
- Registry hosting/publishing (consumption only)
- Tool recommendations/ratings (future enhancement)
- Full-text search of tool descriptions (V2 feature)

## User Stories

### As a developer, I want to search for tools by name
```bash
atmos toolchain registry search terraform
atmos toolchain search terraform           # alias to registry search
atmos toolchain registry search kubectl --registry aqua-public
```

**Expected Output:**
```
Found 3 tools matching "terraform":

  TOOL                      REGISTRY        DESCRIPTION
  hashicorp/terraform       aqua-public     Infrastructure as Code tool
  opentofu/opentofu         aqua-public     Open-source Terraform fork
  gruntwork-io/terragrunt   aqua-public     Terraform wrapper

Use 'atmos toolchain info <tool>' for details
Use 'atmos toolchain install <tool>@<version>' to install
```

### As a developer, I want to list all tools from a registry
```bash
atmos toolchain registry list aqua-public
atmos toolchain registry list acme-corporate --limit 50
atmos toolchain registry list              # lists all registries if no name given
```

**Expected Output (with registry name):**
```
Tools in registry 'aqua-public' (showing 50 of 1,247):

  TOOL                      ALIASES           DESCRIPTION
  hashicorp/terraform       terraform, tf     Infrastructure as Code
  kubernetes/kubectl        kubectl, k        Kubernetes CLI
  helm/helm                 helm              Kubernetes package manager
  ...

Use --limit to show more results
Use 'atmos toolchain registry search <query>' to filter
```

**Expected Output (without registry name):**
```
Configured registries:

  NAME              TYPE    PRIORITY  SOURCE                                    TOOLS
  acme-corporate    aqua    100       https://registry.acme.example.com/...     47
  internal-mirror   aqua    50        https://mirror.internal.example.com/...   1,247
  aqua-public       aqua    10        https://github.com/aquaproj/aqua-...      1,247

Use 'atmos toolchain registry list <name>' to see tools in a registry
```

### As a developer, I want to see tool details before installing
```bash
atmos toolchain info hashicorp/terraform
atmos toolchain info terraform  # resolves via alias
```

**Expected Output:**
```
Tool: hashicorp/terraform
Registry: aqua-public (priority: 10)
Aliases: terraform, tf

Description:
  Terraform is an infrastructure as code tool that lets you build,
  change, and version cloud and on-prem resources safely and efficiently.

Repository: https://github.com/hashicorp/terraform
Homepage: https://www.terraform.io
License: BSL-1.1

Available Versions (latest 10):
  ✓ 1.13.4   (installed, default)
    1.13.3
    1.13.2
    ...

Install:
  atmos toolchain install terraform@1.13.4
  atmos toolchain install terraform@latest
```

## Technical Design

### Command Structure

```
atmos toolchain registry list [registry-name] [flags]
atmos toolchain registry search <query> [flags]
atmos toolchain search <query> [flags]      # alias to 'registry search'
atmos toolchain info <tool> [flags]
```

**Command Hierarchy:**
```
atmos toolchain
├── registry
│   ├── list [name]     # List registries or tools in a registry
│   └── search <query>  # Search across registries
├── search <query>      # Alias to 'registry search'
└── info <tool>         # Tool details (existing location)
```

### Commands

#### `atmos toolchain registry list [registry-name]`

**Purpose:** List configured registries, or list all tools from a specific registry.

**Arguments:**
- `[registry-name]` (optional): Registry name from `atmos.yaml`. If omitted, lists all configured registries.

**Flags:**
- `--limit <n>`: Maximum results to show (default: 50, only applies when listing tools)
- `--offset <n>`: Skip first N results (pagination, only applies when listing tools)
- `--format <json|yaml|table>`: Output format (default: table)
- `--sort <name|date|popularity>`: Sort order (default: name, only applies when listing tools)

**Implementation:**
- **Without registry name:** Show table of all configured registries from `atmos.yaml` with metadata
- **With registry name:** Fetch complete registry manifest and display tools
- Cache registry manifest locally for performance (TTL: 24 hours)
- Show registry metadata (type, source URL, priority, tool count)
- Paginate large result sets
- Implements interface method: `ToolRegistry.ListAll()`

#### `atmos toolchain registry search <query>`

**Purpose:** Fuzzy search across all configured registries.

**Arguments:**
- `<query>` (required): Search query (tool name, owner, repo, or alias)

**Flags:**
- `--limit <n>`: Maximum results to show (default: 20)
- `--registry <name>`: Search only in specific registry
- `--format <json|yaml|table>`: Output format (default: table)
- `--installed-only`: Show only installed tools matching query
- `--available-only`: Show only non-installed tools

**Implementation:**
- Search across all registries by default (respects priority order)
- Match against: owner, repo, aliases, description (if available)
- Case-insensitive fuzzy matching
- Results sorted by relevance score, then alphabetically
- Cache registry metadata locally (TTL: 24 hours)
- Implements interface method: `ToolRegistry.Search(query)`
- Deduplicates results across registries (shows highest priority)

#### `atmos toolchain search <query>`

**Purpose:** Alias to `atmos toolchain registry search`.

**Implementation:**
- Simple command alias that delegates to `atmos toolchain registry search`
- Accepts all the same flags

#### `atmos toolchain info <tool>`

**Purpose:** Show detailed metadata for a specific tool.

**Arguments:**
- `<tool>` (required): Tool name (owner/repo, alias, or repo name)

**Flags:**
- `--registry <name>`: Prefer specific registry if tool exists in multiple
- `--versions <n>`: Number of versions to show (default: 10)
- `--all-versions`: Show all available versions
- `--format <json|yaml|table>`: Output format (default: table)

**Implementation:**
- Resolve tool via existing resolver infrastructure
- Fetch tool metadata from registry API
- Display installed status for each version
- Show which versions are in `.tool-versions`

### Registry Cache

**Cache Location:** `.tools/cache/registry/`

**Cache Structure:**
```
.tools/cache/registry/
  aqua-public/
    manifest.yaml      # Full registry manifest
    metadata.json      # Cache metadata (timestamp, TTL)
  acme-corporate/
    manifest.yaml
    metadata.json
```

**Cache Invalidation:**
- TTL: 24 hours (configurable via `ATMOS_TOOLCHAIN_CACHE_TTL`)
- Manual refresh: `atmos toolchain refresh-cache [registry-name]`
- Automatic refresh on cache miss or expired TTL

### Interface Design

**Extended `ToolRegistry` Interface** (in `toolchain/registry/registry.go`)

All registry implementations (Aqua, Atmos, etc.) MUST implement these methods:

```go
type ToolRegistry interface {
    // Existing methods (already implemented)
    GetTool(owner, repo string) (*Tool, error)
    GetToolWithVersion(owner, repo, version string) (*Tool, error)
    GetLatestVersion(owner, repo string) (string, error)

    // New methods for discovery (to be implemented)
    Search(ctx context.Context, query string, opts ...SearchOption) ([]*Tool, error)
    ListAll(ctx context.Context, opts ...ListOption) ([]*Tool, error)
    GetMetadata(ctx context.Context) (*RegistryMetadata, error)
}

// RegistryMetadata contains registry-level information.
type RegistryMetadata struct {
    Name        string
    Type        string    // "aqua", "atmos"
    Source      string    // URL
    Priority    int
    ToolCount   int
    LastUpdated time.Time
}

// SearchOption configures search behavior.
type SearchOption func(*SearchConfig)

type SearchConfig struct {
    Limit          int
    Offset         int
    InstalledOnly  bool
    AvailableOnly  bool
}

// ListOption configures list behavior.
type ListOption func(*ListConfig)

type ListConfig struct {
    Limit  int
    Offset int
    Sort   string // "name", "date", "popularity"
}
```

**Options Pattern Examples:**

```go
// Search with options
results, err := registry.Search(ctx, "terraform",
    WithLimit(20),
    WithInstalledOnly(true),
)

// List with options
tools, err := registry.ListAll(ctx,
    WithLimit(50),
    WithOffset(100),
    WithSort("name"),
)
```

**Cache Layer** (in `toolchain/registry/cache/cache.go`)

The cache layer wraps any `ToolRegistry` implementation:

```go
package cache

// CachedRegistry wraps a ToolRegistry with caching.
type CachedRegistry struct {
    registry ToolRegistry
    store    CacheStore
    ttl      time.Duration
}

// NewCachedRegistry creates a cached registry wrapper.
func NewCachedRegistry(reg ToolRegistry, opts ...CacheOption) *CachedRegistry {
    // ...
}

// Implements ToolRegistry interface with caching.
func (c *CachedRegistry) Search(ctx context.Context, query string, opts ...SearchOption) ([]*Tool, error) {
    // Check cache first
    // If miss or expired, call underlying registry
    // Update cache
}

// CacheStore handles persistence.
type CacheStore interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, data []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Clear(ctx context.Context) error
}

// FileCacheStore implements filesystem-based caching.
type FileCacheStore struct {
    basePath string
}
```

**Usage in Commands:**

```go
// In cmd/toolchain/registry/search.go
func executeSearch(cmd *cobra.Command, args []string) error {
    // Load registries from config
    registries := loadRegistries()

    // Wrap each with cache layer
    cachedRegistries := make([]ToolRegistry, len(registries))
    for i, reg := range registries {
        cachedRegistries[i] = cache.NewCachedRegistry(reg,
            cache.WithTTL(24*time.Hour),
        )
    }

    // Create composite registry for searching across all
    composite := registry.NewCompositeRegistry(cachedRegistries)

    // Execute search
    results, err := composite.Search(ctx, query, registry.WithLimit(limit))
    // ...
}
```

### Search Algorithm

**Relevance Scoring:**
1. Exact match on alias: score = 100
2. Exact match on repo name: score = 90
3. Prefix match on repo name: score = 70
4. Prefix match on owner: score = 50
5. Contains match on repo name: score = 30
6. Contains match in description: score = 10

**Deduplication:**
- If same tool exists in multiple registries, show highest priority registry
- Indicate alternate registries in detailed view

### Error Handling

**Common Errors:**
- Registry not found: `Error: registry 'foo' not configured in atmos.yaml`
- Tool not found: `Error: tool 'foo' not found in any registry`
- Cache fetch failure: `Warning: failed to fetch registry cache, using stale data`
- Network timeout: `Error: failed to reach registry source (timeout after 30s)`

**Graceful Degradation:**
- If cache is stale but network fails, use stale cache with warning
- If search returns 0 results, suggest `--all-versions` or `list-registry`
- If registry is unreachable, skip it and search remaining registries

## Implementation Phases

### Phase 1: Core Search (MVP)
- `atmos toolchain search` command
- Basic fuzzy matching on tool names
- Table output format
- Local cache implementation

### Phase 2: Registry Listing
- `atmos toolchain list-registry` command
- Pagination support
- JSON/YAML output formats

### Phase 3: Tool Info
- `atmos toolchain info` command
- Enhanced metadata display
- Version availability checking

### Phase 4: Cache Management
- `atmos toolchain refresh-cache` command
- Configurable TTL
- Cache statistics

### Phase 5: Enhanced Search (V2)
- Full-text search in descriptions
- Filter by license, language, category
- Interactive selection with arrow keys (bubbletea)

## Testing Strategy

**Unit Tests:**
- Search algorithm scoring
- Cache expiration logic
- Result deduplication
- Alias resolution

**Integration Tests:**
- Search across multiple registries
- Cache read/write operations
- Network failure scenarios
- Stale cache handling

**CLI Tests:**
- Command flag parsing
- Output format validation
- Error message clarity
- Table rendering

## Documentation

**User Documentation:**
- Add to `website/docs/cli/commands/atmos_toolchain_registry.md` (parent page)
- Add to `website/docs/cli/commands/atmos_toolchain_registry_list.md`
- Add to `website/docs/cli/commands/atmos_toolchain_registry_search.md`
- Add to `website/docs/cli/commands/atmos_toolchain_search.md` (alias page)
- Update existing `website/docs/cli/commands/atmos_toolchain_info.md`
- Update `website/docs/core-concepts/toolchain.md` with discovery examples

**Developer Documentation:**
- Document registry interface extensions in code comments
- Document cache layer architecture in `docs/toolchain-registry-cache.md`
- Document search algorithm scoring in code comments
- Add discovery workflow examples to `docs/developing-atmos-commands.md`

## Success Metrics

**User Experience:**
- Users can discover tools without leaving CLI: 100%
- Average time to find a tool: <10 seconds
- Search result relevance: >90% (manual review)

**Performance:**
- Search response time: <500ms (cached)
- Cache hit rate: >80%
- Registry list load time: <2s for 1000+ tools

**Adoption:**
- `search` command usage: >30% of users
- `info` command usage before install: >50%

## Future Enhancements

**Post-V1:**
1. **Interactive Mode:** Arrow-key navigation with `bubbletea`
2. **Tool Recommendations:** "Users who installed X also installed Y"
3. **Registry Statistics:** Most popular tools, recent additions
4. **Custom Filters:** Filter by license, language, category tags
5. **Bookmarks:** Save favorite tools for quick access
6. **Comparison View:** Compare similar tools side-by-side

## References

- **Aqua Registry Format:** https://aquaproj.github.io/docs/reference/registry-config
- **Similar Tools:**
  - `brew search` (Homebrew)
  - `apt search` (Debian)
  - `asdf plugin list all` (asdf)
- **Related PRDs:**
  - `toolchain-lock-file.md`
  - `command-registry-pattern.md`
