# Registry Pattern vs Factory Pattern

## Overview

This document explains the differences between the registry pattern and factory pattern, when to use each, and their respective trade-offs. It provides architectural guidance for Atmos developers to make consistent decisions when implementing extensibility in the codebase.

## Quick Reference

| Aspect | Factory Pattern | Registry Pattern |
|--------|----------------|------------------|
| **Registration** | None - creates on demand | Self-registering via `init()` |
| **Extensibility** | Requires code changes | Open for extension (plugins) |
| **Type Discovery** | Not possible | Full introspection |
| **Coupling** | Tightly coupled | Loosely coupled |
| **Complexity** | Simple | Moderate |
| **Thread Safety** | N/A (stateless) | Requires synchronization |
| **Testing** | Mock factory function | Mock registry or providers |
| **Plugin Support** | Not possible | First-class support |

**Rule of Thumb:**
- **Use Factory** when you have a fixed set of types and need simple object creation
- **Use Registry** when you want extensibility, plugins, or type discovery

## Factory Pattern

### What It Is

The factory pattern uses a **function** (or method) to create objects based on input parameters. It centralizes creation logic but requires modifying the factory function for each new type.

### Characteristics

1. **Centralized creation logic** - One function creates all types
2. **Switch statement** - Uses type parameter to determine what to create
3. **Stateless** - No persistent registry, just pure functions
4. **Closed for extension** - Adding new types requires modifying factory code
5. **Simple implementation** - Straightforward switch/case logic

### Example: Auth Factory (Current)

```go
// pkg/auth/factory/factory.go
package factory

// NewProvider creates a new provider instance based on configuration.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    if config == nil {
        return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrInvalidAuthConfig)
    }

    switch config.Kind {
    case "aws/iam-identity-center":
        return awsProviders.NewSSOProvider(name, config)
    case "aws/saml":
        return awsProviders.NewSAMLProvider(name, config)
    case "github/oidc":
        return githubProviders.NewOIDCProvider(name, config)
    case "mock":
        return mockProviders.NewProvider(name, config), nil
    default:
        return nil, fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrInvalidProviderKind, config.Kind)
    }
}

// NewIdentity creates a new identity instance based on configuration.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
    if config == nil {
        return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidAuthConfig)
    }

    switch config.Kind {
    case "aws/permission-set":
        return awsIdentities.NewPermissionSetIdentity(name, config)
    case "aws/assume-role":
        return awsIdentities.NewAssumeRoleIdentity(name, config)
    case "aws/user":
        return awsIdentities.NewUserIdentity(name, config)
    case "mock":
        return mockProviders.NewIdentity(name, config), nil
    default:
        return nil, fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
    }
}
```

### When Factory Pattern Is Appropriate

✅ **Use factory pattern when:**

1. **Fixed set of types** - You have a well-defined, stable set of types
2. **Simple creation** - Object creation is straightforward
3. **No plugins needed** - You don't need external extensibility
4. **No type discovery** - You don't need to list or introspect available types
5. **Centralized control** - You want all creation logic in one place
6. **Testing is simple** - Mock the factory function itself

**Example use cases:**
- Creating error objects (fixed set of error types)
- Creating logger instances (fixed set of log levels/formats)
- Creating configuration parsers (limited set of formats)

### Advantages

✅ **Simplicity** - Easy to understand and implement
✅ **Centralized logic** - All creation in one place
✅ **Type safety** - Compiler ensures switch coverage
✅ **No state** - Stateless, thread-safe by default
✅ **Easy testing** - Mock the factory function

### Disadvantages

❌ **Closed for extension** - Must modify factory for new types (violates Open/Closed Principle)
❌ **Tight coupling** - Factory must import all implementations
❌ **No plugins** - Cannot add external types without modifying code
❌ **No discovery** - Cannot list available types programmatically
❌ **Scaling issues** - Large switch statements as types grow

### Store Registry vs Factory

Atmos has a **hybrid** in `pkg/store/registry.go` - it's called a "registry" but actually uses factory pattern:

```go
// pkg/store/registry.go
func NewStoreRegistry(config *StoresConfig) (StoreRegistry, error) {
    registry := make(StoreRegistry)

    for key, storeConfig := range *config {
        switch storeConfig.Type {
        case "artifactory":
            store, err := NewArtifactoryStore(opts)
            registry[key] = store
        case "azure-key-vault":
            store, err := NewAzureKeyVaultStore(opts)
            registry[key] = store
        case "aws-ssm-parameter-store":
            store, err := NewSSMStore(opts)
            registry[key] = store
        // ... more cases
        default:
            return nil, fmt.Errorf("%w: %s", ErrStoreTypeNotFound, storeConfig.Type)
        }
    }

    return registry, nil
}
```

**This is factory pattern because:**
- Uses switch statement on type
- Creates instances at runtime based on config
- No self-registration via `init()`
- No global singleton registry
- Adding new store types requires modifying switch statement

**Why it works for stores:**
- Fixed set of store types (AWS SSM, Azure Key Vault, Google Secret Manager, etc.)
- Store implementations are internal (no plugins)
- No need for type discovery (users configure stores explicitly)
- Store creation is config-driven, not code-driven

## Registry Pattern

### What It Is

The registry pattern maintains a **persistent, thread-safe registry** of providers that self-register during package initialization. It enables plugin-like extensibility and type discovery.

### Characteristics

1. **Self-registration** - Providers register themselves via `init()`
2. **Singleton registry** - Global registry instance with thread-safe access
3. **Open for extension** - New types register without modifying core code
4. **Type discovery** - Can list, introspect, and query available types
5. **Plugin support** - External types can register at runtime

### Example: Command Registry (Existing)

```go
// cmd/internal/registry.go
package internal

import (
    "sync"
    "github.com/spf13/cobra"
)

// Global singleton registry.
var registry = &CommandRegistry{
    providers: make(map[string]CommandProvider),
}

type CommandRegistry struct {
    mu        sync.RWMutex
    providers map[string]CommandProvider
}

// Register adds a provider to the global registry.
// Called during init() in each command package.
func Register(provider CommandProvider) {
    registry.mu.Lock()
    defer registry.mu.Unlock()

    name := provider.GetName()
    registry.providers[name] = provider
}

// GetProvider retrieves a provider by name.
func GetProvider(name string) (CommandProvider, bool) {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    provider, ok := registry.providers[name]
    return provider, ok
}

// ListProviders returns all registered providers.
func ListProviders() map[string][]CommandProvider {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    grouped := make(map[string][]CommandProvider)
    for _, provider := range registry.providers {
        group := provider.GetGroup()
        grouped[group] = append(grouped[group], provider)
    }
    return grouped
}

// RegisterAll adds all providers to root command.
func RegisterAll(root *cobra.Command) error {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    for _, provider := range registry.providers {
        root.AddCommand(provider.GetCommand())
    }
    return nil
}
```

**Usage in command packages:**

```go
// cmd/about/about.go
package about

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

var aboutCmd = &cobra.Command{
    Use:   "about",
    Short: "Show information about Atmos",
}

// Self-registration via init().
func init() {
    internal.Register(&AboutCommandProvider{})
}

// Provider implementation.
type AboutCommandProvider struct{}

func (a *AboutCommandProvider) GetCommand() *cobra.Command {
    return aboutCmd
}

func (a *AboutCommandProvider) GetName() string {
    return "about"
}

func (a *AboutCommandProvider) GetGroup() string {
    return "Other Commands"
}
```

### When Registry Pattern Is Appropriate

✅ **Use registry pattern when:**

1. **Extensibility required** - Need to support plugins or external types
2. **Type discovery needed** - Must list or introspect available types
3. **Open/Closed Principle** - Want to extend without modifying core code
4. **Plugin architecture** - External code can register implementations
5. **Loose coupling** - Core doesn't need to import all implementations
6. **Self-registration** - Types register themselves during initialization

**Example use cases:**
- CLI commands (plugins can add commands)
- Component types (Terraform, Helmfile, Packer, future plugins)
- Authentication providers/identities (extensible auth methods)
- Hook implementations (custom hook types)

### Advantages

✅ **Extensibility** - New types register without modifying core
✅ **Plugin support** - External code can register implementations
✅ **Type discovery** - List available types, group by category, introspect
✅ **Loose coupling** - Core doesn't import all implementations
✅ **Open/Closed Principle** - Open for extension, closed for modification
✅ **Testability** - Easy to reset registry and test with mocks

### Disadvantages

❌ **Complexity** - More code (registry + interface + providers)
❌ **Thread safety** - Requires `sync.RWMutex` for concurrent access
❌ **Init() ordering** - Depends on package import order for registration
❌ **Debugging** - Harder to trace self-registration flow
❌ **More boilerplate** - Each provider needs registration code

## Detailed Comparison

### 1. Registration Mechanism

**Factory Pattern:**
```go
// No registration - factory creates on demand.
provider, err := factory.NewProvider("my-provider", config)
```

**Registry Pattern:**
```go
// Self-registration via init().
func init() {
    registry.Register(&MyProvider{})
}

// Later, retrieve from registry.
provider, ok := registry.GetProvider("my-provider")
```

### 2. Adding New Types

**Factory Pattern:**
```go
// Must modify factory code.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    switch config.Kind {
    case "aws/iam-identity-center":
        return awsProviders.NewSSOProvider(name, config)
    // ... existing cases ...
    case "new-provider-type":  // ADD THIS LINE
        return newProviderPkg.NewProvider(name, config)  // ADD THIS LINE
    default:
        return nil, fmt.Errorf("unsupported provider kind: %s", config.Kind)
    }
}
```

**Registry Pattern:**
```go
// Create new package, no core changes needed.
// pkg/auth/providers/newtype/newtype.go
package newtype

import "github.com/cloudposse/atmos/pkg/auth/registry"

func init() {
    // Self-registers automatically when package is imported.
    registry.RegisterProvider(&NewTypeProvider{})
}

type NewTypeProvider struct{}

func (p *NewTypeProvider) Kind() string {
    return "new-provider-type"
}

// Implement rest of Provider interface...
```

### 3. Type Discovery

**Factory Pattern:**
```go
// NOT POSSIBLE - factory doesn't maintain a list.
// You must hardcode the list of types elsewhere:
supportedTypes := []string{"aws/iam-identity-center", "aws/saml", "github/oidc"}
```

**Registry Pattern:**
```go
// Built-in type discovery.
providerTypes := registry.ListProviderTypes()
// Returns: ["aws/iam-identity-center", "aws/saml", "github/oidc", "mock"]

// Grouped by category.
grouped := registry.ListProviders()
// Returns: map[string][]Provider{
//   "AWS": [ssoProvider, samlProvider],
//   "GitHub": [oidcProvider],
//   "Testing": [mockProvider],
// }
```

### 4. Plugin Support

**Factory Pattern:**
```go
// NOT POSSIBLE - factory must import all implementations.
// External plugins cannot be added without modifying factory code.
```

**Registry Pattern:**
```go
// Future plugin support (no core changes needed):
// ~/.atmos/plugins/auth/custom-provider/main.go
package main

import "github.com/cloudposse/atmos/pkg/auth/registry"

func init() {
    // External plugin registers itself.
    registry.RegisterProvider(&CustomProvider{})
}
```

### 5. Testing

**Factory Pattern:**
```go
// Mock the factory function.
func TestWithMockFactory(t *testing.T) {
    // Inject mock factory.
    oldFactory := factory.NewProvider
    factory.NewProvider = func(name string, config *schema.Provider) (types.Provider, error) {
        return &mockProvider{}, nil
    }
    defer func() { factory.NewProvider = oldFactory }()

    // Test code...
}
```

**Registry Pattern:**
```go
// Reset registry and register mocks.
func TestWithMockRegistry(t *testing.T) {
    registry.Reset() // Clear registry for clean test
    registry.RegisterProvider(&mockProvider{})

    // Test code...
    provider, ok := registry.GetProvider("mock")
    assert.True(t, ok)
}
```

### 6. Thread Safety

**Factory Pattern:**
```go
// Stateless - thread-safe by default.
// Multiple goroutines can call factory simultaneously.
provider1, _ := factory.NewProvider("p1", config1)  // Goroutine 1
provider2, _ := factory.NewProvider("p2", config2)  // Goroutine 2
```

**Registry Pattern:**
```go
// Requires explicit synchronization.
type Registry struct {
    mu        sync.RWMutex  // REQUIRED for thread safety
    providers map[string]Provider
}

func (r *Registry) GetProvider(name string) (Provider, bool) {
    r.mu.RLock()  // Lock for concurrent reads
    defer r.mu.RUnlock()

    provider, ok := r.providers[name]
    return provider, ok
}
```

### 7. Coupling & Dependencies

**Factory Pattern:**
```go
// pkg/auth/factory/factory.go
package factory

import (
    // Factory MUST import ALL implementations.
    awsProviders "github.com/cloudposse/atmos/pkg/auth/providers/aws"
    githubProviders "github.com/cloudposse/atmos/pkg/auth/providers/github"
    mockProviders "github.com/cloudposse/atmos/pkg/auth/providers/mock"
    // Adding new provider? Must import here.
)

// Tight coupling: factory knows about all implementations.
```

**Registry Pattern:**
```go
// pkg/auth/registry/registry.go
package registry

// NO IMPORTS of implementations needed.
// Registry is decoupled from provider implementations.

// Implementations register themselves via init().
// Core code imports registry package, not providers.
```

```go
// cmd/root.go
import (
    "github.com/cloudposse/atmos/pkg/auth/registry"

    // Blank imports trigger init() registration.
    _ "github.com/cloudposse/atmos/pkg/auth/providers/aws"
    _ "github.com/cloudposse/atmos/pkg/auth/providers/github"
)
```

## Decision Matrix

### When to Use Factory Pattern

Use factory pattern for **auth providers and identities** if:

- ✅ Set of auth methods is relatively fixed (SSO, SAML, OIDC, assume-role, etc.)
- ✅ You don't need users to add custom auth providers via plugins
- ✅ Type discovery isn't required (auth config explicitly lists providers)
- ✅ Simplicity is more important than extensibility
- ✅ You want centralized creation logic for debugging

### When to Use Registry Pattern

Use registry pattern for **auth providers and identities** if:

- ✅ You want plugin support for custom auth methods in the future
- ✅ You need type discovery (e.g., `atmos auth list-providers`)
- ✅ You want to follow the same pattern as commands and components
- ✅ You want loose coupling between core and implementations
- ✅ You want to support enterprise custom auth methods

## Recommendation for Auth System

Based on Atmos architecture and future direction:

### Recommendation: **Registry Pattern**

**Why registry pattern is better for auth:**

1. **Consistency with existing patterns** - Commands and components use registry
2. **Future extensibility** - Enterprise users may need custom auth providers
3. **Type discovery** - `atmos auth list` can show available providers/identities
4. **Loose coupling** - Core auth manager doesn't need to import all providers
5. **Plugin readiness** - Foundation for future custom auth methods
6. **Architectural alignment** - Matches other extensibility points in Atmos

**Trade-offs accepted:**
- Slightly more complex implementation (registry + interface + providers)
- Thread safety considerations (use `sync.RWMutex`)
- Init() ordering dependency (manageable with proper imports)

**Current factory pattern issues:**
- Adding new auth provider requires modifying factory
- No way to list available providers programmatically
- Tight coupling between factory and all implementations
- Cannot support external auth plugins without code changes

### Migration Path

**Phase 1: Create registry infrastructure**
```go
// pkg/auth/registry/registry.go
package registry

type ProviderRegistry struct {
    mu        sync.RWMutex
    providers map[string]ProviderFactory
}

type ProviderFactory func(name string, config *schema.Provider) (types.Provider, error)

func RegisterProvider(kind string, factory ProviderFactory) {
    // Register provider factory
}

func GetProvider(kind string) (ProviderFactory, bool) {
    // Retrieve provider factory
}
```

**Phase 2: Migrate providers to self-registration**
```go
// pkg/auth/providers/aws/sso.go
package aws

func init() {
    registry.RegisterProvider("aws/iam-identity-center", func(name string, config *schema.Provider) (types.Provider, error) {
        return NewSSOProvider(name, config)
    })
}
```

**Phase 3: Replace factory with registry calls**
```go
// pkg/auth/manager.go (updated)
func (m *manager) authenticateProvider(providerName string) (types.ICredentials, error) {
    providerConfig := m.config.Providers[providerName]

    // OLD: factory.NewProvider(providerName, providerConfig)
    // NEW: Use registry
    factory, ok := registry.GetProvider(providerConfig.Kind)
    if !ok {
        return nil, fmt.Errorf("unknown provider kind: %s", providerConfig.Kind)
    }

    provider, err := factory(providerName, providerConfig)
    // ... rest of logic
}
```

**Phase 4: Add type discovery commands**
```go
// atmos auth list-providers
func listProviders() {
    kinds := registry.ListProviderKinds()
    for _, kind := range kinds {
        fmt.Printf("- %s\n", kind)
    }
}
```

## Hybrid Approach: Best of Both Worlds

For **auth providers and identities**, consider a **hybrid approach**:

1. **Internal creation via registry** - Loose coupling, extensibility
2. **User-facing simplicity** - Config-driven (like current factory)
3. **Gradual migration** - Wrap factory calls with registry lookups

```go
// pkg/auth/factory/factory.go (transitional)
package factory

import "github.com/cloudposse/atmos/pkg/auth/registry"

// NewProvider now delegates to registry.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    // Try registry first (new way).
    factory, ok := registry.GetProvider(config.Kind)
    if ok {
        return factory(name, config)
    }

    // Fall back to switch statement (legacy, eventually removed).
    switch config.Kind {
    case "aws/iam-identity-center":
        return awsProviders.NewSSOProvider(name, config)
    // ... more cases ...
    default:
        return nil, fmt.Errorf("unsupported provider kind: %s", config.Kind)
    }
}
```

This allows:
- ✅ Gradual migration without breaking changes
- ✅ Registry benefits once providers self-register
- ✅ Backward compatibility during transition
- ✅ Remove switch statement once all providers migrated

## Examples from Other Projects

### Registry Pattern Examples

**Docker CLI:**
```go
// github.com/docker/cli/cli/command/registry.go
var commandRegistry = make(map[string]*cobra.Command)

func Register(name string, cmd *cobra.Command) {
    commandRegistry[name] = cmd
}
```

**Kubernetes API Scheme:**
```go
// k8s.io/apimachinery/pkg/runtime/scheme.go
type Scheme struct {
    gvkToType map[schema.GroupVersionKind]reflect.Type
}

func (s *Scheme) AddKnownTypes(gv schema.GroupVersion, types ...Object) {
    // Registry of resource types
}
```

**Terraform Providers:**
```go
// github.com/hashicorp/terraform/plugin/provider_registry.go
type ProviderRegistry struct {
    providers map[string]Provider
}

func (r *ProviderRegistry) RegisterProvider(name string, p Provider) {
    r.providers[name] = p
}
```

### Factory Pattern Examples

**Go standard library errors:**
```go
// errors/errors.go
func New(text string) error {
    return &errorString{text}
}

// Simple factory - no registry needed
```

**Database drivers (sql.Register is actually a registry!):**
```go
// database/sql/driver.go
func Register(name string, driver driver.Driver) {
    // This is REGISTRY pattern, not factory
    driversMu.Lock()
    defer driversMu.Unlock()
    drivers[name] = driver
}
```

## Summary

### Key Takeaways

1. **Factory Pattern = Simple creation** - Fixed types, no plugins, centralized logic
2. **Registry Pattern = Extensible architecture** - Self-registration, plugins, type discovery
3. **Store Registry is actually factory** - Uses switch statement, config-driven creation
4. **Auth should use registry** - Matches commands/components, enables future plugins
5. **Hybrid approach recommended** - Gradual migration, backward compatibility

### Decision Flowchart

```
Do you need external plugins?
├─ Yes → Use Registry Pattern
└─ No → Do you need type discovery?
    ├─ Yes → Use Registry Pattern
    └─ No → Is the set of types fixed?
        ├─ Yes → Use Factory Pattern
        └─ No → Use Registry Pattern (future extensibility)
```

### Atmos Consistency Guidelines

For architectural consistency in Atmos:

1. **Commands** → Registry (existing)
2. **Components** → Registry (existing)
3. **Auth Providers/Identities** → **Should be Registry** (currently Factory)
4. **Stores** → Factory is acceptable (fixed set, config-driven)
5. **Hooks** → Could benefit from Registry (future extensibility)
6. **Workflows** → Factory is acceptable (defined in config)

## References

- [Command Registry Pattern PRD](command-registry-pattern.md)
- [Component Registry Pattern PRD](component-registry-pattern.md)
- [Gang of Four: Factory Pattern](https://refactoring.guru/design-patterns/factory-method)
- [Martin Fowler: Plugin Architecture](https://www.martinfowler.com/articles/injection.html)
- [Go Plugin Documentation](https://pkg.go.dev/plugin)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-10-29 | Initial PRD comparing registry vs factory patterns |
