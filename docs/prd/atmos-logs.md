# PRD: `atmos logs` - Cloud-Native Log Streaming

## Executive Summary

Add `atmos logs` command for streaming, searching, and tailing logs from cloud infrastructure. Uses **log providers** (like `aws/cloudwatch`, `aws/cloudtrail`, `github/actions`) that piggyback on **atmos auth identities** for authentication. Architecture mirrors `atmos auth` extensibility pattern.

---

## 1. Problem Statement

Infrastructure operators need to view logs from deployed components but must currently:
- Know provider-specific log locations (log group names, workflow run IDs)
- Use separate CLI tools (aws logs, gh run view)
- Manually correlate components to their log sources
- Handle authentication separately for each log source
- Context-switch between infrastructure definitions and observability tools

## 2. Goals

1. **Stack-aware logging**: `atmos logs --stack dev --component api` finds and streams logs
2. **Identity-integrated**: Piggyback on `atmos auth` identities for credentials
3. **Multiple log providers per platform**: `aws/cloudwatch`, `aws/cloudtrail`, `github/actions`
4. **Direct access mode**: `atmos logs --kind aws/cloudwatch --log-group /aws/lambda/foo`
5. **Real-time streaming**: `--follow` flag for live tail
6. **Interactive TUI**: `--interactive` for fuzzy search, pinning, navigation
7. **Multi-component**: `--component vpc,api` or `--component 'api-*'` patterns

## 3. Non-Goals (Phase 1)

- Azure/GCP providers (architecture supports them, not implemented in v1)
- Cross-provider unified query language (expose native capabilities)
- Log aggregation/storage (use existing providers)
- Log metrics/alerting configuration

## 4. User Experience

### 4.1 Command Structure

```bash
# Component mode (uses component logs.sources configuration)
atmos logs view --stack dev-us-east-1 --component api-gateway
atmos logs view --stack dev-us-east-1 --component api-gateway --source audit  # specific source
atmos logs tail --stack prod --component 'lambda-*' --follow
atmos logs search --stack staging --component vpc --query "ERROR" --since 1h

# Direct mode (uses named sources from atmos.yaml logs.sources)
atmos logs view --source lambda-functions
atmos logs tail --source ecs-services --follow
atmos logs view --source cloudtrail-audit --since 24h
atmos logs view --source github-deploy --run-id 12345

# Interactive TUI
atmos logs --interactive
atmos logs --stack dev --interactive
```

**Key design decision:** Provider-specific options (log group, event name, repo, etc.) are configured in `atmos.yaml`, not passed as CLI flags. This avoids the complexity of documenting provider-specific flags and keeps the CLI simple.

### 4.1.1 Global Log Sources (`atmos.yaml`)

Named log sources can be defined globally for direct access without a component:

```yaml
# atmos.yaml
logs:
  settings:
    default_since: 15m
    format: pretty

  sources:
    # Named sources for direct access (atmos logs view --source lambda-functions)
    lambda-functions:
      kind: aws/cloudwatch
      identity: dev-admin
      spec:
        log_group: "/aws/lambda/*"

    ecs-services:
      kind: aws/cloudwatch
      identity: dev-admin
      spec:
        log_group: "/aws/ecs/*"

    cloudtrail-audit:
      kind: aws/cloudtrail
      identity: security-readonly
      spec:
        event_source: "*"

    github-deploy:
      kind: github/actions
      spec:
        repo: cloudposse/infrastructure
        workflow: deploy.yml
```

### 4.2 Component Configuration (Top-Level `logs` Block)

Logs configuration is **top-level** in component config (not under `settings`).
Structure mirrors auth with `settings` + `sources`:

```yaml
# stacks/catalog/api-gateway.yaml
components:
  terraform:
    api-gateway:
      vars:
        name: my-api
      # Top-level logs block (parallel to vars, settings, env)
      logs:
        settings:                       # <-- optional component-level settings
          default_since: 1h

        sources:                        # <-- map of log sources (like auth.identities)
          application:                  # <-- name is the key
            kind: aws/cloudwatch
            default: true               # <-- default log source
            identity: dev-admin         # Uses atmos auth identity
            spec:
              log_group: "/aws/apigateway/{{ .vars.name }}"
              stream_prefix: "api/"
          audit:
            kind: aws/cloudtrail
            identity: security-readonly
            spec:
              event_source: apigateway.amazonaws.com
          ci:
            kind: github/actions
            spec:
              repo: cloudposse/infrastructure
              workflow: deploy-api.yml
```

### 4.3 Output Formats

```bash
# Pretty (default) - colored, formatted for terminals
atmos logs view --stack dev --component api

# JSON - for piping/processing
atmos logs view --stack dev --component api --format json | jq '.level == "ERROR"'

# Raw - just the message, no metadata
atmos logs view --stack dev --component api --format raw

# Log - output via atmos logger (respects atmos log settings)
atmos logs view --stack dev --component api --format log
```

The `log` format routes output through the atmos logging system, enabling:
- Log level filtering (respects `logs.level` in atmos.yaml)
- File output (respects `logs.file` in atmos.yaml)
- Integration with existing log aggregation pipelines
- Consistent formatting with other atmos log output

## 5. Log Provider Architecture

### 5.1 Provider Interface

```go
// pkg/logs/types/interfaces.go

// Provider is the core interface all log providers implement.
type Provider interface {
    Kind() string                                    // "aws/cloudwatch", "aws/cloudtrail", "github/actions"
    Name() string                                    // Instance name from config
    Capabilities() CapabilitySet                     // What this provider can do
    ListSources(ctx, opts) ([]LogSource, error)      // Discover available log sources
    GetLogs(ctx, source, opts) (*LogResult, error)   // Fetch logs
    Validate() error                                 // Validate configuration
    Close() error                                    // Cleanup
}

// StreamingProvider extends Provider with real-time tailing.
type StreamingProvider interface {
    Provider
    TailLogs(ctx, source, opts) (<-chan LogEntry, <-chan error)
}

// QueryProvider extends Provider with advanced query support.
type QueryProvider interface {
    Provider
    ExecuteQuery(ctx, query) (*QueryResult, error)
    QueryLanguage() string  // "CloudWatch Insights", "KQL", etc.
}

// ProviderFactory creates provider instances from configuration.
// Each provider implementation registers a factory function.
type ProviderFactory func(name string, spec map[string]any, creds types.ICredentials) (Provider, error)
```

### 5.2 Registry Pattern (Self-Registration)

Uses the **registry pattern** (like `cmd/internal/registry.go`) instead of a central factory switch statement. Providers self-register via `init()` functions, making the system extensible without modifying central code.

```go
// pkg/logs/registry/registry.go

package registry

import (
    "fmt"
    "sync"

    "github.com/cloudposse/atmos/pkg/logs/types"
)

// ProviderRegistry manages log provider registration.
// Providers register themselves during package initialization.
var registry = &ProviderRegistry{
    factories: make(map[string]types.ProviderFactory),
}

type ProviderRegistry struct {
    mu        sync.RWMutex
    factories map[string]types.ProviderFactory
}

// Register adds a provider factory to the registry.
// Called by providers in their init() functions.
//
// Example usage in pkg/logs/providers/aws/cloudwatch.go:
//
//     func init() {
//         registry.Register("aws/cloudwatch", NewCloudWatchProvider)
//     }
func Register(kind string, factory types.ProviderFactory) {
    registry.mu.Lock()
    defer registry.mu.Unlock()
    registry.factories[kind] = factory
}

// NewProvider creates a provider instance by kind.
// Returns an error if the kind is not registered.
func NewProvider(kind string, name string, spec map[string]any, creds types.ICredentials) (types.Provider, error) {
    registry.mu.RLock()
    factory, ok := registry.factories[kind]
    registry.mu.RUnlock()

    if !ok {
        return nil, fmt.Errorf("%w: %s", ErrUnknownProviderKind, kind)
    }
    return factory(name, spec, creds)
}

// ListKinds returns all registered provider kinds.
func ListKinds() []string {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    kinds := make([]string, 0, len(registry.factories))
    for kind := range registry.factories {
        kinds = append(kinds, kind)
    }
    return kinds
}
```

### 5.3 Provider Self-Registration

Each provider registers itself during package initialization:

```go
// pkg/logs/providers/aws/cloudwatch.go

package aws

import (
    "github.com/cloudposse/atmos/pkg/logs/registry"
    "github.com/cloudposse/atmos/pkg/logs/types"
)

func init() {
    registry.Register("aws/cloudwatch", NewCloudWatchProvider)
}

func NewCloudWatchProvider(name string, spec map[string]any, creds types.ICredentials) (types.Provider, error) {
    // ... implementation
}
```

```go
// pkg/logs/providers/aws/cloudtrail.go

package aws

func init() {
    registry.Register("aws/cloudtrail", NewCloudTrailProvider)
}
```

```go
// pkg/logs/providers/github/actions.go

package github

func init() {
    registry.Register("github/actions", NewActionsProvider)
}
```

### 5.4 Blank Imports Enable Providers

Providers are enabled via blank imports in a central registration file:

```go
// pkg/logs/providers/providers.go

package providers

// Import all built-in providers.
// Each provider registers itself via init().
import (
    _ "github.com/cloudposse/atmos/pkg/logs/providers/aws"
    _ "github.com/cloudposse/atmos/pkg/logs/providers/github"
    // Future: _ "github.com/cloudposse/atmos/pkg/logs/providers/azure"
    // Future: _ "github.com/cloudposse/atmos/pkg/logs/providers/gcp"
)
```

**Benefits of Registry Pattern:**
- **Extensible**: Adding a new provider only requires the provider package + blank import
- **Decoupled**: No central switch statement to modify
- **Plugin-friendly**: Future plugin system can register external providers
- **Testable**: Easy to register mock providers for testing
- **Consistent**: Matches `cmd/internal/registry.go` pattern used for commands

**Note**: This pattern should also be adopted for `atmos auth` in a future refactor.

### 5.5 Provider Kinds (Phase 1)

| Kind | Log Identifier | Filter Syntax | Special Features |
|------|---------------|---------------|------------------|
| `aws/cloudwatch` | Log Group + Stream | Filter Patterns, Insights QL | Live Tail API |
| `aws/cloudtrail` | Event Source + Name | Lookup attributes | Management/Data events |
| `github/actions` | Repo + Workflow/Run | Job/Step filtering | Live workflow streaming |

### 5.6 Future Provider Kinds (Not in Phase 1)

| Kind | Description |
|------|-------------|
| `azure/monitor` | Azure Log Analytics with KQL |
| `gcp/logging` | Cloud Logging with resource hierarchy |
| `datadog/logs` | Datadog Log Management |
| `aws/s3-access` | S3 server access logs |

## 6. Configuration Schema

### 6.1 Global Configuration (`atmos.yaml`)

Following the auth pattern which has `auth.logs` for baseline settings alongside `auth.providers` and `auth.identities`:

```yaml
# atmos.yaml
logs:
  settings:                    # <-- baseline settings (like auth.logs)
    default_since: 15m
    format: pretty             # pretty, json, raw, log
    timestamp_format: "2006-01-02T15:04:05Z07:00"
    colorize: true
```

This mirrors auth's structure:
- `auth.logs` = baseline settings
- `auth.providers` = map of providers
- `auth.identities` = map of identities

For logs:
- `logs.settings` = baseline settings
- `logs.sources` = map of log sources (on components, not global)

### 6.2 Component Configuration (Top-Level `logs`)

```yaml
# stacks/catalog/lambda-api.yaml
components:
  terraform:
    lambda-api:
      vars:
        function_name: api-handler

      # Top-level logs block - NOT under settings
      logs:
        settings:                         # <-- component-level settings (optional, overrides global)
          default_since: 30m              # Override global default

        sources:                          # <-- map of log sources (like auth.identities)
          function-logs:                  # <-- name is the key
            kind: aws/cloudwatch
            default: true                 # <-- default log source (like auth identity default)
            identity: dev-admin           # Uses atmos auth identity
            spec:
              log_group: "/aws/lambda/{{ .vars.function_name }}"

          api-gateway-logs:
            kind: aws/cloudwatch
            identity: dev-admin
            spec:
              log_group: "/aws/apigateway/{{ .vars.function_name }}"
              stream_prefix: "api/"

          audit-trail:
            kind: aws/cloudtrail
            identity: security-readonly
            spec:
              event_source: lambda.amazonaws.com
              event_name: Invoke

          deployments:
            kind: github/actions
            spec:
              repo: "{{ .settings.github_repo }}"
              workflow: deploy.yml
```

### 6.3 Identity Integration

Log providers piggyback on `atmos auth` identities:

```yaml
# atmos.yaml
auth:
  identities:
    dev-admin:
      kind: aws/permission-set
      default: true           # Default auth identity
      # ... auth config
    security-readonly:
      kind: aws/assume-role
      # ... auth config
```

```yaml
# Component logs reference auth identities
logs:
  sources:
    function-logs:
      kind: aws/cloudwatch
      default: true           # Default log source for this component
      identity: dev-admin     # References auth identity (uses default if omitted)
      spec:
        log_group: "/aws/lambda/..."
```

Authentication flow:
1. Resolve identity from log source config
2. If no identity specified, use the `default: true` auth identity
3. Get credentials via `authManager.GetCachedCredentials(ctx, identityName)`
4. If no cached creds, prompt for auth or fail gracefully
5. Pass credentials to log provider factory

## 7. Technical Architecture

### 7.1 Package Structure

```
pkg/logs/
├── logs.go                  # Service struct (entry point)
├── manager.go               # LogManager (like auth/manager.go)
├── config_helpers.go        # Config loading/merging
├── types/
│   ├── interfaces.go        # Provider, LogEntry, LogSource, ProviderFactory
│   ├── credentials.go       # Credential handling (delegates to auth)
│   └── capabilities.go      # CapabilitySet definition
├── registry/
│   └── registry.go          # Provider registry (self-registration pattern)
├── providers/
│   ├── providers.go         # Blank imports to enable built-in providers
│   ├── aws/
│   │   ├── cloudwatch.go    # aws/cloudwatch provider (self-registers)
│   │   └── cloudtrail.go    # aws/cloudtrail provider (self-registers)
│   └── github/
│       └── actions.go       # github/actions provider (self-registers)

cmd/logs/
├── logs.go                  # CommandProvider implementation
├── view.go                  # atmos logs view
├── tail.go                  # atmos logs tail --follow
├── search.go                # atmos logs search --query
└── interactive.go           # TUI mode (--interactive)
```

### 7.2 Key Types

```go
// pkg/logs/types/interfaces.go

type LogEntry struct {
    Timestamp     time.Time
    Message       string
    Level         string              // INFO, ERROR, WARN, DEBUG
    Source        LogSource
    RequestID     string
    Metadata      map[string]any      // Provider-specific fields
}

type LogSource struct {
    Kind           string             // aws/cloudwatch, aws/cloudtrail, github/actions
    Name           string             // Key from the logs map
    Identity       string             // atmos auth identity to use
    Spec           map[string]any     // Provider-specific config
}

// LogsConfig - top-level logs block on component (mirrors auth structure)
type LogsConfig struct {
    Settings LogsSettings               `yaml:"settings,omitempty" json:"settings,omitempty" mapstructure:"settings"`
    Sources  map[string]LogSourceConfig `yaml:"sources,omitempty" json:"sources,omitempty" mapstructure:"sources"`
}

// LogsSettings - baseline settings (like auth.logs)
type LogsSettings struct {
    DefaultSince    string `yaml:"default_since,omitempty" json:"default_since,omitempty" mapstructure:"default_since"`
    Format          string `yaml:"format,omitempty" json:"format,omitempty" mapstructure:"format"` // pretty, json, raw, log
    TimestampFormat string `yaml:"timestamp_format,omitempty" json:"timestamp_format,omitempty" mapstructure:"timestamp_format"`
    Colorize        bool   `yaml:"colorize,omitempty" json:"colorize,omitempty" mapstructure:"colorize"`
}

// Format options:
// - "pretty" (default): Colored, formatted for terminals
// - "json": Machine-readable JSON objects
// - "raw": Just the message, no metadata
// - "log": Route through atmos logger (respects logs.level, logs.file)

// LogSourceConfig - individual log source (like auth Identity)
type LogSourceConfig struct {
    Kind     string         `yaml:"kind" json:"kind" mapstructure:"kind"`           // aws/cloudwatch, aws/cloudtrail, github/actions
    Default  bool           `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"` // Default log source (like auth identity default)
    Identity string         `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"` // Optional: atmos auth identity
    Spec     map[string]any `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec"`     // Provider-specific config
}

type GetLogsOptions struct {
    TimeRange TimeRange
    Filter    string  // Provider-native filter
    Limit     int
    Follow    bool
}
```

### 7.3 Manager (like `auth/manager.go`)

```go
// pkg/logs/manager.go

type Manager struct {
    config      *schema.LogsConfig
    authManager auth.AuthManager    // For credential resolution
    providers   map[string]types.Provider
}

func NewManager(config *schema.LogsConfig, authMgr auth.AuthManager) (*Manager, error)

// GetLogs fetches logs for a component's log source
func (m *Manager) GetLogs(ctx context.Context, component, stack, sourceName string, opts GetLogsOptions) (*LogResult, error) {
    // 1. Resolve log source config from component
    // 2. Get identity (from source config or default_identity)
    // 3. Get credentials from authManager
    // 4. Create/get provider instance
    // 5. Fetch logs
}

// TailLogs streams logs in real-time
func (m *Manager) TailLogs(ctx context.Context, component, stack, sourceName string, opts TailOptions) (<-chan LogEntry, <-chan error)

// ListSources returns available log sources for a component
func (m *Manager) ListSources(ctx context.Context, component, stack string) ([]LogSource, error)
```

## 8. Component Resolution Strategy

Log sources are resolved from the **top-level `logs.sources` block** in component configuration:

```go
func (m *Manager) ResolveSources(ctx, component, stack string) ([]LogSource, error) {
    // 1. Get component config from stack (using describe component)
    componentConfig := m.getComponentConfig(component, stack)

    // 2. Get logs.sources block (map structure like auth.identities)
    logsConfig := componentConfig.Logs  // LogsConfig { Settings, Sources }

    if len(logsConfig.Sources) == 0 {
        return nil, ErrNoLogSourcesConfigured
    }

    // 3. Process each log source, applying template interpolation
    sources := make([]LogSource, 0, len(logsConfig.Sources))
    for name, cfg := range logsConfig.Sources {  // name is the map key
        // Interpolate templates in spec (e.g., {{ .vars.function_name }})
        spec := m.interpolateSpec(cfg.Spec, componentConfig)

        sources = append(sources, LogSource{
            Kind:     cfg.Kind,
            Name:     name,              // Key from the map
            Identity: cfg.Identity,      // May be empty (uses default auth identity)
            Spec:     spec,
        })
    }

    return sources, nil
}
```

## 9. Multi-Component Support

```bash
# Explicit list
atmos logs tail --stack dev --component vpc,api,lambda

# Wildcard pattern
atmos logs tail --stack dev --component 'api-*'

# All components in stack
atmos logs tail --stack dev --all-components
```

Output interleaves logs with source prefix:
```
[api-gateway] 2025-01-05T10:00:01Z INFO Request received
[lambda-auth] 2025-01-05T10:00:01Z INFO Processing auth
[api-gateway] 2025-01-05T10:00:02Z INFO Response sent
```

## 10. Interactive TUI Mode

Inspired by [cwlogs](https://github.com/uvxdotdev/cwlogs):

- **Tab 1: Sources** - Fuzzy search log sources, pin favorites (max 5)
- **Tab 2: Streams** - Navigate streams within selected source
- **Tab 3: Logs** - Real-time log viewer with filtering
- **Keybindings**: Arrow navigation, Enter to select, Ctrl+R refresh, Q quit
- **Persistence**: `~/.atmos/logs/pinned_sources.json`

## 11. Implementation Phases

| Phase | Scope | Deliverables |
|-------|-------|--------------|
| 1 | Core Infrastructure | Types, Provider interface, Factory, Manager skeleton, Command skeleton |
| 2 | `aws/cloudwatch` | GetLogs, Filter patterns, Live Tail, Insights queries |
| 3 | `aws/cloudtrail` | Event lookup, management/data events, time filtering |
| 4 | Component Resolution | Top-level `logs` block parsing, template interpolation |
| 5 | Streaming & Output | `--follow`, `--format` (pretty/json/raw/log), multi-component |
| 6 | `github/actions` | Workflow logs, job/step filtering, live streaming |
| 7 | Interactive TUI | Bubbletea-based TUI, source pinning, fuzzy search |
| Future | Azure/GCP | `azure/monitor`, `gcp/logging` (architecture ready) |

### 11.1 Detailed Roadmap

**Phase 1: Core Infrastructure**
- [ ] Define `LogsConfig`, `LogsSettings`, `LogSourceConfig` types in `pkg/schema/`
- [ ] Create `pkg/logs/types/interfaces.go` with Provider, LogEntry, LogSource, ProviderFactory
- [ ] Create `pkg/logs/registry/registry.go` with self-registration pattern
- [ ] Create `pkg/logs/manager.go` with auth integration
- [ ] Create `cmd/logs/` command structure (view, tail, search subcommands)

**Phase 2: aws/cloudwatch Provider**
- [ ] Implement `GetLogs` with FilterLogEvents API
- [ ] Support CloudWatch filter patterns
- [ ] Implement `TailLogs` with StartLiveTail API
- [ ] Support CloudWatch Insights queries

**Phase 3: aws/cloudtrail Provider**
- [ ] Implement `GetLogs` with LookupEvents API
- [ ] Support event source and event name filtering
- [ ] Handle management vs data events

**Phase 4: Component Resolution**
- [ ] Parse `logs.sources` from component configuration
- [ ] Implement Go template interpolation for spec values
- [ ] Merge component settings with global settings
- [ ] Resolve default log source (`default: true`)

**Phase 5: Streaming & Output**
- [ ] Implement `--follow` flag for real-time streaming
- [ ] Implement `--format pretty` (colored terminal output)
- [ ] Implement `--format json` (machine-readable)
- [ ] Implement `--format raw` (message only)
- [ ] Implement `--format log` (route through atmos logger)
- [ ] Support multi-component with interleaved output

**Phase 6: github/actions Provider**
- [ ] Implement workflow run log fetching
- [ ] Support job/step filtering
- [ ] Live streaming of in-progress workflows

**Phase 7: Interactive TUI**
- [ ] Bubbletea-based interface
- [ ] Fuzzy search for log sources
- [ ] Source pinning with persistence
- [ ] Real-time log viewer with filtering

## 12. Success Metrics

- Time from `atmos apply` to viewing logs: < 5 seconds
- Auth integration: No separate credential management
- Native query performance (server-side filtering, not client grep)

## 13. Research References

**Existing Tools Analyzed:**
- [saw](https://github.com/TylerBrock/saw) - AWS CloudWatch, colorized output, filter patterns
- [cw](https://github.com/lucagrulla/cw) - Multi-group tailing, grep/grepv, JMESPath queries
- [cwlogs](https://github.com/uvxdotdev/cwlogs) - TUI with fuzzy search, pinning, caching
- [go-awslogs](https://github.com/dzhg/go-awslogs) - Simple CLI, flexible time parsing
- [Vercel logs](https://vercel.com/docs/cli/logs) - JSON output, deployment-aware

**Cloud Provider APIs:**
- AWS CloudWatch: GetLogEvents, FilterLogEvents, StartLiveTail, Insights
- AWS CloudTrail: LookupEvents, event selectors
- GitHub Actions: Workflow runs API, job logs download

---

## Files to Create/Modify

### New Files
- `docs/prd/atmos-logs.md` - This PRD document

### Future Implementation Files (not in this PR)
- `pkg/logs/` - Core package
- `pkg/logs/types/` - Interfaces and types (Provider, ProviderFactory)
- `pkg/logs/registry/` - Provider registry (self-registration pattern)
- `pkg/logs/providers/aws/` - AWS CloudWatch, CloudTrail providers (self-register)
- `pkg/logs/providers/github/` - GitHub Actions provider (self-registers)
- `pkg/logs/providers/providers.go` - Blank imports to enable built-in providers
- `cmd/logs/` - Command implementation
- `pkg/schema/schema.go` - Add LogsConfig and LogSourceConfig types
- `website/docs/cli/commands/logs/` - Documentation

### Reference Files (patterns to follow)
- `cmd/internal/registry.go` - Registry pattern with self-registration
- `pkg/auth/manager.go` - Manager pattern
- `pkg/auth/types/interfaces.go` - Interface definitions
- `pkg/schema/schema_auth.go` - Schema patterns for auth config
