# Atmos Registry PRD

## Overview

The Atmos Registry feature provides a unified interface for discovering and searching Terraform/OpenTofu modules from multiple registry sources. This enables users to find modules across different registries (OpenTofu, GitHub organizations, and future Terraform Registry support) using a single CLI command.

## Problem Statement

Users need to discover Terraform modules from various sources:
- **OpenTofu Registry** (`api.opentofu.org`) - Community modules
- **GitHub Organizations** - Private/enterprise modules in org repositories
- **Artifactory Terraform Registry** - Enterprise private module registries
- **Terraform Registry** (`registry.terraform.io`) - HashiCorp's registry (future)

Each registry has different APIs and search mechanisms, making it difficult to:
1. Search across multiple sources uniformly
2. Configure organization-specific module repositories
3. Filter by naming patterns or custom properties

## Goals

1. **Unified search interface** - Single `atmos registry search` command for all registries
2. **Multi-provider support** - Abstract different registry APIs behind a common interface
3. **GitHub organization support** - Search repos by name pattern, regex, or custom properties
4. **Configurable registries** - Define registries in `atmos.yaml` with provider-specific options
5. **Extensible architecture** - Easy to add new registry providers

## Non-Goals

- Module installation/vendoring (covered by existing `atmos vendor` command)
- Module version management
- Module publishing

---

## Architecture

### Registry Interface

The core abstraction that all registry providers implement:

```go
// Registry defines the interface for interacting with module registries.
type Registry interface {
    // Search queries the registry for modules matching the given criteria.
    Search(ctx context.Context, opts SearchOptions) (*SearchResult, error)

    // Name returns the registry name for display purposes.
    Name() string

    // BaseURL returns the base URL of the registry.
    BaseURL() string
}

// SearchOptions contains parameters for searching modules.
type SearchOptions struct {
    Query     string // Search term (required)
    Limit     int    // Max results (default: 20)
    Offset    int    // Pagination offset (default: 0)
    Provider  string // Filter by provider (Terraform registry only)
    Namespace string // Filter by namespace (Terraform registry only)
}

// SearchResult contains the search response.
type SearchResult struct {
    Modules []Module   // Matching modules
    Meta    SearchMeta // Pagination info
}
```

### Package Structure

```
pkg/registry/
├── registry.go              # Core Registry interface
├── types.go                 # Module, SearchResult, SearchMeta types
├── errors.go                # Sentinel errors
├── options.go               # Functional options for configuration
├── opentofu/
│   ├── opentofu.go          # OpenTofu registry implementation
│   └── types.go             # OpenTofu API response types
├── github/
│   ├── github.go            # GitHub org registry implementation
│   └── types.go             # GitHub-specific types
├── artifactory/
│   ├── artifactory.go       # Artifactory Terraform Registry implementation
│   └── types.go             # Artifactory-specific types
└── terraform/               # Future implementation
    └── terraform.go

cmd/registry/
├── registry.go              # CommandProvider registration
└── search.go                # Search subcommand
```

---

## Module Search Strategy

### Overview

Different registries expose completely different search APIs. The strategy is to implement provider-specific clients that normalize results into the common `SearchResult` format.

### Provider Implementations

#### 1. OpenTofu Registry

**API Details:**
- Base URL: `https://api.opentofu.org`
- Search endpoint: `/registry/docs/search?q=<query>`
- No server-side pagination (client-side required)

**Response Format:**
```json
{
  "results": [
    {
      "id": "namespace/name/provider",
      "namespace": "hashicorp",
      "name": "consul",
      "provider": "aws",
      "version": "1.0.0",
      "description": "Module description",
      "rank": 0.95,
      "term_match_count": 3
    }
  ]
}
```

**Implementation Notes:**
- Fetch all results, apply client-side pagination
- Use `rank` and `term_match_count` for sorting relevance
- No authentication required

#### 2. GitHub Organization Registry

**API Details:**
- Uses `go-github` SDK (existing `pkg/github/` infrastructure)
- Authentication: `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN` environment variables
- Rate limiting: Handled by existing `handleGitHubAPIError()` function

**Search Methods:**

1. **List by Organization** - `client.Repositories.ListByOrg()`
   - Fetches all repos in org, filters client-side
   - Better for small-medium orgs

2. **Search Repositories** - `client.Search.Repositories()`
   - Uses GitHub search API with `org:` qualifier
   - Better for large orgs with many repos

**Filtering Options (mutually exclusive, regex takes precedence):**

| Option | Description | Example |
|--------|-------------|---------|
| `pattern` | Glob-style wildcards | `terraform-aws-*` |
| `regex` | Full Go regex | `^terraform-(aws\|gcp)-.*$` |
| `properties` | GitHub custom properties (Enterprise) | `module-type: terraform` |

**Pattern to Regex Conversion:**
- `*` → `.*` (match any characters)
- `?` → `.` (match single character)
- Other characters escaped for regex safety

**Custom Properties (GitHub Enterprise):**
- Endpoint: `GET /orgs/{org}/properties/values`
- Search qualifier: `props.{property}:{value}`
- Requires GitHub Enterprise or GitHub.com with custom properties enabled

#### 3. Artifactory Terraform Registry

**Overview:**
JFrog Artifactory supports private Terraform module registries implementing the Terraform Module Registry Protocol. This enables enterprises to host private modules with access control.

**API Details:**
- Implements Terraform Module Registry Protocol
- List versions endpoint: `GET /api/terraform/<repo>/<namespace>/<name>/<provider>/versions`
- Download endpoint: `GET /api/terraform/<repo>/<namespace>/<name>/<provider>/<version>/download`
- No native search API - must list repository contents and filter client-side

**Authentication:**
- Uses existing Atmos patterns from `pkg/store/artifactory_store.go`
- Token precedence: Config `access_token` → `ARTIFACTORY_ACCESS_TOKEN` → `JFROG_ACCESS_TOKEN`
- Anonymous access supported with token value `"anonymous"`
- Uses JFrog SDK (`github.com/jfrog/jfrog-client-go/artifactory`)

**Module Path Convention:**
```
<repo>/<namespace>/<module-name>/<provider-name>/<version>.zip
```

**Search Implementation:**
Since Artifactory lacks a native search API, search is implemented by:
1. Listing all items in the configured repository path
2. Parsing module metadata from directory structure
3. Filtering by search query client-side
4. Optionally using Artifactory's AQL (Artifactory Query Language) for advanced filtering

**Response Format (versions endpoint):**
```json
{
  "modules": [{
    "versions": [
      {"version": "1.0.0"},
      {"version": "1.1.0"}
    ]
  }]
}
```

#### 4. Terraform Registry (Future)

**API Details:**
- Base URL: `https://registry.terraform.io`
- Search endpoint: `/v1/modules/search`
- Server-side pagination supported

**Parameters:**
| Param | Description |
|-------|-------------|
| `q` | Search query |
| `limit` | Results per page (max 100) |
| `offset` | Pagination offset |
| `provider` | Filter by provider (aws, gcp, etc.) |
| `namespace` | Filter by namespace |
| `verified` | Filter to verified modules only |

**Response Format:**
```json
{
  "meta": {
    "limit": 20,
    "current_offset": 0,
    "next_offset": 20
  },
  "modules": [
    {
      "id": "namespace/name/provider",
      "namespace": "hashicorp",
      "name": "consul",
      "provider": "aws",
      "version": "1.0.0",
      "description": "Module description",
      "verified": true,
      "downloads": 12345
    }
  ]
}
```

---

## Configuration

### Schema

```yaml
registry:
  # Default registry for searches without --registry flag
  default: opentofu

  # Named registry configurations
  registries:
    # OpenTofu public registry
    opentofu:
      kind: opentofu
      base_url: https://api.opentofu.org  # Optional, has default
      timeout: 30                          # Seconds
      enabled: true

    # GitHub organization with glob pattern
    cloudposse:
      kind: github
      org: cloudposse
      pattern: "terraform-aws-*"           # Glob pattern
      timeout: 30
      enabled: true

    # GitHub organization with regex
    cloudposse-multi:
      kind: github
      org: cloudposse
      regex: "^terraform-(aws|gcp)-.*$"    # Full regex
      enabled: true

    # GitHub Enterprise with custom properties
    enterprise-modules:
      kind: github
      org: my-enterprise-org
      properties:                          # GitHub custom properties
        module-type: "terraform"
        status: "production"
      enabled: true

    # Artifactory Terraform Registry
    artifactory-modules:
      kind: artifactory
      url: https://artifactory.example.com/artifactory
      repo: terraform-modules            # Repository name
      prefix: modules                     # Optional path prefix
      # Auth: ARTIFACTORY_ACCESS_TOKEN or JFROG_ACCESS_TOKEN env var
      timeout: 30
      enabled: true

    # Terraform registry (future)
    # terraform:
    #   kind: terraform
    #   base_url: https://registry.terraform.io
    #   timeout: 30
    #   enabled: true
```

### Schema Types

```go
// RegistryConfig contains module registry configuration.
type RegistryConfig struct {
    // Default is the default registry name for searches.
    Default string `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`

    // Registries is a map of named registry configurations.
    Registries map[string]RegistryEntry `yaml:"registries,omitempty" json:"registries,omitempty" mapstructure:"registries"`
}

// RegistryEntry defines configuration for a single registry.
type RegistryEntry struct {
    // Kind is the registry type (opentofu, github, artifactory, terraform).
    Kind string `yaml:"kind" json:"kind" mapstructure:"kind"`

    // BaseURL is the base URL for the registry API (optional).
    BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty" mapstructure:"base_url"`

    // URL is the Artifactory URL (artifactory kind only).
    URL string `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`

    // Timeout is the HTTP timeout in seconds (default: 30).
    Timeout int `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`

    // Enabled controls whether this registry is active.
    Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`

    // GitHub-specific fields
    Org        string            `yaml:"org,omitempty" json:"org,omitempty" mapstructure:"org"`
    Pattern    string            `yaml:"pattern,omitempty" json:"pattern,omitempty" mapstructure:"pattern"`
    Regex      string            `yaml:"regex,omitempty" json:"regex,omitempty" mapstructure:"regex"`
    Properties map[string]string `yaml:"properties,omitempty" json:"properties,omitempty" mapstructure:"properties"`

    // Artifactory-specific fields
    Repo   string `yaml:"repo,omitempty" json:"repo,omitempty" mapstructure:"repo"`
    Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty" mapstructure:"prefix"`
}
```

---

## CLI Command

### Usage

```bash
atmos registry search [flags] <query>
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--registry` | `-r` | (from config) | Registry to search |
| `--provider` | `-p` | | Filter by provider (OpenTofu/Terraform) |
| `--limit` | `-l` | 20 | Maximum results |
| `--offset` | `-o` | 0 | Pagination offset |
| `--format` | `-f` | table | Output format (table, json, yaml) |

### Examples

```bash
# Search default registry
atmos registry search vpc

# Search specific registry
atmos registry search --registry cloudposse vpc
atmos registry search --registry opentofu s3

# Filter by provider (OpenTofu/Terraform registries)
atmos registry search --provider aws lambda

# Output formats
atmos registry search vpc --format json
atmos registry search vpc --format yaml

# Pagination
atmos registry search vpc --limit 10 --offset 20

# GitHub org search
atmos registry search --registry cloudposse eks
# → Searches cloudposse org for repos matching "terraform-aws-*" containing "eks"

# GitHub Enterprise with custom properties
atmos registry search --registry enterprise-modules vpc
# → Searches repos with properties module-type=terraform, status=production

# Artifactory private registry
atmos registry search --registry artifactory-modules networking
# → Searches Artifactory terraform-modules repo for modules containing "networking"
```

### Output Format

**Table (default):**
```
MODULE                          VERSION   PROVIDER   DESCRIPTION
cloudposse/terraform-aws-vpc    1.2.3     aws        Terraform module for AWS VPC
hashicorp/consul                0.15.0    aws        Consul cluster on AWS
...

Showing 20 of 156 results
```

**JSON:**
```json
{
  "modules": [...],
  "meta": {
    "limit": 20,
    "current_offset": 0,
    "next_offset": 20,
    "total_count": 156
  }
}
```

---

## Error Handling

### Sentinel Errors

| Error | Description | User Hint |
|-------|-------------|-----------|
| `ErrRegistryNotFound` | Registry name not in config | Check `atmos.yaml` registry configuration |
| `ErrRegistrySearchFailed` | Search operation failed | Check network connection |
| `ErrRegistryConnectionFailed` | Cannot connect to registry | Verify registry URL and network |
| `ErrRegistryRateLimited` | Rate limit exceeded | Set GITHUB_TOKEN for higher limits |
| `ErrInvalidSearchQuery` | Empty or invalid query | Provide a search term |
| `ErrInvalidRegexPattern` | Invalid regex in config | Check regex syntax |
| `ErrArtifactoryAuthFailed` | Artifactory authentication failed | Check ARTIFACTORY_ACCESS_TOKEN or JFROG_ACCESS_TOKEN |

### Error Builder Pattern

```go
return errUtils.Build(registry.ErrRegistrySearchFailed).
    WithCause(err).
    WithExplanation("Failed to search OpenTofu registry").
    WithContext("query", opts.Query).
    WithHint("Check your network connection and try again").
    Err()
```

---

## Implementation Phases

### Phase 1: Core Package
- [ ] Create `pkg/registry/registry.go` - Interface definitions
- [ ] Create `pkg/registry/types.go` - Common types (Module, SearchResult)
- [ ] Create `pkg/registry/errors.go` - Sentinel errors
- [ ] Create `pkg/registry/options.go` - Functional options

### Phase 2: OpenTofu Provider
- [ ] Create `pkg/registry/opentofu/opentofu.go` - Client implementation
- [ ] Create `pkg/registry/opentofu/types.go` - API response types
- [ ] Add unit tests with mocked HTTP client

### Phase 3: GitHub Provider
- [ ] Create `pkg/registry/github/github.go` - Client implementation
- [ ] Create `pkg/registry/github/types.go` - GitHub-specific types
- [ ] Implement glob-to-regex conversion
- [ ] Implement custom properties support
- [ ] Add unit tests with mocked GitHub client

### Phase 3.5: Artifactory Provider
- [ ] Create `pkg/registry/artifactory/artifactory.go` - Client implementation
- [ ] Create `pkg/registry/artifactory/types.go` - Artifactory-specific types
- [ ] Reuse JFrog SDK patterns from `pkg/store/artifactory_store.go`
- [ ] Implement repository listing and client-side search
- [ ] Add unit tests with mocked Artifactory client

### Phase 4: CLI Command
- [ ] Create `cmd/registry/registry.go` - CommandProvider
- [ ] Create `cmd/registry/search.go` - Search subcommand
- [ ] Add blank import to `cmd/root.go`
- [ ] Add output formatting (table, JSON, YAML)

### Phase 5: Configuration
- [ ] Update `pkg/schema/schema.go` with RegistryConfig
- [ ] Update JSON schemas in `pkg/datafetcher/schema/`
- [ ] Add configuration loading and validation

### Phase 6: Documentation
- [ ] Create Docusaurus docs at `website/docs/cli/commands/registry/`
- [ ] Update CLI help text
- [ ] Add configuration examples

---

## Testing Strategy

### Unit Tests
- Mock HTTP client for OpenTofu API calls
- Mock GitHub client for org searches
- Mock Artifactory client for repository listing
- Test glob-to-regex conversion
- Test pagination logic
- Test error handling

### Integration Tests
- Test with real OpenTofu registry (rate-limited)
- Test configuration loading
- Test CLI output formats

### Test Coverage Target
- 80% minimum coverage
- All error paths tested
- All configuration variations tested

---

## Security Considerations

1. **GitHub Token Handling**
   - Tokens read from environment variables only
   - Never logged or displayed
   - Automatic masking via `pkg/io` infrastructure

2. **Rate Limiting**
   - Respect GitHub API rate limits
   - Provide clear error messages when limits exceeded
   - Suggest token usage for higher limits

3. **Input Validation**
   - Validate regex patterns before compilation
   - Sanitize search queries
   - Validate configuration values

4. **Artifactory Token Handling**
   - Tokens read from environment variables: `ARTIFACTORY_ACCESS_TOKEN`, `JFROG_ACCESS_TOKEN`
   - Support for anonymous access with `"anonymous"` token value
   - Automatic masking via `pkg/io` infrastructure

---

## Future Considerations

1. **Terraform Registry Support**
   - Add `terraform` provider kind
   - Support verified module filtering
   - Support namespace filtering

2. **Caching**
   - Cache search results with TTL
   - Reduce API calls for repeated searches

3. **Interactive Mode**
   - TUI for browsing search results
   - Select and vendor directly from search
   - See "Interactive TUI Search" section below

4. **Version Listing**
   - `atmos registry versions <module>` command
   - List available versions for a module

---

## Interactive TUI Search (Future Enhancement)

Borrow patterns from existing Atmos TUI components for an interactive search experience.

### Existing Patterns to Reuse

**Pager Search** (`pkg/pager/model.go`):
- `/` key to enter search mode
- `n`/`N` for next/previous match navigation
- Yellow highlight (`#FFFF00`) for search matches
- Case-insensitive search with ANSI code stripping
- Incremental search updates while typing

```go
// Highlight style from pager
highlightStyle = lipgloss.NewStyle().
    Background(lipgloss.Color("#FFFF00")).
    Foreground(lipgloss.Color("#000000")).
    Bold(true).
    Render
```

**Multi-Column List** (`internal/tui/atmos/model.go`):
- Three-column layout using `bubbles/list`
- Mouse support via `bubblezone`
- Arrow key navigation between columns
- Dynamic filtering based on selection

### Proposed Interactive Search UX

```
┌─ Registries ─────┬─ Modules ─────────────────┬─ Details ────────────────┐
│ > opentofu       │ > hashicorp/consul/aws    │ Version: 0.15.0          │
│   cloudposse     │   hashicorp/vault/aws     │ Provider: aws            │
│   artifactory    │   cloudposse/vpc/aws      │ Downloads: 12,345        │
│                  │   cloudposse/eks/aws      │                          │
│                  │                           │ Description:             │
│                  │   Search: vpc_            │ Consul cluster on AWS... │
└──────────────────┴───────────────────────────┴──────────────────────────┘
  ←/→ columns  ↑/↓ navigate  / search  enter select  q quit
```

### Key Bindings

| Key | Action |
|-----|--------|
| `←` `→` | Switch column focus |
| `↑` `↓` | Navigate items |
| `/` | Enter search mode |
| `n` | Next search match |
| `N` | Previous search match |
| `Enter` | Select module (copy source to clipboard or vendor) |
| `q` `Esc` | Quit |

### Implementation Notes

1. **Reuse existing TUI infrastructure**
   - `internal/tui/atmos/` for column layout
   - `pkg/pager/` for search functionality
   - `bubbles/list` for list rendering

2. **State Management**
   - Column pointer for focus tracking
   - Search term with incremental filtering
   - Selected registry determines module list

3. **Integration with CLI**
   - `atmos registry search --interactive` flag
   - Falls back to table output in non-TTY environments
