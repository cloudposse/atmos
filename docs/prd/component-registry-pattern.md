# Component Registry Pattern

## Overview

This document describes the component registry pattern for Atmos, which provides a modular, extensible architecture for component types (Terraform, Helmfile, Packer, and future types) using a plugin-like registry system similar to the command registry pattern.

## Problem Statement

### Current State

Atmos currently supports three component types with **hardcoded, type-specific implementations**:

```go
// Current approach: String-based type checking
if componentType == "terraform" {
    return ExecuteTerraform(info)
} else if componentType == "helmfile" {
    return ExecuteHelmfile(info)
} else if componentType == "packer" {
    return ExecutePacker(info)
}

// Component listing with hardcoded types
if terraformComponents, ok := componentsMap["terraform"].(map[string]any); ok {
    allComponents = append(allComponents, lo.Keys(terraformComponents)...)
}
if helmfileComponents, ok := componentsMap["helmfile"].(map[string]any); ok {
    allComponents = append(allComponents, lo.Keys(helmfileComponents)...)
}
if packerComponents, ok := componentsMap["packer"].(map[string]any); ok {
    allComponents = append(allComponents, lo.Keys(packerComponents)...)
}
```

### Challenges

1. **Limited extensibility** - Adding new component types (CDK, Pulumi, CloudFormation) requires modifying core code
2. **No plugin support** - Cannot add external component types without forking
3. **Type detection brittleness** - Try/catch pattern for type detection is fragile
4. **Code duplication** - Similar logic repeated across component types
5. **Testing complexity** - Difficult to test without real cloud provider binaries
6. **Inconsistent patterns** - Component architecture differs from command registry pattern

### Key Requirements

1. **Maintain backward compatibility** - Existing configurations must work unchanged
2. **Support all current functionality**:
   - `atmos describe affected` - Must work with all component types
   - `atmos list components` - Must discover all registered types
   - `atmos describe stacks` - Must process all component configurations
   - `atmos describe component` - Must work across all types
   - Component execution (terraform/helmfile/packer commands)
3. **Enable future extensibility** - Plugin system for external component types
4. **Preserve YAML-driven configuration** - Map-based component configs remain
5. **No breaking changes** - Zero impact on end users

## Solution: Component Registry Pattern

### Architecture Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                       Atmos CLI                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────┐             │
│  │       Component Registry                   │             │
│  │  (pkg/component/registry.go)               │             │
│  └────────────────────────────────────────────┘             │
│           │                        │                         │
│           │                        │                         │
│           ▼                        ▼                         │
│  ┌─────────────────┐    ┌──────────────────────┐           │
│  │  Built-in       │    │  Plugin Components   │           │
│  │  Components     │    │  (future)            │           │
│  │                 │    │                      │           │
│  │  - terraform    │    │  - CDK (AWS)         │           │
│  │  - helmfile     │    │  - Pulumi            │           │
│  │  - packer       │    │  - CloudFormation    │           │
│  │  - mock (POC)   │    │  - Custom types      │           │
│  └─────────────────┘    └──────────────────────┘           │
│                                                              │
│  Component Discovery:                                        │
│  1. Load built-in components via registry                   │
│  2. Component types self-register via init()                │
│  3. All commands use registry for component operations      │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Parallel with command registry** - Same patterns and conventions
2. **Self-registration** - Components register themselves via `init()`
3. **Type safety** - Interface-based instead of string-based
4. **Backward compatibility** - Wraps existing implementation
5. **Testability** - Mock providers for unit tests
6. **Plugin readiness** - Foundation for future external plugins
7. **Component-specific schemas** - Each component type defines its own configuration structure
8. **Cross-component dependencies** - Component dependencies work across all types in the registry

## Component Configuration Architecture

### Configuration Levels

Each component type in Atmos has **configuration at multiple levels**, exactly like Terraform, Helmfile, and Packer currently work:

#### 1. Global Configuration (`atmos.yaml`)

Component types define their global settings in the Atmos configuration:

```yaml
# atmos.yaml
components:
  terraform:
    base_path: "components/terraform"
    command: "/usr/local/bin/terraform"
    apply_auto_approve: false
    init:
      pass_vars: false
    plan:
      skip_planfile: false

  helmfile:
    base_path: "components/helmfile"
    command: "/usr/local/bin/helmfile"
    use_eks: true
    kubeconfig_path: "/dev/shm"
    cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-eks-cluster"

  packer:
    base_path: "components/packer"
    command: "/usr/local/bin/packer"

  # Plugin types (POC - not for user documentation)
  plugins:
    mock:
      base_path: "components/mock"
      enabled: true
      dry_run: false
      tags:
        - test
        - development
      metadata:
        owner: "platform-team"
        version: "1.0.0"
      dependencies: []
```

**Schema in Go:**

```go
// pkg/schema/schema.go
type Components struct {
    // Built-in component types (legacy - will migrate to plugin model)
    Terraform Terraform `yaml:"terraform" json:"terraform" mapstructure:"terraform"`
    Helmfile  Helmfile  `yaml:"helmfile" json:"helmfile" mapstructure:"helmfile"`
    Packer    Packer    `yaml:"packer" json:"packer" mapstructure:"packer"`

    // Dynamic plugin component types
    // Uses mapstructure:",remain" to capture all unmapped fields
    Plugins   map[string]any `yaml:",inline" json:",inline" mapstructure:",remain"`
}

// GetComponentConfig retrieves configuration for any component type
// Works for both built-in and plugin types
func (c *Components) GetComponentConfig(componentType string) (any, bool) {
    switch componentType {
    case "terraform":
        return c.Terraform, true
    case "helmfile":
        return c.Helmfile, true
    case "packer":
        return c.Packer, true
    default:
        // Check plugin types
        if config, ok := c.Plugins[componentType]; ok {
            return config, true
        }
        return nil, false
    }
}

// Mock component configuration (POC - uses plugin model)
// This demonstrates how future plugins will register their config
type MockComponentConfig struct {
    BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
    Timeout  int    `yaml:"timeout" json:"timeout" mapstructure:"timeout"`
    DryRun   bool   `yaml:"dry_run" json:"dry_run" mapstructure:"dry_run"`
}
```

**Configuration Access Pattern:**

```go
// Built-in types use type-safe access
terraformConfig := atmosConfig.Components.Terraform

// Plugin types use helper method
if rawConfig, ok := atmosConfig.Components.GetComponentConfig("mock"); ok {
    // Parse raw map into typed config
    mockConfig := parseComponentConfig[MockComponentConfig](rawConfig)
}
```

#### 2. Stack Configuration

Component instances are configured in stack files with **full flexibility** on schema:

```yaml
# stacks/catalog/myapp.yaml
components:
  terraform:
    vpc:
      metadata:
        component: "vpc"
        type: terraform
      settings:
        spacelift:
          workspace_enabled: true
      vars:
        cidr: "10.0.0.0/16"
        azs: ["us-east-1a", "us-east-1b"]
      env:
        TF_LOG: "DEBUG"

  helmfile:
    myapp:
      metadata:
        component: "myapp"
        type: helmfile
      vars:
        replicas: 3
        image_tag: "v1.2.3"

  # Mock component (for testing)
  mock:
    test-component:
      metadata:
        component: "test-component"
        type: mock
      settings:
        timeout: 120
      vars:
        test_mode: true
        expected_result: "success"
```

**Key Points:**

1. **Schema is NOT enforced by registry** - Component types define what's valid for them
2. **Standard sections preserved**:
   - `metadata` - Component metadata (type, component name, etc.)
   - `settings` - Component-specific settings
   - `vars` - Variables passed to the component
   - `env` - Environment variables
   - `backend` - Backend configuration (Terraform)
   - `remote_state_backend` - Remote state backend (Terraform)
   - `providers` - Provider configuration (Terraform)
3. **Each component type validates its own schema** via `ValidateComponent()`

#### 3. Stack Inheritance & Merging

Component configurations are **merged through stack inheritance** exactly as they work today:

```yaml
# stacks/orgs/acme/_defaults.yaml
components:
  terraform:
    vpc:
      settings:
        spacelift:
          workspace_enabled: true
      vars:
        enable_nat_gateway: true
        tags:
          Team: "platform"

# stacks/orgs/acme/prod/us-east-1.yaml
import:
  - orgs/acme/_defaults

components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"  # Merged with parent
        tags:
          Environment: "prod"  # Merged with parent tags
```

**Result after deep merge:**

```yaml
components:
  terraform:
    vpc:
      settings:
        spacelift:
          workspace_enabled: true
      vars:
        enable_nat_gateway: true
        cidr: "10.0.0.0/16"
        tags:
          Team: "platform"        # From parent
          Environment: "prod"     # From child
```

**This merging is component-type agnostic** - the registry doesn't care about internal structure.

### Cross-Component Dependencies (DAG)

**Component dependencies work across ALL component types** - not just within a single type.

#### Dependency Syntax

Dependencies are declared in `metadata.depends_on`:

```yaml
components:
  terraform:
    vpc:
      metadata:
        component: "vpc"
        type: terraform
      vars:
        cidr: "10.0.0.0/16"

  terraform:
    eks-cluster:
      metadata:
        component: "eks-cluster"
        type: terraform
        depends_on:
          - component: vpc
            type: terraform  # Can depend on same type
      vars:
        cluster_name: "prod-cluster"

  helmfile:
    myapp:
      metadata:
        component: "myapp"
        type: helmfile
        depends_on:
          - component: eks-cluster
            type: terraform  # Can depend on different type!
      vars:
        replicas: 3

  # Mock component can have dependencies too
  mock:
    test-runner:
      metadata:
        component: "test-runner"
        type: mock
        depends_on:
          - component: myapp
            type: helmfile  # Can depend on any registered type
      vars:
        test_suite: "integration"
```

#### Dependency Resolution

The dependency resolver is **type-agnostic** and works with the registry:

```go
// internal/exec/describe_affected.go (updated)
func BuildDependencyGraph(
    atmosConfig *schema.AtmosConfiguration,
    stacksMap map[string]any,
) (*DependencyGraph, error) {
    graph := NewDependencyGraph()

    // Iterate through all registered component types
    componentTypes := component.ListTypes()

    for stackName, stackConfig := range stacksMap {
        stackData := stackConfig.(map[string]any)
        componentsSection := stackData["components"].(map[string]any)

        // Process each component type
        for _, componentType := range componentTypes {
            provider, ok := component.GetProvider(componentType)
            if !ok {
                continue
            }

            typeComponents := componentsSection[componentType].(map[string]any)

            for componentName, componentConfig := range typeComponents {
                config := componentConfig.(map[string]any)

                // Extract dependencies from metadata
                metadata := config["metadata"].(map[string]any)
                dependsOn := metadata["depends_on"].([]any)

                // Add component to graph
                node := ComponentNode{
                    Type:      componentType,
                    Component: componentName,
                    Stack:     stackName,
                }
                graph.AddNode(node)

                // Add edges for dependencies (cross-type!)
                for _, dep := range dependsOn {
                    depMap := dep.(map[string]any)
                    depNode := ComponentNode{
                        Type:      depMap["type"].(string),
                        Component: depMap["component"].(string),
                        Stack:     stackName,
                    }
                    graph.AddEdge(node, depNode)
                }
            }
        }
    }

    return graph, nil
}

type ComponentNode struct {
    Type      string  // "terraform", "helmfile", "packer", "mock"
    Component string
    Stack     string
}
```

#### DAG Output

The `deps` field in `atmos describe component` output shows **all dependencies** regardless of type:

```yaml
# atmos describe component myapp -s prod/us-east-1
component: myapp
component_type: helmfile
stack: prod/us-east-1
deps:
  - catalog/terraform/vpc
  - catalog/terraform/eks-cluster
  - catalog/helmfile/myapp
deps_all:
  - catalog/terraform/vpc
  - catalog/terraform/eks-cluster
  - catalog/helmfile/myapp
  - orgs/acme/_defaults
  - orgs/acme/prod/us-east-1
```

**Key Points:**

1. ✅ **Dependencies work across component types** - Helmfile can depend on Terraform
2. ✅ **Mock components participate in DAG** - Same as real components
3. ✅ **Affected analysis uses DAG** - Changes to VPC affect EKS and myapp
4. ✅ **Deployment ordering** - Components deployed in dependency order
5. ✅ **Type-agnostic graph** - Registry provides type information

### Component-Specific Settings

Each component type can have **arbitrary settings** that are specific to that type:

```yaml
# Terraform-specific settings
components:
  terraform:
    vpc:
      settings:
        spacelift:
          workspace_enabled: true
          autodeploy: true
        validation:
          schema: "vpc-schema.json"

# Helmfile-specific settings
components:
  helmfile:
    myapp:
      settings:
        release_name: "myapp-release"
        namespace: "applications"
        wait: true
        timeout: 600

# Mock-specific settings (POC)
components:
  mock:
    test-component:
      settings:
        timeout: 120
        retry_count: 3
        dry_run: false
```

**Component providers validate their own settings:**

```go
func (t *TerraformComponentProvider) ValidateComponent(config map[string]any) error {
    settings, ok := config["settings"].(map[string]any)
    if !ok {
        return nil // Settings optional
    }

    // Validate Terraform-specific settings
    if spacelift, ok := settings["spacelift"].(map[string]any); ok {
        if _, ok := spacelift["workspace_enabled"].(bool); !ok {
            return fmt.Errorf("spacelift.workspace_enabled must be boolean")
        }
    }

    return nil
}
```

### Schema Extensibility: Hybrid Approach

The `Components` struct uses a **hybrid approach** that supports both built-in types (with type safety) and dynamic plugin types:

**Current State (Phase 1-3):**

```yaml
# atmos.yaml - Hybrid configuration
components:
  # Built-in types (type-safe structs)
  terraform:
    base_path: "components/terraform"
    command: "/usr/local/bin/terraform"
    apply_auto_approve: false

  helmfile:
    base_path: "components/helmfile"
    use_eks: true

  packer:
    base_path: "components/packer"

  # Plugin types (dynamic map) - Mock uses this pattern
  # Demonstrates inheritance, array/object merging, and dependency tracking
  mock:
    base_path: "components/mock"
    enabled: true
    dry_run: false
    tags:
      - test
      - development
    metadata:
      owner: "platform-team"
      version: "1.0.0"
    dependencies: []

  # Future plugin examples (Phase 4+)
  # pulumi:
  #   base_path: "components/pulumi"
  #   backend: "s3://my-bucket/pulumi"
  # cdk:
  #   base_path: "components/cdk"
  #   bootstrap: true
```

**How It Works:**

1. **Built-in types** (terraform, helmfile, packer): Access via `atmosConfig.Components.Terraform.BasePath`
2. **Plugin types** (mock, pulumi, cdk): Access via `atmosConfig.Components.Plugins["mock"]`
3. **Helper method** unifies access: `GetComponentConfig(componentType)`

**Component Provider Pattern:**

Each component provider can define its own config structure:

```go
// pkg/component/mock/config.go
package mock

type Config struct {
    BasePath string `yaml:"base_path" mapstructure:"base_path"`
    Timeout  int    `yaml:"timeout" mapstructure:"timeout"`
    DryRun   bool   `yaml:"dry_run" mapstructure:"dry_run"`
}

// Provider accesses config from Plugins map
func (m *MockComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
    rawConfig, ok := atmosConfig.Components.GetComponentConfig("mock")
    if !ok {
        return "components/mock" // Default
    }

    // Parse raw config into typed struct
    config := parseConfig[Config](rawConfig)
    return config.BasePath
}
```

**Migration Path (Future):**

Eventually, Terraform, Helmfile, and Packer will migrate to the plugin model:

```go
// Phase 5+ (Future): All types use plugin model
type Components struct {
    // All component types in Plugins map
    Plugins map[string]any `yaml:",inline" mapstructure:",remain"`

    // Deprecated: kept for backward compatibility
    Terraform Terraform `yaml:"terraform,omitempty" mapstructure:"terraform,omitempty"`
    Helmfile  Helmfile  `yaml:"helmfile,omitempty" mapstructure:"helmfile,omitempty"`
    Packer    Packer    `yaml:"packer,omitempty" mapstructure:"packer,omitempty"`
}
```

This gradual migration ensures:
- ✅ No breaking changes during transition
- ✅ Terraform/Helmfile/Packer continue to work
- ✅ New plugins use modern pattern
- ✅ Built-ins can migrate incrementally

## Implementation

### 1. ComponentProvider Interface

```go
// pkg/component/provider.go
package component

import (
    "context"
    "github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProvider is the interface that component type implementations
// must satisfy to register with the Atmos component registry.
type ComponentProvider interface {
    // GetType returns the component type identifier (e.g., "terraform", "helmfile").
    GetType() string

    // GetGroup returns the component group for categorization.
    // Examples: "Infrastructure as Code", "Kubernetes", "Images"
    GetGroup() string

    // GetBasePath returns the base directory path for this component type.
    // Example: For terraform, returns "components/terraform"
    GetBasePath(atmosConfig *schema.AtmosConfiguration) string

    // ListComponents discovers all components of this type in a stack.
    // Returns component names found in the stack configuration.
    ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error)

    // ValidateComponent validates component configuration.
    // Returns error if configuration is invalid for this component type.
    ValidateComponent(config map[string]any) error

    // Execute runs a command for this component type.
    // Context provides all necessary information for execution.
    Execute(ctx *ExecutionContext) error

    // GenerateArtifacts creates necessary files for component execution.
    // Examples: backend.tf for Terraform, varfile for Helmfile
    GenerateArtifacts(ctx *ExecutionContext) error

    // GetAvailableCommands returns list of commands this component type supports.
    // Example: For terraform: ["plan", "apply", "destroy", "workspace"]
    GetAvailableCommands() []string
}

// ExecutionContext provides all necessary context for component execution.
type ExecutionContext struct {
    AtmosConfig        *schema.AtmosConfiguration
    ComponentType      string
    Component          string
    Stack              string
    Command            string
    SubCommand         string
    ComponentConfig    map[string]any
    ConfigAndStacksInfo schema.ConfigAndStacksInfo
    Args               []string
    Flags              map[string]any
}

// ComponentInfo provides metadata about a component provider.
type ComponentInfo struct {
    Type        string
    Group       string
    Commands    []string
    Description string
}
```

### 2. Component Registry

```go
// pkg/component/registry.go
package component

import (
    "fmt"
    "sort"
    "sync"
)

var (
    // Global registry instance.
    registry = &ComponentRegistry{
        providers: make(map[string]ComponentProvider),
    }
)

// ComponentRegistry manages component provider registration.
type ComponentRegistry struct {
    mu        sync.RWMutex
    providers map[string]ComponentProvider
}

// Register adds a component provider to the registry.
// This is called during package init() for built-in components.
// Returns an error if the provider is nil, has an empty type, or is already registered.
// Callers should handle the returned error appropriately.
func Register(provider ComponentProvider) error {
    registry.mu.Lock()
    defer registry.mu.Unlock()

    if provider == nil {
        return fmt.Errorf("component provider cannot be nil")
    }

    componentType := provider.GetType()
    if componentType == "" {
        return fmt.Errorf("component type cannot be empty")
    }

    if _, exists := registry.providers[componentType]; exists {
        return fmt.Errorf("component %s already registered", componentType)
    }

    registry.providers[componentType] = provider
    return nil
}

// GetProvider returns a component provider by type.
func GetProvider(componentType string) (ComponentProvider, bool) {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    provider, ok := registry.providers[componentType]
    return provider, ok
}

// ListProviders returns all registered providers grouped by category.
func ListProviders() map[string][]ComponentProvider {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    grouped := make(map[string][]ComponentProvider)

    for _, provider := range registry.providers {
        group := provider.GetGroup()
        grouped[group] = append(grouped[group], provider)
    }

    return grouped
}

// ListTypes returns all registered component types sorted alphabetically.
func ListTypes() []string {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    types := make([]string, 0, len(registry.providers))
    for componentType := range registry.providers {
        types = append(types, componentType)
    }

    sort.Strings(types)
    return types
}

// Count returns the number of registered providers.
func Count() int {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    return len(registry.providers)
}

// GetInfo returns metadata for all registered component providers.
func GetInfo() []ComponentInfo {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    infos := make([]ComponentInfo, 0, len(registry.providers))
    for _, provider := range registry.providers {
        infos = append(infos, ComponentInfo{
            Type:     provider.GetType(),
            Group:    provider.GetGroup(),
            Commands: provider.GetAvailableCommands(),
        })
    }

    sort.Slice(infos, func(i, j int) bool {
        return infos[i].Type < infos[j].Type
    })

    return infos
}

// Reset clears the registry (for testing only).
func Reset() {
    registry.mu.Lock()
    defer registry.mu.Unlock()

    registry.providers = make(map[string]ComponentProvider)
}
```

### 3. Mock Component Provider (POC)

```go
// pkg/component/mock/mock.go
package mock

import (
    "context"
    "fmt"

    "github.com/cloudposse/atmos/pkg/component"
    "github.com/cloudposse/atmos/pkg/schema"
)

// MockComponentProvider is a proof-of-concept component type for testing
// the component registry pattern. It demonstrates the interface implementation
// without requiring external tools or cloud providers.
//
// This component type is NOT documented for end users and is intended only
// for development and testing purposes.
type MockComponentProvider struct{}

func init() {
    // Self-register with the component registry.
    if err := component.Register(&MockComponentProvider{}); err != nil {
        panic(fmt.Sprintf("failed to register mock component provider: %v", err))
    }
}

func (m *MockComponentProvider) GetType() string {
    return "mock"
}

func (m *MockComponentProvider) GetGroup() string {
    return "Testing"
}

func (m *MockComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
    if atmosConfig == nil {
        return "components/mock"
    }

    // Try to get config from Plugins map.
    rawConfig, ok := atmosConfig.Components.GetComponentConfig("mock")
    if !ok {
        return "components/mock"
    }

    // Parse raw config into typed struct.
    config, err := parseConfig(rawConfig)
    if err != nil {
        return "components/mock"
    }

    if config.BasePath != "" {
        return config.BasePath
    }

    return "components/mock"
}

func (m *MockComponentProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
    componentsSection, ok := stackConfig["components"].(map[string]any)
    if !ok {
        return []string{}, nil
    }

    mockComponents, ok := componentsSection["mock"].(map[string]any)
    if !ok {
        return []string{}, nil
    }

    componentNames := make([]string, 0, len(mockComponents))
    for name := range mockComponents {
        componentNames = append(componentNames, name)
    }

    return componentNames, nil
}

func (m *MockComponentProvider) ValidateComponent(config map[string]any) error {
    // Mock validation - always succeeds unless explicitly marked invalid
    if invalid, ok := config["invalid"].(bool); ok && invalid {
        return fmt.Errorf("mock component validation failed: invalid flag set")
    }
    return nil
}

func (m *MockComponentProvider) Execute(ctx component.ExecutionContext) error {
    // Mock execution - simulates command execution without external dependencies
    fmt.Printf("Mock component execution:\n")
    fmt.Printf("  Type: %s\n", ctx.ComponentType)
    fmt.Printf("  Component: %s\n", ctx.Component)
    fmt.Printf("  Stack: %s\n", ctx.Stack)
    fmt.Printf("  Command: %s\n", ctx.Command)

    if ctx.SubCommand != "" {
        fmt.Printf("  SubCommand: %s\n", ctx.SubCommand)
    }

    return nil
}

func (m *MockComponentProvider) GenerateArtifacts(ctx component.ExecutionContext) error {
    // Mock artifact generation - no actual files created
    fmt.Printf("Mock artifact generation for %s in stack %s\n", ctx.Component, ctx.Stack)
    return nil
}

func (m *MockComponentProvider) GetAvailableCommands() []string {
    return []string{"plan", "apply", "destroy", "validate"}
}
```

### 4. Terraform Component Provider

```go
// pkg/component/terraform/terraform.go
package terraform

import (
    "context"

    "github.com/cloudposse/atmos/pkg/component"
    e "github.com/cloudposse/atmos/internal/exec"
    "github.com/cloudposse/atmos/pkg/schema"
)

// TerraformComponentProvider implements ComponentProvider for Terraform.
type TerraformComponentProvider struct{}

func init() {
    component.Register(&TerraformComponentProvider{})
}

func (t *TerraformComponentProvider) GetType() string {
    return "terraform"
}

func (t *TerraformComponentProvider) GetGroup() string {
    return "Infrastructure as Code"
}

func (t *TerraformComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
    if atmosConfig.Components.Terraform.BasePath != "" {
        return atmosConfig.Components.Terraform.BasePath
    }
    return "components/terraform"
}

func (t *TerraformComponentProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
    componentsSection, ok := stackConfig["components"].(map[string]any)
    if !ok {
        return []string{}, nil
    }

    terraformComponents, ok := componentsSection["terraform"].(map[string]any)
    if !ok {
        return []string{}, nil
    }

    componentNames := make([]string, 0, len(terraformComponents))
    for name := range terraformComponents {
        componentNames = append(componentNames, name)
    }

    return componentNames, nil
}

func (t *TerraformComponentProvider) ValidateComponent(config map[string]any) error {
    // Delegate to existing validation logic
    return e.ValidateTerraformComponent(config)
}

func (t *TerraformComponentProvider) Execute(ctx component.ExecutionContext) error {
    // Delegate to existing execution logic
    return e.ExecuteTerraform(ctx.ConfigAndStacksInfo)
}

func (t *TerraformComponentProvider) GenerateArtifacts(ctx component.ExecutionContext) error {
    // Delegate to existing artifact generation
    return e.GenerateTerraformBackendAndVarfile(ctx.ConfigAndStacksInfo)
}

func (t *TerraformComponentProvider) GetAvailableCommands() []string {
    return []string{
        "plan", "apply", "destroy", "workspace", "import", "state",
        "refresh", "output", "show", "validate", "init", "providers",
        "fmt", "version", "taint", "untaint", "force-unlock", "console",
        "get", "graph", "test",
    }
}
```

### 5. Integration with List Components

```go
// pkg/list/list_components.go (updated)
package list

import (
    "context"
    "github.com/cloudposse/atmos/pkg/component"
    "github.com/samber/lo"
)

func FilterAndListComponents(stackFlag string, stacksMap map[string]any) ([]string, error) {
    allComponents := []string{}

    // Use component registry instead of hardcoded types
    componentTypes := component.ListTypes()

    for _, stack := range stacksList {
        stackConfig := stacksMap[stack].(map[string]any)

        // Iterate through registered component types
        for _, componentType := range componentTypes {
            provider, ok := component.GetProvider(componentType)
            if !ok {
                continue
            }

            components, err := provider.ListComponents(context.Background(), stack, stackConfig)
            if err != nil {
                return nil, err
            }

            allComponents = append(allComponents, components...)
        }
    }

    return lo.Uniq(allComponents), nil
}
```

### 6. Integration with Describe Component

```go
// internal/exec/describe_component.go (updated)
func ExecuteDescribeComponentWithContext(...) (*DescribeComponentResult, error) {
    var configAndStacksInfo schema.ConfigAndStacksInfo
    var err error

    // Try registered component types instead of hardcoded list
    componentTypes := component.ListTypes()

    for _, componentType := range componentTypes {
        provider, ok := component.GetProvider(componentType)
        if !ok {
            continue
        }

        configAndStacksInfo, err = ProcessStacks(
            atmosConfig,
            stack,
            component,
            componentType,
            // ... other params
        )

        if err == nil {
            // Successfully found component type
            break
        }
    }

    if err != nil {
        return nil, fmt.Errorf("component not found in any registered component type")
    }

    // Rest of existing logic remains unchanged
    // ...
}
```

### 7. Integration with Root Package

```go
// pkg/component/component.go (entry point)
package component

// Import all built-in component providers for side-effect registration
import (
    _ "github.com/cloudposse/atmos/pkg/component/terraform"
    _ "github.com/cloudposse/atmos/pkg/component/helmfile"
    _ "github.com/cloudposse/atmos/pkg/component/packer"
    _ "github.com/cloudposse/atmos/pkg/component/mock" // POC only
)

// Initialize initializes the component registry.
// This is called during application startup to ensure
// all component providers are registered.
func Initialize() {
    // Registration happens via init() functions in imported packages
    // This function can be used for any additional setup if needed
}
```

## Integration Points

### Commands That Use Components

All these commands will work transparently with the registry:

1. **`atmos describe affected`** ✅
   - Iterates through registered component types
   - Uses provider.ListComponents() for discovery
   - No changes to affected logic needed

2. **`atmos list components`** ✅
   - Uses component.ListTypes() instead of hardcoded types
   - Calls provider.ListComponents() for each type
   - Maintains current filtering and deduplication

3. **`atmos describe stacks`** ✅
   - Stack processing remains unchanged
   - Component configurations stored as maps (backward compatible)
   - Registry used only for type validation

4. **`atmos describe component`** ✅
   - Uses registry to try component types
   - Replaces hardcoded try/catch with registry iteration
   - Same result structure

5. **Component execution** (terraform/helmfile/packer) ✅
   - Uses provider.Execute() instead of type-specific functions
   - Existing execution logic wrapped by providers
   - No behavior changes

### Backward Compatibility

**No breaking changes:**

```yaml
# Stack configurations remain unchanged
components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"

  helmfile:
    myapp:
      vars:
        replicas: 3

  # Mock component for testing (not documented for users)
  mock:
    test-component:
      vars:
        example: value
```

**String constants preserved:**

```go
// Existing code using string constants still works
const (
    TerraformComponentType = "terraform"
    HelmfileComponentType  = "helmfile"
    PackerComponentType    = "packer"
)

// New code can use registry
provider, ok := component.GetProvider("terraform")
```

## Testing Strategy

### 1. Unit Tests for Registry

```go
// pkg/component/registry_test.go
package component

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestRegisterAndGetProvider(t *testing.T) {
    Reset() // Clear registry for clean test

    provider := &MockComponentProvider{}
    Register(provider)

    retrieved, ok := GetProvider("mock")
    assert.True(t, ok)
    assert.Equal(t, "mock", retrieved.GetType())
}

func TestListTypes(t *testing.T) {
    Reset()

    Register(&MockComponentProvider{})
    Register(&TerraformComponentProvider{})

    types := ListTypes()
    assert.Contains(t, types, "mock")
    assert.Contains(t, types, "terraform")
}

func TestListProviders(t *testing.T) {
    Reset()

    Register(&MockComponentProvider{})
    Register(&TerraformComponentProvider{})

    grouped := ListProviders()
    assert.Contains(t, grouped, "Testing")
    assert.Contains(t, grouped, "Infrastructure as Code")
}
```

### 2. Mock Component Tests

```go
// pkg/component/mock/mock_test.go
package mock

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/cloudposse/atmos/pkg/component"
)

func TestMockComponentProvider(t *testing.T) {
    provider := &MockComponentProvider{}

    assert.Equal(t, "mock", provider.GetType())
    assert.Equal(t, "Testing", provider.GetGroup())

    stackConfig := map[string]any{
        "components": map[string]any{
            "mock": map[string]any{
                "test-component": map[string]any{
                    "vars": map[string]any{
                        "example": "value",
                    },
                },
            },
        },
    }

    components, err := provider.ListComponents(context.Background(), "test-stack", stackConfig)
    assert.NoError(t, err)
    assert.Equal(t, []string{"test-component"}, components)
}

func TestMockValidation(t *testing.T) {
    provider := &MockComponentProvider{}

    // Valid component
    validConfig := map[string]any{"vars": map[string]any{}}
    err := provider.ValidateComponent(validConfig)
    assert.NoError(t, err)

    // Invalid component
    invalidConfig := map[string]any{"invalid": true}
    err = provider.ValidateComponent(invalidConfig)
    assert.Error(t, err)
}

func TestMockExecution(t *testing.T) {
    provider := &MockComponentProvider{}

    ctx := component.ExecutionContext{
        ComponentType: "mock",
        Component:     "test-component",
        Stack:         "test-stack",
        Command:       "plan",
    }

    err := provider.Execute(ctx)
    assert.NoError(t, err)
}
```

### 3. Thread Safety Tests

```go
// pkg/component/registry_concurrent_test.go
func TestConcurrentRegistration(t *testing.T) {
    Reset()

    var wg sync.WaitGroup
    numGoroutines := 100

    // Test concurrent registration doesn't cause race conditions
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            provider := &mockProvider{
                name: fmt.Sprintf("test-%d", id),
            }
            Register(provider)
        }(i)
    }

    wg.Wait()
    assert.Equal(t, numGoroutines, Count())
}

func TestConcurrentReadWrite(t *testing.T) {
    Reset()
    Register(&MockComponentProvider{})

    var wg sync.WaitGroup
    numReaders := 50
    numWriters := 10

    // Concurrent reads
    for i := 0; i < numReaders; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, ok := GetProvider("mock")
            assert.True(t, ok)
        }()
    }

    // Concurrent writes
    for i := 0; i < numWriters; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            provider := &mockProvider{
                name: fmt.Sprintf("concurrent-%d", id),
            }
            Register(provider)
        }(i)
    }

    wg.Wait()
}
```

### 4. Edge Case Tests

```go
// pkg/component/registry_edge_cases_test.go
func TestEmptyRegistry(t *testing.T) {
    Reset()

    assert.Equal(t, 0, Count())
    assert.Empty(t, ListTypes())
    assert.Empty(t, ListProviders())

    _, ok := GetProvider("nonexistent")
    assert.False(t, ok)
}

func TestDuplicateRegistration(t *testing.T) {
    Reset()

    provider1 := &MockComponentProvider{}
    provider2 := &MockComponentProvider{}

    Register(provider1)
    assert.Equal(t, 1, Count())

    // Re-registration should not error (allows override)
    Register(provider2)
    assert.Equal(t, 1, Count())

    // Latest registration wins
    retrieved, ok := GetProvider("mock")
    assert.True(t, ok)
    assert.Equal(t, provider2, retrieved)
}

func TestInvalidComponentConfig(t *testing.T) {
    provider := &MockComponentProvider{}

    tests := []struct {
        name    string
        config  map[string]any
        wantErr bool
    }{
        {
            name:    "nil config",
            config:  nil,
            wantErr: false, // Should handle gracefully
        },
        {
            name:    "empty config",
            config:  map[string]any{},
            wantErr: false,
        },
        {
            name: "invalid flag set",
            config: map[string]any{
                "invalid": true,
            },
            wantErr: true,
        },
        {
            name: "missing required fields",
            config: map[string]any{
                "vars": map[string]any{},
            },
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := provider.ValidateComponent(tt.config)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 5. Integration Tests

```go
// tests/component_registry_integration_test.go
func TestListComponentsWithRegistry(t *testing.T) {
    // Test that list components works with registered types
    // Including mock component type
}

func TestDescribeComponentWithRegistry(t *testing.T) {
    // Test that describe component detects type via registry
    // Including mock component type
}

func TestMockComponentInStacks(t *testing.T) {
    // Create test stack with mock component
    // Verify atmos describe stacks shows mock component
    // Verify atmos list components includes mock component
}

func TestCrossComponentDependencies(t *testing.T) {
    // Create stack with dependencies:
    // terraform/vpc -> helmfile/myapp -> mock/test-runner
    // Verify DAG is built correctly
    // Verify affected analysis propagates changes
}
```

### 6. Test Coverage Requirements

**Minimum Coverage Targets:**

| Package | Minimum Coverage | Focus Areas |
|---------|------------------|-------------|
| `pkg/component/registry.go` | **90%** | All public functions, concurrent access, edge cases |
| `pkg/component/mock/mock.go` | **90%** | All interface methods, validation, execution |
| `pkg/component/provider.go` | **100%** | Interface definition (trivial, but must be covered) |

**Coverage Verification:**

```bash
# Run tests with coverage
go test ./pkg/component/... -coverprofile=coverage.out -covermode=atomic

# View coverage report
go tool cover -html=coverage.out

# Check minimum thresholds
go test ./pkg/component/... -cover | grep -E '(registry|mock)' | awk '{if ($5 < 90.0) exit 1}'
```

**What Must Be Tested:**

1. **Registry Core Functions:**
   - ✅ Register() - single and multiple providers
   - ✅ GetProvider() - found and not found cases
   - ✅ ListTypes() - empty, single, multiple types
   - ✅ ListProviders() - grouping by category
   - ✅ Count() - accurate count tracking
   - ✅ Reset() - complete cleanup

2. **Thread Safety:**
   - ✅ Concurrent registration (100+ goroutines)
   - ✅ Concurrent reads (50+ goroutines)
   - ✅ Concurrent read/write mix
   - ✅ Race detector enabled (`go test -race`)

3. **Mock Component Provider:**
   - ✅ GetType() returns "mock"
   - ✅ GetGroup() returns "Testing"
   - ✅ GetBasePath() with and without config
   - ✅ ListComponents() - empty, single, multiple
   - ✅ ValidateComponent() - valid and invalid cases
   - ✅ Execute() - various command contexts
   - ✅ GenerateArtifacts() - artifact generation
   - ✅ GetAvailableCommands() - command list

4. **Edge Cases:**
   - ✅ Nil pointer checks
   - ✅ Empty registry operations
   - ✅ Duplicate registrations (override behavior)
   - ✅ Invalid component configurations
   - ✅ Missing required fields
   - ✅ Type mismatches in stack config

5. **Integration:**
   - ✅ Mock component in stack configuration
   - ✅ List components includes mock
   - ✅ Describe component detects mock
   - ✅ Cross-component dependencies work
   - ✅ DAG includes mock components

## Migration Plan

### Phase 1: Foundation (Week 1)

**Goal:** Create registry infrastructure without changing behavior

**Tasks:**
1. Create `pkg/component/provider.go` with ComponentProvider interface
2. Create `pkg/component/registry.go` with registry implementation
3. Create `pkg/component/mock/mock.go` with MockComponentProvider (POC)
4. **Update `pkg/schema/schema.go` with hybrid approach:**
   ```go
   type Components struct {
       // Built-in types (legacy - will migrate to plugin model)
       Terraform Terraform `yaml:"terraform" json:"terraform" mapstructure:"terraform"`
       Helmfile  Helmfile  `yaml:"helmfile" json:"helmfile" mapstructure:"helmfile"`
       Packer    Packer    `yaml:"packer" json:"packer" mapstructure:"packer"`

       // Dynamic plugin types (mock uses this)
       Plugins   map[string]any `yaml:",inline" json:",inline" mapstructure:",remain"`
   }

   // GetComponentConfig retrieves config for any component type
   func (c *Components) GetComponentConfig(componentType string) (any, bool) {
       switch componentType {
       case "terraform":
           return c.Terraform, true
       case "helmfile":
           return c.Helmfile, true
       case "packer":
           return c.Packer, true
       default:
           if config, ok := c.Plugins[componentType]; ok {
               return config, true
           }
           return nil, false
       }
   }
   ```
5. Create `pkg/component/mock/config.go` for mock component config struct
6. **Update JSON schemas** in `pkg/datafetcher/schema/`:
   - Update `config/global/1.0.json` to allow additional properties in components
   - Update `stacks/stack-config/1.0.json` to allow any component type
6. Add comprehensive unit test coverage:
   - **90%+ coverage** on `pkg/component/registry.go`
   - **90%+ coverage** on `pkg/component/mock/mock.go`
   - Thread safety tests (concurrent registration/access)
   - Edge cases (nil checks, empty registries, duplicate registration)
   - Integration tests for mock component in stacks
7. Document mock component for developers only (not users)

**Success Criteria:**
- ✅ Registry compiles and passes all tests
- ✅ Mock component can be registered and retrieved
- ✅ Schema changes allow mock in atmos.yaml and stacks
- ✅ No changes to existing functionality
- ✅ **90%+ test coverage** on registry and mock provider
- ✅ Thread safety verified with concurrent tests
- ✅ All edge cases covered

### Phase 2: Provider Implementation (Week 2)

**Goal:** Wrap existing component implementations

**Tasks:**
1. Create `pkg/component/terraform/terraform.go` wrapping existing logic
2. Create `pkg/component/helmfile/helmfile.go` wrapping existing logic
3. Create `pkg/component/packer/packer.go` wrapping existing logic
4. Add tests for each provider
5. Verify providers work with mock stack configurations

**Success Criteria:**
- ✅ All three providers registered successfully
- ✅ Providers delegate to existing execution logic
- ✅ No behavior changes from user perspective
- ✅ Tests pass for all providers

### Phase 3: Integration with List/Describe (Week 3)

**Goal:** Migrate list and describe commands to use registry

**Tasks:**
1. Update `pkg/list/list_components.go` to use registry instead of hardcoded types
2. Update `internal/exec/describe_component.go` to use registry for type detection
3. **Update dependency graph building** in `internal/exec/describe_affected.go`:
   - Use `component.ListTypes()` instead of hardcoded types
   - Ensure cross-component dependencies work (terraform → helmfile → mock)
   - Verify DAG output includes all component types
4. **Update provenance tracking** to work with registry:
   - Component type detection
   - Dependency tracing across types
5. Add integration tests with mock component type:
   - List components includes mock
   - Describe component works with mock
   - Describe affected shows mock dependencies
   - DAG ordering respects cross-type dependencies
6. Verify all existing tests pass
7. Test with real stack configurations

**Success Criteria:**
- ✅ `atmos list components` includes mock components
- ✅ `atmos describe component` works with mock components
- ✅ `atmos describe stacks` shows mock component configuration
- ✅ `atmos describe affected` includes mock in dependency graph
- ✅ Cross-component dependencies work (terraform → helmfile → mock)
- ✅ DAG ordering correct for mixed component types
- ✅ All existing functionality preserved
- ✅ Integration tests pass

### Phase 4: Documentation (Week 4)

**Goal:** Document the pattern for future development

**Tasks:**
1. Create developer guide for adding new component types
2. Document mock component usage for testing
3. Document hybrid configuration approach
4. Update architecture diagrams
5. Add examples of creating custom component providers
6. Document migration path for built-in types

**Success Criteria:**
- ✅ Complete developer documentation
- ✅ Clear examples and guidelines
- ✅ Mock component documented as POC/testing tool
- ✅ Hybrid approach documented
- ✅ Architecture diagrams updated

### Phase 5: Built-in Type Migration (Future)

**Goal:** Migrate Terraform, Helmfile, and Packer to plugin model

This phase is **not required for initial implementation** but provides a clear path forward.

**Tasks:**
1. **Migrate Terraform to plugin model:**
   - Create `pkg/component/terraform/config.go` with Config struct
   - Update TerraformComponentProvider to use Plugins map
   - Add deprecation warnings for direct access to `Components.Terraform`
   - Maintain backward compatibility (read from both locations)

2. **Migrate Helmfile to plugin model:**
   - Same pattern as Terraform
   - Ensure existing configurations work unchanged

3. **Migrate Packer to plugin model:**
   - Same pattern as Terraform
   - Complete migration of all built-in types

4. **Deprecation Timeline:**
   ```go
   // Phase 5.1: Add deprecation warnings
   type Components struct {
       Terraform Terraform `yaml:"terraform" json:"terraform" mapstructure:"terraform"` // Deprecated: use Plugins["terraform"]
       Helmfile  Helmfile  `yaml:"helmfile" json:"helmfile" mapstructure:"helmfile"`   // Deprecated: use Plugins["helmfile"]
       Packer    Packer    `yaml:"packer" json:"packer" mapstructure:"packer"`         // Deprecated: use Plugins["packer"]
       Plugins   map[string]any `yaml:",inline" mapstructure:",remain"`
   }

   // Phase 5.2 (Major version): Remove deprecated fields
   type Components struct {
       Plugins map[string]any `yaml:",inline" mapstructure:",remain"`
   }
   ```

5. **Update GetComponentConfig to handle migration:**
   ```go
   func (c *Components) GetComponentConfig(componentType string) (any, bool) {
       // First check Plugins map (new location)
       if config, ok := c.Plugins[componentType]; ok {
           return config, true
       }

       // Fall back to built-in fields (deprecated)
       switch componentType {
       case "terraform":
           if !reflect.ValueOf(c.Terraform).IsZero() {
               log.Warn("Using deprecated Components.Terraform field, migrate to Plugins map")
               return c.Terraform, true
           }
       case "helmfile":
           if !reflect.ValueOf(c.Helmfile).IsZero() {
               return c.Helmfile, true
           }
       case "packer":
           if !reflect.ValueOf(c.Packer).IsZero() {
               return c.Packer, true
           }
       }

       return nil, false
   }
   ```

**Success Criteria:**
- ✅ All built-in types use plugin model internally
- ✅ Backward compatibility maintained
- ✅ Deprecation warnings in place
- ✅ Documentation updated with migration guide
- ✅ All existing configurations work without changes
- ✅ Clear timeline for deprecation removal (major version)

**Benefits of Migration:**
- ✅ Consistent pattern for all component types
- ✅ No special-casing for built-in vs plugin types
- ✅ Simpler codebase (no switch statements)
- ✅ True plugin architecture
- ✅ Foundation for external plugins (Phase 6)

## Future: External Plugin Support (Phase 6+)

The component registry provides foundation for external plugins:

### Plugin Architecture (Future Phase)

```go
// pkg/plugin/component_plugin.go (Future)
package plugin

import (
    "github.com/cloudposse/atmos/pkg/component"
)

// ComponentPluginProvider wraps external component type plugins.
type ComponentPluginProvider struct {
    pluginPath string
    pluginInfo PluginInfo
}

func (p *ComponentPluginProvider) GetType() string {
    return p.pluginInfo.Type
}

func (p *ComponentPluginProvider) Execute(ctx component.ExecutionContext) error {
    // Execute external plugin binary
    return executePlugin(p.pluginPath, ctx)
}

// LoadComponentPlugins discovers and registers external component plugins.
func LoadComponentPlugins(pluginDir string) error {
    plugins := discoverPlugins(pluginDir)
    for _, plugin := range plugins {
        provider := &ComponentPluginProvider{
            pluginPath: plugin.Path,
            pluginInfo: plugin.Info,
        }
        component.Register(provider)
    }
    return nil
}
```

**Plugin discovery flow (future):**
1. Atmos starts → loads built-in components via registry
2. Discovers plugins in `~/.atmos/plugins/components/`
3. Registers plugins as ComponentProviders
4. All Atmos commands work with plugin components
5. Examples of plugin types:
   - AWS CDK components
   - Pulumi components
   - CloudFormation components
   - Ansible playbooks
   - Custom deployment tools

## Benefits

### Immediate Benefits (Phase 1-3)

1. ✅ **Type safety** - Interface-based instead of string comparisons
2. ✅ **Extensibility** - Easy to add new component types
3. ✅ **Consistency** - Parallel with command registry pattern
4. ✅ **Testability** - Mock components for unit tests without cloud dependencies
5. ✅ **Maintainability** - Clear contract for all component types
6. ✅ **Discoverability** - Introspection of available component types

### Future Benefits (Phase 4+)

7. ✅ **Plugin support** - Foundation for external component types
8. ✅ **Community extensions** - Users can share custom component types
9. ✅ **Vendor flexibility** - Not locked into specific tools
10. ✅ **Innovation** - New orchestration patterns without forking

## Risks & Mitigation

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking existing workflows | Low | High | Comprehensive testing, gradual rollout |
| Performance overhead | Low | Low | Providers wrap existing code, no additional work |
| Component type conflicts | Low | Medium | Registry allows explicit override, clear errors |
| Template function compatibility | Medium | Medium | Providers expose ToMap() for templates |
| Testing complexity | Low | Low | Mock component provides test coverage |

## Success Criteria

### Phase 1: Foundation
- ✅ Registry pattern implemented and tested
- ✅ Mock component provider working
- ✅ 100% test coverage for registry
- ✅ No behavior changes
- ✅ Documentation complete

### Phase 2: Provider Implementation
- ✅ All component types wrapped in providers
- ✅ Providers delegate to existing logic
- ✅ All existing tests pass
- ✅ No regression in functionality

### Phase 3: Integration
- ✅ List/describe commands use registry
- ✅ Mock components work in all commands
- ✅ Integration tests pass
- ✅ Performance unchanged

### Phase 4: Documentation
- ✅ Developer guide complete
- ✅ Mock component documented
- ✅ Examples provided
- ✅ Architecture documented

## FAQ

### Q: Will this break existing stack configurations?

**A:** No. Stack configurations remain unchanged. Component types are still defined as strings in YAML, and all existing configurations work exactly as before.

### Q: Why create a mock component type?

**A:** The mock component type validates the registry pattern without requiring external tools (terraform, helmfile, packer binaries) or cloud provider access. It enables:
- Fast unit testing
- CI/CD without cloud credentials
- Development without installing tools
- Proof of concept for plugin system

### Q: Is the mock component documented for users?

**A:** No. The mock component is documented only for developers in the PRD and developer guides. It's a development/testing tool, not a user-facing feature.

### Q: Can users create custom component types?

**A:** Not in Phase 1-3. The foundation is laid for plugin support in future phases, but initially only built-in types (terraform, helmfile, packer, mock) are supported.

### Q: What happens to existing string-based type checking?

**A:** String constants remain for backward compatibility. Existing code using `componentType == "terraform"` continues to work. New code should use the registry for better type safety.

### Q: How does this affect template functions?

**A:** Template functions continue to work with map-based component configurations. Providers can expose ToMap() methods for template context if needed in future phases.

### Q: What's the performance impact?

**A:** Minimal. Providers wrap existing execution logic without additional overhead. Registry lookup is O(1) with minimal allocation.

### Q: Will this work with `atmos describe affected`?

**A:** Yes. `describe affected` iterates through registered component types instead of hardcoded types, making it automatically work with any registered component type including mock.

### Q: How do component-specific configuration schemas work?

**A:** Each component type defines its own configuration structure:

1. **Global config** (`atmos.yaml`): Each component type has a struct in `pkg/schema/schema.go` (Terraform, Helmfile, Packer, Mock)
2. **Stack config**: Component instances use flexible map[string]any structure - no schema enforcement at registry level
3. **Validation**: Each provider validates its own configuration via `ValidateComponent()` method
4. **Settings**: Component-specific settings (like `spacelift` for Terraform) are validated by the provider

Example: Terraform provider validates `backend`, `providers`, `settings.spacelift`, etc. Mock provider validates `settings.timeout`, `settings.retry_count`, etc.

### Q: Do cross-component dependencies work with the registry?

**A:** Yes! Dependencies work across ALL component types registered in the registry:

- **Syntax**: Use `metadata.depends_on` with `component` and `type` fields
- **Cross-type deps**: Helmfile can depend on Terraform, Mock can depend on Helmfile
- **DAG building**: Dependency graph uses `component.ListTypes()` to iterate all registered types
- **Affected analysis**: Changes propagate across component type boundaries
- **Deployment order**: DAG respects dependencies regardless of type

Example: `terraform/vpc → terraform/eks → helmfile/myapp → mock/test-runner` all works seamlessly.

### Q: Can I add custom fields to component configuration?

**A:** Yes! Component configuration is map-based and flexible:

```yaml
components:
  mock:
    my-component:
      vars:
        custom_field: "value"
      settings:
        my_custom_setting: 123
      # Any arbitrary fields
      custom_section:
        nested: data
```

The component provider decides what to validate and what to ignore. This matches how Terraform, Helmfile, and Packer work today.

## References

- [Command Registry Pattern PRD](command-registry-pattern.md)
- [Atmos Component Architecture Research](../../.conductor/bordeaux/component-architecture-research.md)
- [Go Plugin Documentation](https://pkg.go.dev/plugin)
- [Terraform Plugin Protocol](https://www.terraform.io/plugin)
- [kubectl Plugin System](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-10-16 | Initial PRD with mock component POC |
| 1.1 | 2025-10-16 | Added component configuration architecture section |
| | | Added cross-component dependencies (DAG) section |
| | | Added comprehensive test coverage requirements (90%+) |
| | | Added thread safety and edge case test examples |
| | | Updated FAQ with configuration and dependency questions |
| | | Clarified schema flexibility and validation approach |
| 2.0 | 2025-10-16 | **BREAKING**: Changed to hybrid configuration approach |
| | | Mock component uses `Components.Plugins` map instead of static struct |
| | | Added `GetComponentConfig()` helper method to Components |
| | | Documented migration path for built-in types (Phase 5) |
| | | Updated migration plan to reflect hybrid approach |
| | | Added Phase 5: Built-in Type Migration to plugin model |
| | | Clarified that Terraform/Helmfile/Packer will eventually migrate |
| | | Updated all code examples to use hybrid pattern |
