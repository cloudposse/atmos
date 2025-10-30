# Auth Registry Pattern Refactoring

## Overview

This document describes the refactoring of Atmos authentication providers and identities from a factory pattern to a registry pattern, aligning with the architectural patterns used for commands (`cmd/internal/registry.go`) and components (`pkg/component/registry.go`).

## Problem Statement

### Current State

The auth system currently uses a **factory pattern** in `pkg/auth/factory/factory.go`:

```go
// pkg/auth/factory/factory.go
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

### Challenges

1. **Limited extensibility** - Adding new auth providers requires modifying factory code
2. **No plugin support** - Cannot add custom enterprise auth providers without forking
3. **Tight coupling** - Factory must import all provider/identity implementations
4. **No type discovery** - Cannot list available providers/identities programmatically
5. **Inconsistent architecture** - Commands and components use registry, auth uses factory
6. **Violates Open/Closed Principle** - Closed for extension, must modify for new types
7. **Validation coupling** - Validator imports factory, creating tight coupling for kind validation

### Why This Matters

1. **Enterprise use cases** - Large organizations may need custom auth providers (e.g., internal IdP, custom credential management)
2. **Architectural consistency** - Registry pattern is established best practice in Atmos
3. **Future extensibility** - Foundation for auth plugins in Atmos Pro or community extensions
4. **Type discovery** - Commands like `atmos auth list-providers` require registry

## Solution: Auth Registry Pattern

### Architecture Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                    Atmos Auth System                         │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────┐             │
│  │       Provider Registry                    │             │
│  │  (pkg/auth/registry/provider_registry.go)  │             │
│  └────────────────────────────────────────────┘             │
│           │                        │                         │
│           ▼                        ▼                         │
│  ┌─────────────────┐    ┌──────────────────────┐           │
│  │  Built-in       │    │  Plugin Providers    │           │
│  │  Providers      │    │  (future)            │           │
│  │                 │    │                      │           │
│  │  - AWS SSO      │    │  - Enterprise IdP    │           │
│  │  - AWS SAML     │    │  - Custom vaults     │           │
│  │  - GitHub OIDC  │    │  - Custom auth       │           │
│  │  - Mock         │    │                      │           │
│  └─────────────────┘    └──────────────────────┘           │
│                                                              │
│  ┌────────────────────────────────────────────┐             │
│  │       Identity Registry                    │             │
│  │  (pkg/auth/registry/identity_registry.go)  │             │
│  └────────────────────────────────────────────┘             │
│           │                        │                         │
│           ▼                        ▼                         │
│  ┌─────────────────┐    ┌──────────────────────┐           │
│  │  Built-in       │    │  Plugin Identities   │           │
│  │  Identities     │    │  (future)            │           │
│  │                 │    │                      │           │
│  │  - Permission   │    │  - Custom identity   │           │
│  │    Set          │    │    resolution        │           │
│  │  - Assume Role  │    │                      │           │
│  │  - User         │    │                      │           │
│  │  - Mock         │    │                      │           │
│  └─────────────────┘    └──────────────────────┘           │
│                                                              │
│  Registration Flow:                                          │
│  1. Load built-in providers/identities via registry         │
│  2. Providers/identities self-register via init()           │
│  3. Auth manager uses registry to create instances          │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Parallel with command/component registry** - Same patterns and conventions
2. **Self-registration** - Providers/identities register themselves via `init()`
3. **Type safety** - Interface-based with factory functions
4. **Backward compatibility** - No changes to auth configuration or behavior
5. **Testability** - Mock providers/identities for unit tests
6. **Plugin readiness** - Foundation for future external auth plugins
7. **Loose coupling** - Core auth manager doesn't import implementations

## Implementation

### 1. Provider Registry

```go
// pkg/auth/registry/provider_registry.go
package registry

import (
    "fmt"
    "sort"
    "sync"

    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/perf"
    "github.com/cloudposse/atmos/pkg/schema"
)

// Global provider registry instance.
var providerRegistry = &ProviderRegistry{
    factories: make(map[string]ProviderFactory),
}

// ProviderFactory creates a provider instance from configuration.
type ProviderFactory func(name string, config *schema.Provider) (types.Provider, error)

// ProviderRegistry manages authentication provider registration.
// It is thread-safe and supports concurrent registration and access.
type ProviderRegistry struct {
    mu        sync.RWMutex
    factories map[string]ProviderFactory
}

// RegisterProvider adds a provider factory to the registry.
// This is called during package init() for built-in providers.
//
// Example usage:
//
//	func init() {
//	    registry.RegisterProvider("aws/iam-identity-center", NewSSOProvider)
//	}
func RegisterProvider(kind string, factory ProviderFactory) error {
    defer perf.Track(nil, "registry.RegisterProvider")()

    if kind == "" {
        return fmt.Errorf("%w: provider kind cannot be empty", errUtils.ErrInvalidProviderKind)
    }

    if factory == nil {
        return fmt.Errorf("%w: provider factory cannot be nil", errUtils.ErrInvalidProviderKind)
    }

    providerRegistry.mu.Lock()
    defer providerRegistry.mu.Unlock()

    // Allow re-registration (last wins) for testing and future plugin override.
    providerRegistry.factories[kind] = factory

    return nil
}

// GetProviderFactory returns a provider factory by kind.
// Returns the factory and true if found, nil and false otherwise.
func GetProviderFactory(kind string) (ProviderFactory, bool) {
    defer perf.Track(nil, "registry.GetProviderFactory")()

    providerRegistry.mu.RLock()
    defer providerRegistry.mu.RUnlock()

    factory, ok := providerRegistry.factories[kind]
    return factory, ok
}

// NewProvider creates a provider instance using the registered factory.
// This replaces the factory.NewProvider function.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    defer perf.Track(nil, "registry.NewProvider")()

    if config == nil {
        return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrInvalidAuthConfig)
    }

    factory, ok := GetProviderFactory(config.Kind)
    if !ok {
        return nil, fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrInvalidProviderKind, config.Kind)
    }

    return factory(name, config)
}

// ListProviderKinds returns all registered provider kinds sorted alphabetically.
// Example: ["aws/iam-identity-center", "aws/saml", "github/oidc", "mock"].
func ListProviderKinds() []string {
    defer perf.Track(nil, "registry.ListProviderKinds")()

    providerRegistry.mu.RLock()
    defer providerRegistry.mu.RUnlock()

    kinds := make([]string, 0, len(providerRegistry.factories))
    for kind := range providerRegistry.factories {
        kinds = append(kinds, kind)
    }

    sort.Strings(kinds)
    return kinds
}

// CountProviders returns the number of registered provider kinds.
func CountProviders() int {
    defer perf.Track(nil, "registry.CountProviders")()

    providerRegistry.mu.RLock()
    defer providerRegistry.mu.RUnlock()

    return len(providerRegistry.factories)
}

// ResetProviders clears the provider registry (for testing only).
// WARNING: This function is for TESTING ONLY. It should never be called
// in production code. It allows tests to start with a clean registry state.
func ResetProviders() {
    providerRegistry.mu.Lock()
    defer providerRegistry.mu.Unlock()

    providerRegistry.factories = make(map[string]ProviderFactory)
}
```

### 2. Identity Registry

```go
// pkg/auth/registry/identity_registry.go
package registry

import (
    "fmt"
    "sort"
    "sync"

    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/perf"
    "github.com/cloudposse/atmos/pkg/schema"
)

// Global identity registry instance.
var identityRegistry = &IdentityRegistry{
    factories: make(map[string]IdentityFactory),
}

// IdentityFactory creates an identity instance from configuration.
type IdentityFactory func(name string, config *schema.Identity) (types.Identity, error)

// IdentityRegistry manages authentication identity registration.
// It is thread-safe and supports concurrent registration and access.
type IdentityRegistry struct {
    mu        sync.RWMutex
    factories map[string]IdentityFactory
}

// RegisterIdentity adds an identity factory to the registry.
// This is called during package init() for built-in identities.
//
// Example usage:
//
//	func init() {
//	    registry.RegisterIdentity("aws/permission-set", NewPermissionSetIdentity)
//	}
func RegisterIdentity(kind string, factory IdentityFactory) error {
    defer perf.Track(nil, "registry.RegisterIdentity")()

    if kind == "" {
        return fmt.Errorf("%w: identity kind cannot be empty", errUtils.ErrInvalidIdentityKind)
    }

    if factory == nil {
        return fmt.Errorf("%w: identity factory cannot be nil", errUtils.ErrInvalidIdentityKind)
    }

    identityRegistry.mu.Lock()
    defer identityRegistry.mu.Unlock()

    // Allow re-registration (last wins) for testing and future plugin override.
    identityRegistry.factories[kind] = factory

    return nil
}

// GetIdentityFactory returns an identity factory by kind.
// Returns the factory and true if found, nil and false otherwise.
func GetIdentityFactory(kind string) (IdentityFactory, bool) {
    defer perf.Track(nil, "registry.GetIdentityFactory")()

    identityRegistry.mu.RLock()
    defer identityRegistry.mu.RUnlock()

    factory, ok := identityRegistry.factories[kind]
    return factory, ok
}

// NewIdentity creates an identity instance using the registered factory.
// This replaces the factory.NewIdentity function.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
    defer perf.Track(nil, "registry.NewIdentity")()

    if config == nil {
        return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidAuthConfig)
    }

    factory, ok := GetIdentityFactory(config.Kind)
    if !ok {
        return nil, fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
    }

    return factory(name, config)
}

// ListIdentityKinds returns all registered identity kinds sorted alphabetically.
// Example: ["aws/assume-role", "aws/permission-set", "aws/user", "mock"].
func ListIdentityKinds() []string {
    defer perf.Track(nil, "registry.ListIdentityKinds")()

    identityRegistry.mu.RLock()
    defer identityRegistry.mu.RUnlock()

    kinds := make([]string, 0, len(identityRegistry.factories))
    for kind := range identityRegistry.factories {
        kinds = append(kinds, kind)
    }

    sort.Strings(kinds)
    return kinds
}

// CountIdentities returns the number of registered identity kinds.
func CountIdentities() int {
    defer perf.Track(nil, "registry.CountIdentities")()

    identityRegistry.mu.RLock()
    defer identityRegistry.mu.RUnlock()

    return len(identityRegistry.factories)
}

// ResetIdentities clears the identity registry (for testing only).
// WARNING: This function is for TESTING ONLY. It should never be called
// in production code. It allows tests to start with a clean registry state.
func ResetIdentities() {
    identityRegistry.mu.Lock()
    defer identityRegistry.mu.Unlock()

    identityRegistry.factories = make(map[string]IdentityFactory)
}
```

### 3. Provider Self-Registration

```go
// pkg/auth/providers/aws/sso.go
package aws

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with provider registry.
func init() {
    if err := registry.RegisterProvider("aws/iam-identity-center", NewSSOProvider); err != nil {
        panic(fmt.Sprintf("failed to register aws/iam-identity-center provider: %v", err))
    }
}

// NewSSOProvider creates a new AWS SSO provider.
// This function signature matches registry.ProviderFactory.
func NewSSOProvider(name string, config *schema.Provider) (types.Provider, error) {
    // ... existing implementation ...
}

// ... rest of existing implementation ...
```

```go
// pkg/auth/providers/aws/saml.go
package aws

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with provider registry.
func init() {
    if err := registry.RegisterProvider("aws/saml", NewSAMLProvider); err != nil {
        panic(fmt.Sprintf("failed to register aws/saml provider: %v", err))
    }
}

// NewSAMLProvider creates a new AWS SAML provider.
func NewSAMLProvider(name string, config *schema.Provider) (types.Provider, error) {
    // ... existing implementation ...
}

// ... rest of existing implementation ...
```

```go
// pkg/auth/providers/github/oidc.go
package github

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with provider registry.
func init() {
    if err := registry.RegisterProvider("github/oidc", NewOIDCProvider); err != nil {
        panic(fmt.Sprintf("failed to register github/oidc provider: %v", err))
    }
}

// NewOIDCProvider creates a new GitHub OIDC provider.
func NewOIDCProvider(name string, config *schema.Provider) (types.Provider, error) {
    // ... existing implementation ...
}

// ... rest of existing implementation ...
```

```go
// pkg/auth/providers/mock/provider.go
package mock

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with provider registry.
func init() {
    if err := registry.RegisterProvider("mock", NewProvider); err != nil {
        panic(fmt.Sprintf("failed to register mock provider: %v", err))
    }
}

// NewProvider creates a new mock provider.
// Note: Returns concrete type, not error (mock provider never fails).
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    return &mockProvider{
        name:   name,
        config: config,
    }, nil
}

// ... rest of existing implementation ...
```

### 4. Identity Self-Registration

```go
// pkg/auth/identities/aws/permission_set.go
package aws

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with identity registry.
func init() {
    if err := registry.RegisterIdentity("aws/permission-set", NewPermissionSetIdentity); err != nil {
        panic(fmt.Sprintf("failed to register aws/permission-set identity: %v", err))
    }
}

// NewPermissionSetIdentity creates a new AWS permission set identity.
func NewPermissionSetIdentity(name string, config *schema.Identity) (types.Identity, error) {
    // ... existing implementation ...
}

// ... rest of existing implementation ...
```

```go
// pkg/auth/identities/aws/assume_role.go
package aws

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with identity registry.
func init() {
    if err := registry.RegisterIdentity("aws/assume-role", NewAssumeRoleIdentity); err != nil {
        panic(fmt.Sprintf("failed to register aws/assume-role identity: %v", err))
    }
}

// NewAssumeRoleIdentity creates a new AWS assume role identity.
func NewAssumeRoleIdentity(name string, config *schema.Identity) (types.Identity, error) {
    // ... existing implementation ...
}

// ... rest of existing implementation ...
```

```go
// pkg/auth/identities/aws/user.go
package aws

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with identity registry.
func init() {
    if err := registry.RegisterIdentity("aws/user", NewUserIdentity); err != nil {
        panic(fmt.Sprintf("failed to register aws/user identity: %v", err))
    }
}

// NewUserIdentity creates a new AWS user identity.
func NewUserIdentity(name string, config *schema.Identity) (types.Identity, error) {
    // ... existing implementation ...
}

// ... rest of existing implementation ...
```

```go
// pkg/auth/providers/mock/identity.go
package mock

import (
    "context"
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// Self-register with identity registry.
func init() {
    if err := registry.RegisterIdentity("mock", NewIdentity); err != nil {
        panic(fmt.Sprintf("failed to register mock identity: %v", err))
    }
}

// NewIdentity creates a new mock identity.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
    return &mockIdentity{
        name:   name,
        config: config,
    }, nil
}

// ... rest of existing implementation ...
```

### 5. Update Auth Manager

```go
// pkg/auth/manager.go
package auth

import (
    // ... existing imports ...

    // Import providers/identities for side-effect registration.
    _ "github.com/cloudposse/atmos/pkg/auth/identities/aws"
    _ "github.com/cloudposse/atmos/pkg/auth/providers/aws"
    _ "github.com/cloudposse/atmos/pkg/auth/providers/github"
    _ "github.com/cloudposse/atmos/pkg/auth/providers/mock"

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// authenticateProvider authenticates with a provider and returns credentials.
func (m *manager) authenticateProvider(providerName string) (types.ICredentials, error) {
    defer perf.Track(m.config, "auth.authenticateProvider")()

    providerConfig, ok := m.authConfig.Providers[providerName]
    if !ok {
        return nil, fmt.Errorf("%w: provider %s not found", errUtils.ErrProviderNotFound, providerName)
    }

    // OLD: Use factory
    // provider, err := factory.NewProvider(providerName, providerConfig)

    // NEW: Use registry
    provider, err := registry.NewProvider(providerName, providerConfig)
    if err != nil {
        return nil, fmt.Errorf("%w: failed to create provider %s: %w", errUtils.ErrProviderCreationFailed, providerName, err)
    }

    // ... rest of existing logic ...
}

// createIdentity creates an identity instance from configuration.
func (m *manager) createIdentity(identityName string, identityConfig *schema.Identity) (types.Identity, error) {
    defer perf.Track(m.config, "auth.createIdentity")()

    // OLD: Use factory
    // identity, err := factory.NewIdentity(identityName, identityConfig)

    // NEW: Use registry
    identity, err := registry.NewIdentity(identityName, identityConfig)
    if err != nil {
        return nil, fmt.Errorf("%w: failed to create identity %s: %w", errUtils.ErrIdentityCreationFailed, identityName, err)
    }

    return identity, nil
}
```

### 6. Deprecate Factory Package

```go
// pkg/auth/factory/factory.go
package factory

import (
    "github.com/cloudposse/atmos/pkg/auth/registry"
    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

// Deprecated: Use registry.NewProvider instead.
// This function will be removed in a future version.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    return registry.NewProvider(name, config)
}

// Deprecated: Use registry.NewIdentity instead.
// This function will be removed in a future version.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
    return registry.NewIdentity(name, config)
}
```

### 7. Update Validation

The validation system must be updated to use the registry for kind validation:

```go
// pkg/auth/validation/validator.go
package validation

import (
    // ... existing imports ...

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

// ValidateProvider validates a provider configuration.
func (v *validator) ValidateProvider(name string, provider *schema.Provider) error {
    defer perf.Track(nil, "validation.ValidateProvider")()

    if name == "" {
        return fmt.Errorf("%w: provider name cannot be empty", errUtils.ErrInvalidProviderConfig)
    }

    if provider.Kind == "" {
        return fmt.Errorf("%w: provider kind is required", errUtils.ErrInvalidProviderConfig)
    }

    // OLD: Use factory (tight coupling)
    // providerInstance, err := factory.NewProvider(name, provider)

    // NEW: Use registry to validate kind exists
    providerInstance, err := registry.NewProvider(name, provider)
    if err != nil {
        // Registry will return ErrInvalidProviderKind if kind is not registered
        return err
    }

    // Call provider-specific validation
    return providerInstance.Validate()
}

// ValidateIdentity validates an identity configuration.
func (v *validator) ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error {
    defer perf.Track(nil, "validation.ValidateIdentity")()

    if name == "" {
        return fmt.Errorf("%w: identity name cannot be empty", errUtils.ErrInvalidIdentityConfig)
    }

    if identity.Kind == "" {
        return fmt.Errorf("%w: identity kind is required", errUtils.ErrInvalidIdentityConfig)
    }

    // Validate via configuration - AWS User identities don't require via provider.
    if err := v.validateViaConfiguration(identity, providers); err != nil {
        return err
    }

    // OLD: Use factory (tight coupling)
    // identityInstance, err := factory.NewIdentity(name, identity)

    // NEW: Use registry to validate kind exists
    identityInstance, err := registry.NewIdentity(name, identity)
    if err != nil {
        // Registry will return ErrInvalidIdentityKind if kind is not registered
        return err
    }

    // Call identity-specific validation
    return identityInstance.Validate()
}
```

**Validation Flow with Registry:**

1. **Kind validation** - Registry checks if provider/identity kind is registered
2. **Instance creation** - Registry creates provider/identity instance
3. **Provider-specific validation** - Each provider validates its own config
4. **Identity-specific validation** - Each identity validates its own config
5. **Reference validation** - Validator checks that referenced providers/identities exist
6. **Cycle detection** - Validator checks for circular identity chains

**Error Types:**

```go
// Provider validation errors
registry.NewProvider("my-provider", &schema.Provider{Kind: "nonexistent"})
// Returns: ErrInvalidProviderKind: unsupported provider kind: nonexistent

// Identity validation errors
registry.NewIdentity("my-identity", &schema.Identity{Kind: "nonexistent"})
// Returns: ErrInvalidIdentityKind: unsupported identity kind: nonexistent

// Reference validation (still in validator)
validator.ValidateIdentity("my-identity", &schema.Identity{
    Kind: "aws/permission-set",
    Via:  &schema.Via{Provider: "nonexistent-provider"},
}, providers)
// Returns: ErrInvalidAuthConfig: referenced provider "nonexistent-provider" does not exist
```

**Benefits of Registry-Based Validation:**

1. ✅ **Early error detection** - Invalid kinds caught at validation time
2. ✅ **Better error messages** - Registry provides specific error for unknown kinds
3. ✅ **Type discovery** - Can list valid kinds: `registry.ListProviderKinds()`
4. ✅ **Loose coupling** - Validator doesn't import factory
5. ✅ **Plugin support** - External plugins automatically validated if registered

### 8. Type Discovery Commands (Future)

```go
// cmd/auth/list.go (future enhancement)
package auth

import (
    "fmt"

    "github.com/spf13/cobra"

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

var listCmd = &cobra.Command{
    Use:   "list",
    Short: "List available authentication providers and identities",
}

var listProvidersCmd = &cobra.Command{
    Use:   "providers",
    Short: "List available authentication providers",
    RunE: func(cmd *cobra.Command, args []string) error {
        kinds := registry.ListProviderKinds()

        fmt.Println("Available authentication providers:")
        for _, kind := range kinds {
            fmt.Printf("  - %s\n", kind)
        }

        return nil
    },
}

var listIdentitiesCmd = &cobra.Command{
    Use:   "identities",
    Short: "List available authentication identities",
    RunE: func(cmd *cobra.Command, args []string) error {
        kinds := registry.ListIdentityKinds()

        fmt.Println("Available authentication identities:")
        for _, kind := range kinds {
            fmt.Printf("  - %s\n", kind)
        }

        return nil
    },
}

func init() {
    listCmd.AddCommand(listProvidersCmd)
    listCmd.AddCommand(listIdentitiesCmd)
}
```

## Testing Strategy

### 1. Registry Unit Tests

```go
// pkg/auth/registry/provider_registry_test.go
package registry

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

func TestRegisterProvider(t *testing.T) {
    ResetProviders() // Clean state

    factory := func(name string, config *schema.Provider) (types.Provider, error) {
        return &mockProvider{name: name}, nil
    }

    err := RegisterProvider("test-provider", factory)
    require.NoError(t, err)

    assert.Equal(t, 1, CountProviders())
}

func TestGetProviderFactory(t *testing.T) {
    ResetProviders()

    factory := func(name string, config *schema.Provider) (types.Provider, error) {
        return &mockProvider{name: name}, nil
    }

    RegisterProvider("test-provider", factory)

    retrieved, ok := GetProviderFactory("test-provider")
    assert.True(t, ok)
    assert.NotNil(t, retrieved)
}

func TestListProviderKinds(t *testing.T) {
    ResetProviders()

    RegisterProvider("aws/iam-identity-center", mockProviderFactory)
    RegisterProvider("github/oidc", mockProviderFactory)
    RegisterProvider("mock", mockProviderFactory)

    kinds := ListProviderKinds()

    assert.Len(t, kinds, 3)
    assert.Equal(t, []string{"aws/iam-identity-center", "github/oidc", "mock"}, kinds)
}

func TestNewProvider(t *testing.T) {
    ResetProviders()

    RegisterProvider("test-provider", func(name string, config *schema.Provider) (types.Provider, error) {
        return &mockProvider{name: name, config: config}, nil
    })

    config := &schema.Provider{Kind: "test-provider"}
    provider, err := NewProvider("my-provider", config)

    require.NoError(t, err)
    assert.NotNil(t, provider)
}

func TestNewProviderUnsupportedKind(t *testing.T) {
    ResetProviders()

    config := &schema.Provider{Kind: "nonexistent"}
    provider, err := NewProvider("my-provider", config)

    assert.Error(t, err)
    assert.Nil(t, provider)
    assert.Contains(t, err.Error(), "unsupported provider kind")
}

func TestConcurrentProviderRegistration(t *testing.T) {
    ResetProviders()

    var wg sync.WaitGroup
    numGoroutines := 100

    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            kind := fmt.Sprintf("provider-%d", id)
            RegisterProvider(kind, mockProviderFactory)
        }(i)
    }

    wg.Wait()
    assert.Equal(t, numGoroutines, CountProviders())
}
```

```go
// pkg/auth/registry/identity_registry_test.go
package registry

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

func TestRegisterIdentity(t *testing.T) {
    ResetIdentities()

    factory := func(name string, config *schema.Identity) (types.Identity, error) {
        return &mockIdentity{name: name}, nil
    }

    err := RegisterIdentity("test-identity", factory)
    require.NoError(t, err)

    assert.Equal(t, 1, CountIdentities())
}

func TestGetIdentityFactory(t *testing.T) {
    ResetIdentities()

    factory := func(name string, config *schema.Identity) (types.Identity, error) {
        return &mockIdentity{name: name}, nil
    }

    RegisterIdentity("test-identity", factory)

    retrieved, ok := GetIdentityFactory("test-identity")
    assert.True(t, ok)
    assert.NotNil(t, retrieved)
}

func TestListIdentityKinds(t *testing.T) {
    ResetIdentities()

    RegisterIdentity("aws/permission-set", mockIdentityFactory)
    RegisterIdentity("aws/assume-role", mockIdentityFactory)
    RegisterIdentity("aws/user", mockIdentityFactory)

    kinds := ListIdentityKinds()

    assert.Len(t, kinds, 3)
    assert.Equal(t, []string{"aws/assume-role", "aws/permission-set", "aws/user"}, kinds)
}

func TestNewIdentity(t *testing.T) {
    ResetIdentities()

    RegisterIdentity("test-identity", func(name string, config *schema.Identity) (types.Identity, error) {
        return &mockIdentity{name: name, config: config}, nil
    })

    config := &schema.Identity{Kind: "test-identity"}
    identity, err := NewIdentity("my-identity", config)

    require.NoError(t, err)
    assert.NotNil(t, identity)
}

func TestNewIdentityUnsupportedKind(t *testing.T) {
    ResetIdentities()

    config := &schema.Identity{Kind: "nonexistent"}
    identity, err := NewIdentity("my-identity", config)

    assert.Error(t, err)
    assert.Nil(t, identity)
    assert.Contains(t, err.Error(), "unsupported identity kind")
}

func TestConcurrentIdentityRegistration(t *testing.T) {
    ResetIdentities()

    var wg sync.WaitGroup
    numGoroutines := 100

    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            kind := fmt.Sprintf("identity-%d", id)
            RegisterIdentity(kind, mockIdentityFactory)
        }(i)
    }

    wg.Wait()
    assert.Equal(t, numGoroutines, CountIdentities())
}
```

### 2. Integration Tests

```go
// pkg/auth/manager_registry_test.go
package auth

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/cloudposse/atmos/pkg/auth/registry"
)

func TestManagerUsesRegistry(t *testing.T) {
    // Verify manager creates providers via registry
    config := &schema.AuthConfig{
        Providers: map[string]*schema.Provider{
            "my-sso": {
                Kind:     "aws/iam-identity-center",
                StartURL: "https://my-sso.awsapps.com/start",
                Region:   "us-east-1",
            },
        },
        Identities: map[string]*schema.Identity{
            "my-identity": {
                Kind: "aws/permission-set",
                Via: &schema.Via{
                    Provider: "my-sso",
                },
                PermissionSetName: "PowerUserAccess",
                Account:           &schema.Account{Name: "production"},
            },
        },
    }

    manager := NewManager(atmosConfig, config)

    // Verify provider created via registry
    creds, err := manager.Authenticate(context.Background(), "my-identity")
    require.NoError(t, err)
    assert.NotNil(t, creds)
}

func TestManagerFailsWithUnregisteredProvider(t *testing.T) {
    registry.ResetProviders()

    config := &schema.AuthConfig{
        Providers: map[string]*schema.Provider{
            "my-provider": {
                Kind: "nonexistent",
            },
        },
    }

    manager := NewManager(atmosConfig, config)

    _, err := manager.Authenticate(context.Background(), "my-identity")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unsupported provider kind")
}
```

### 3. Validation Tests

```go
// pkg/auth/validation/validator_registry_test.go
package validation

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/cloudposse/atmos/pkg/auth/registry"
    "github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateProviderWithUnregisteredKind(t *testing.T) {
    registry.ResetProviders()

    validator := NewValidator()

    provider := &schema.Provider{
        Kind:     "nonexistent-provider",
        StartURL: "https://example.com",
        Region:   "us-east-1",
    }

    err := validator.ValidateProvider("my-provider", provider)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "unsupported provider kind")
    assert.Contains(t, err.Error(), "nonexistent-provider")
}

func TestValidateProviderWithRegisteredKind(t *testing.T) {
    // Providers registered via init() - no need to reset

    validator := NewValidator()

    provider := &schema.Provider{
        Kind:     "aws/iam-identity-center",
        StartURL: "https://my-sso.awsapps.com/start",
        Region:   "us-east-1",
    }

    err := validator.ValidateProvider("my-provider", provider)

    require.NoError(t, err)
}

func TestValidateIdentityWithUnregisteredKind(t *testing.T) {
    registry.ResetIdentities()

    validator := NewValidator()

    identity := &schema.Identity{
        Kind: "nonexistent-identity",
        Via: &schema.Via{
            Provider: "my-provider",
        },
    }

    providers := map[string]*schema.Provider{
        "my-provider": {
            Kind: "aws/iam-identity-center",
        },
    }

    err := validator.ValidateIdentity("my-identity", identity, providers)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "unsupported identity kind")
    assert.Contains(t, err.Error(), "nonexistent-identity")
}

func TestValidateIdentityWithRegisteredKind(t *testing.T) {
    // Identities registered via init() - no need to reset

    validator := NewValidator()

    identity := &schema.Identity{
        Kind: "aws/permission-set",
        Via: &schema.Via{
            Provider: "my-provider",
        },
        PermissionSetName: "PowerUserAccess",
        Account: &schema.Account{
            Name: "production",
        },
    }

    providers := map[string]*schema.Provider{
        "my-provider": {
            Kind:     "aws/iam-identity-center",
            StartURL: "https://my-sso.awsapps.com/start",
            Region:   "us-east-1",
        },
    }

    err := validator.ValidateIdentity("my-identity", identity, providers)

    require.NoError(t, err)
}

func TestValidateIdentityWithNonexistentProvider(t *testing.T) {
    validator := NewValidator()

    identity := &schema.Identity{
        Kind: "aws/permission-set",
        Via: &schema.Via{
            Provider: "nonexistent-provider", // Provider doesn't exist
        },
        PermissionSetName: "PowerUserAccess",
    }

    providers := map[string]*schema.Provider{
        "my-provider": {
            Kind: "aws/iam-identity-center",
        },
    }

    err := validator.ValidateIdentity("my-identity", identity, providers)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "referenced provider")
    assert.Contains(t, err.Error(), "does not exist")
}

func TestValidateAuthConfigWithInvalidProviderKind(t *testing.T) {
    registry.ResetProviders()

    validator := NewValidator()

    config := &schema.AuthConfig{
        Providers: map[string]schema.Provider{
            "my-provider": {
                Kind: "invalid-provider-kind",
            },
        },
        Identities: map[string]schema.Identity{},
    }

    err := validator.ValidateAuthConfig(config)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "provider \"my-provider\" validation failed")
    assert.Contains(t, err.Error(), "unsupported provider kind")
}

func TestValidateAuthConfigWithInvalidIdentityKind(t *testing.T) {
    registry.ResetIdentities()

    validator := NewValidator()

    config := &schema.AuthConfig{
        Providers: map[string]schema.Provider{
            "my-provider": {
                Kind:     "aws/iam-identity-center",
                StartURL: "https://my-sso.awsapps.com/start",
                Region:   "us-east-1",
            },
        },
        Identities: map[string]schema.Identity{
            "my-identity": {
                Kind: "invalid-identity-kind",
                Via: &schema.Via{
                    Provider: "my-provider",
                },
            },
        },
    }

    err := validator.ValidateAuthConfig(config)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "identity \"my-identity\" validation failed")
    assert.Contains(t, err.Error(), "unsupported identity kind")
}
```

### 4. Test Coverage Requirements

**Minimum Coverage Targets:**

| Package | Minimum Coverage | Focus Areas |
|---------|------------------|-------------|
| `pkg/auth/registry/provider_registry.go` | **90%** | All functions, concurrent access, edge cases |
| `pkg/auth/registry/identity_registry.go` | **90%** | All functions, concurrent access, edge cases |
| `pkg/auth/validation/validator.go` | **90%** | Registry integration, invalid kinds, reference validation |

## Migration Plan

### Phase 1: Create Registry Infrastructure (Week 1)

**Goal:** Create registry without changing behavior

**Tasks:**
1. Create `pkg/auth/registry/provider_registry.go`
2. Create `pkg/auth/registry/identity_registry.go`
3. Add comprehensive unit tests (90%+ coverage)
4. Add thread safety tests (concurrent registration/access)
5. Add edge case tests (nil checks, empty registry, duplicate registration)

**Success Criteria:**
- ✅ Registry compiles and passes all tests
- ✅ 90%+ test coverage on both registries
- ✅ Thread safety verified with concurrent tests
- ✅ No integration with existing code yet

### Phase 2: Provider/Identity Self-Registration (Week 2)

**Goal:** Providers and identities register themselves

**Tasks:**
1. Add `init()` functions to all provider implementations:
   - `pkg/auth/providers/aws/sso.go`
   - `pkg/auth/providers/aws/saml.go`
   - `pkg/auth/providers/github/oidc.go`
   - `pkg/auth/providers/mock/provider.go`
2. Add `init()` functions to all identity implementations:
   - `pkg/auth/identities/aws/permission_set.go`
   - `pkg/auth/identities/aws/assume_role.go`
   - `pkg/auth/identities/aws/user.go`
   - `pkg/auth/providers/mock/identity.go`
3. Verify all registrations happen during initialization
4. Add tests to verify registration count and kinds

**Success Criteria:**
- ✅ All providers/identities self-register
- ✅ `registry.ListProviderKinds()` returns all kinds
- ✅ `registry.ListIdentityKinds()` returns all kinds
- ✅ No behavior changes from user perspective

### Phase 3: Update Auth Manager and Validation (Week 3)

**Goal:** Auth manager and validator use registry instead of factory

**Tasks:**
1. Update `pkg/auth/manager.go`:
   - Add blank imports for providers/identities
   - Replace `factory.NewProvider()` with `registry.NewProvider()`
   - Replace `factory.NewIdentity()` with `registry.NewIdentity()`
2. Update `pkg/auth/validation/validator.go`:
   - Replace `factory.NewProvider()` with `registry.NewProvider()` in `ValidateProvider()`
   - Replace `factory.NewIdentity()` with `registry.NewIdentity()` in `ValidateIdentity()`
   - Update import to use registry instead of factory
3. Update error messages to reference registry
4. Add integration tests for:
   - Auth manager with registry
   - Validator with invalid provider/identity kinds
   - Validator with valid provider/identity kinds
5. Verify all existing auth tests pass
6. Add validation tests for unregistered kinds

**Success Criteria:**
- ✅ Auth manager uses registry exclusively
- ✅ Validator uses registry for kind validation
- ✅ Invalid provider kinds caught at validation time
- ✅ Invalid identity kinds caught at validation time
- ✅ All existing auth tests pass
- ✅ No behavior changes for users
- ✅ Integration tests pass

### Phase 4: Deprecate Factory (Week 4)

**Goal:** Mark factory as deprecated

**Tasks:**
1. Update `pkg/auth/factory/factory.go` with deprecation notices
2. Make factory functions delegate to registry
3. Add compile-time deprecation warnings (Go 1.22+)
4. Update internal documentation
5. Search codebase for direct factory usage and update

**Success Criteria:**
- ✅ Factory marked as deprecated
- ✅ Factory delegates to registry (backward compatible)
- ✅ No direct factory usage in codebase
- ✅ Documentation updated

### Phase 5: Documentation (Week 5)

**Goal:** Document new pattern

**Tasks:**
1. Update developer guide with registry pattern
2. Document how to add custom providers/identities
3. Add examples for self-registration
4. Update architecture diagrams
5. Document plugin readiness for future

**Success Criteria:**
- ✅ Complete developer documentation
- ✅ Clear examples and guidelines
- ✅ Architecture diagrams updated
- ✅ Plugin architecture documented

### Phase 6: Future - Plugin Support (Post-refactoring)

**Goal:** Enable external auth plugins

This phase is **not required for initial implementation** but provides a clear path forward.

**Tasks:**
1. Define plugin discovery mechanism
2. Create plugin loading system
3. Add plugin configuration to `atmos.yaml`
4. Add security model for plugins
5. Document plugin development guide

**Success Criteria:**
- ✅ External plugins can register providers/identities
- ✅ Plugins load from configured directory
- ✅ Plugin security model in place
- ✅ Developer guide for plugins

## Backward Compatibility

**No breaking changes:**

1. **Auth configuration unchanged** - Users don't modify `atmos.yaml` auth config
2. **Provider/identity behavior unchanged** - Same authentication flows
3. **API compatibility** - Factory package remains (deprecated but functional)
4. **Test compatibility** - All existing tests pass unchanged

**Migration for external code:**

If external code imports the factory (unlikely):

```go
// OLD (deprecated but still works)
import "github.com/cloudposse/atmos/pkg/auth/factory"

provider, err := factory.NewProvider(name, config)

// NEW (recommended)
import "github.com/cloudposse/atmos/pkg/auth/registry"

provider, err := registry.NewProvider(name, config)
```

## Benefits

### Immediate Benefits

1. ✅ **Architectural consistency** - Auth uses same pattern as commands/components
2. ✅ **Type discovery** - Can list available providers/identities programmatically
3. ✅ **Extensibility foundation** - Ready for future plugins
4. ✅ **Loose coupling** - Auth manager and validator don't import all implementations
5. ✅ **Better testability** - Easy to reset registry and test with mocks
6. ✅ **Early validation** - Invalid kinds caught at configuration validation time
7. ✅ **Better error messages** - Registry provides specific errors for unknown kinds

### Future Benefits

6. ✅ **Plugin support** - External auth providers/identities
7. ✅ **Enterprise customization** - Custom IdP integration without forking
8. ✅ **Community extensions** - Share auth methods
9. ✅ **Vendor flexibility** - Not locked into specific auth methods

## Risks & Mitigation

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing auth flows | Low | High | Comprehensive testing, gradual rollout |
| Init() ordering issues | Low | Medium | Explicit blank imports in auth manager |
| Thread safety bugs | Low | Medium | Extensive concurrent tests, race detector |
| Testing complexity | Low | Low | Registry reset functions, clear test patterns |

## Success Criteria

### Phase 1-4: Refactoring Complete

- ✅ Registry pattern implemented and tested
- ✅ All providers/identities self-register
- ✅ Auth manager uses registry exclusively
- ✅ Factory deprecated but functional
- ✅ 90%+ test coverage on registries
- ✅ All existing auth tests pass
- ✅ No behavior changes from user perspective

### Phase 5: Documentation Complete

- ✅ Developer guide updated
- ✅ Architecture documented
- ✅ Examples provided
- ✅ Plugin readiness documented

## FAQ

### Q: Will this break existing auth configurations?

**A:** No. Auth configurations in `atmos.yaml` remain unchanged. The registry pattern is purely internal.

### Q: Can users add custom auth providers now?

**A:** Not immediately. The refactoring lays the foundation, but external plugin support requires additional work (Phase 6).

### Q: Why not keep the factory pattern?

**A:** The factory pattern works but limits extensibility. Registry pattern:
- Aligns with command/component architecture
- Enables future plugins
- Provides type discovery
- Reduces coupling

### Q: What happens to the factory package?

**A:** It becomes deprecated but remains functional for backward compatibility. It delegates to the registry internally.

### Q: Will this affect performance?

**A:** No measurable impact. Registry lookup is O(1) with minimal overhead.

### Q: How do I test custom providers with the registry?

**A:** Use `registry.ResetProviders()` in tests to start with a clean state, then register your mock provider:

```go
func TestCustomProvider(t *testing.T) {
    registry.ResetProviders()
    registry.RegisterProvider("custom", func(name string, config *schema.Provider) (types.Provider, error) {
        return &mockProvider{}, nil
    })

    // Test code using custom provider
}
```

### Q: How does validation work with the registry?

**A:** Validation uses the registry to check if provider/identity kinds exist:

1. **Kind validation** - Registry checks if the kind is registered when creating instances
2. **Early error detection** - Invalid kinds are caught during `ValidateAuthConfig()` before authentication
3. **Better error messages** - Registry provides specific errors for unknown kinds
4. **Provider-specific validation** - Each provider/identity validates its own configuration via `Validate()` method
5. **Reference validation** - Validator checks that referenced providers/identities exist in config
6. **Cycle detection** - Validator checks for circular identity chains

**Example error flow:**

```yaml
# atmos.yaml with invalid provider kind
auth:
  providers:
    my-provider:
      kind: nonexistent-provider  # This kind is not registered
```

```bash
$ atmos auth login my-identity
Error: provider "my-provider" validation failed: unsupported provider kind: nonexistent-provider
```

**Benefits:**
- ✅ Invalid kinds caught at startup (validation time)
- ✅ User never tries to authenticate with invalid provider
- ✅ Clear error message pointing to the problem
- ✅ Registry provides list of valid kinds for error message

### Q: What happens if I configure an invalid provider/identity kind?

**A:** The auth configuration validation will fail with a clear error message:

```go
validator := validation.NewValidator()
err := validator.ValidateAuthConfig(config)
// Returns: ErrInvalidAuthConfig: provider "my-provider" validation failed:
//          unsupported provider kind: invalid-kind
```

This happens **before** any authentication attempt, during configuration loading. The user will see the error immediately when Atmos starts, not when trying to authenticate.

**Valid kinds are:**

Providers:
- `aws/iam-identity-center`
- `aws/saml`
- `github/oidc`
- `mock` (testing only)

Identities:
- `aws/permission-set`
- `aws/assume-role`
- `aws/user`
- `mock` (testing only)

Future plugins will add their kinds to the registry automatically.

## References

- [Command Registry Pattern PRD](command-registry-pattern.md)
- [Component Registry Pattern PRD](component-registry-pattern.md)
- [Registry Pattern vs Factory Pattern](../registry-pattern-vs-factory-pattern.md)
- [Go Plugin Documentation](https://pkg.go.dev/plugin)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-10-29 | Initial PRD for auth registry pattern refactoring |
| 1.1 | 2025-10-29 | Added validation section showing registry integration |
| | | Added validation tests for invalid provider/identity kinds |
| | | Updated migration plan to include validator updates |
| | | Added FAQ entries for validation with registry |
| | | Updated benefits to include validation improvements |
