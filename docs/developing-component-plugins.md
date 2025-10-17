# Developing Component Plugins

This guide explains how to develop custom component types for Atmos using the component registry pattern.

## Overview

Atmos supports pluggable component types through the component registry pattern. While Atmos ships with built-in support for Terraform, Helmfile, and Packer, you can extend it to support any infrastructure tool by implementing the `ComponentProvider` interface.

**Key Concepts:**

- **Component Provider**: An implementation of the `ComponentProvider` interface that defines how Atmos interacts with a specific component type.
- **Component Registry**: A global registry that manages all component providers using thread-safe registration.
- **Plugin Configuration**: Component-specific settings stored in the `Components.Plugins` map in `atmos.yaml`.
- **Self-Registration**: Providers automatically register themselves via `init()` functions.

## When to Create a Component Plugin

Create a component plugin when you want to:

- Support a new infrastructure tool (Pulumi, Ansible, CDK, etc.)
- Create custom component types for your organization's workflows
- Extend Atmos with specialized deployment mechanisms
- Test component lifecycle integration (like the mock component)

## Component Provider Interface

All component providers must implement the `ComponentProvider` interface:

```go
// pkg/component/provider.go
package component

import (
    "context"
    "github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProvider defines the interface for component type implementations.
type ComponentProvider interface {
    // GetType returns the component type identifier (e.g., "terraform", "helmfile", "mock").
    GetType() string

    // GetGroup returns the category for organizing commands (e.g., "Infrastructure", "Testing").
    GetGroup() string

    // GetBasePath returns the filesystem path to components of this type.
    GetBasePath(atmosConfig *schema.AtmosConfiguration) string

    // ListComponents returns all components of this type in a stack.
    ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error)

    // ValidateComponent validates the configuration of a component.
    ValidateComponent(config map[string]any) error

    // Execute runs a command for a component.
    Execute(ctx ExecutionContext) error

    // GenerateArtifacts generates configuration files needed by the component.
    GenerateArtifacts(ctx ExecutionContext) error

    // GetAvailableCommands returns the list of commands this component type supports.
    GetAvailableCommands() []string
}

// ExecutionContext provides all information needed to execute a component command.
type ExecutionContext struct {
    AtmosConfig         *schema.AtmosConfiguration
    ComponentType       string
    Component           string
    Stack               string
    Command             string
    SubCommand          string
    ComponentConfig     map[string]any
    ConfigAndStacksInfo schema.ConfigAndStacksInfo
    Args                []string
    Flags               map[string]any
}
```

## Step-by-Step Guide

### 1. Create Package Structure

Create a new package for your component provider:

```
pkg/component/
├── yourtype/
│   ├── config.go      # Configuration structure
│   ├── yourtype.go    # Provider implementation
│   └── yourtype_test.go  # Comprehensive tests
├── provider.go        # Interface definition
└── registry.go        # Global registry
```

### 2. Define Configuration Structure

Create a configuration struct that defines component-specific settings:

```go
// pkg/component/yourtype/config.go
package yourtype

// Config represents the configuration structure for your component type.
// This configuration is stored in the Components.Plugins map in atmos.yaml.
type Config struct {
    // BasePath is the filesystem path to components of this type.
    BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`

    // Add your component-specific configuration fields here.
    // These can be overridden at global, stack, and component levels.

    // Example: ExecutablePath specifies the path to the tool binary.
    ExecutablePath string `yaml:"executable_path" json:"executable_path" mapstructure:"executable_path"`

    // Example: DefaultVersion specifies the default version to use.
    DefaultVersion string `yaml:"default_version" json:"default_version" mapstructure:"default_version"`

    // Example: Timeout for component operations in seconds.
    Timeout int `yaml:"timeout" json:"timeout" mapstructure:"timeout"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
    return Config{
        BasePath:       "components/yourtype",
        ExecutablePath: "/usr/local/bin/yourtool",
        DefaultVersion: "latest",
        Timeout:        300,
    }
}
```

### 3. Implement the Provider

Create your provider implementation:

```go
// pkg/component/yourtype/yourtype.go
package yourtype

import (
    "context"
    "fmt"

    "github.com/mitchellh/mapstructure"

    "github.com/cloudposse/atmos/pkg/component"
    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/schema"
)

// YourTypeProvider implements the ComponentProvider interface.
type YourTypeProvider struct{}

// Self-register on package import.
func init() {
    if err := component.Register(&YourTypeProvider{}); err != nil {
        panic(fmt.Sprintf("failed to register yourtype component provider: %v", err))
    }
}

// GetType returns the component type identifier.
func (p *YourTypeProvider) GetType() string {
    return "yourtype"
}

// GetGroup returns the category for organizing in CLI help.
func (p *YourTypeProvider) GetGroup() string {
    return "Infrastructure" // or "Testing", "Deployment", etc.
}

// GetBasePath returns the filesystem path to components.
func (p *YourTypeProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
    // Try to get config from Plugins map.
    rawConfig, ok := atmosConfig.Components.GetComponentConfig("yourtype")
    if !ok {
        return DefaultConfig().BasePath
    }

    config, err := parseConfig(rawConfig)
    if err != nil {
        return DefaultConfig().BasePath
    }

    if config.BasePath != "" {
        return config.BasePath
    }

    return DefaultConfig().BasePath
}

// ListComponents returns all components of this type in a stack.
func (p *YourTypeProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
    components := []string{}

    // Extract components section from stack config.
    componentsSection, ok := stackConfig["components"].(map[string]any)
    if !ok {
        return components, nil
    }

    // Extract this component type's section.
    typeSection, ok := componentsSection[p.GetType()].(map[string]any)
    if !ok {
        return components, nil
    }

    // List all component names.
    for componentName := range typeSection {
        components = append(components, componentName)
    }

    return components, nil
}

// ValidateComponent validates component configuration.
func (p *YourTypeProvider) ValidateComponent(config map[string]any) error {
    if config == nil {
        return nil
    }

    // Add your validation logic here.
    // Example: Check required fields, validate values, etc.

    return nil
}

// Execute runs a command for the component.
func (p *YourTypeProvider) Execute(ctx component.ExecutionContext) error {
    // Implement your execution logic here.
    // This is where you would:
    // 1. Parse the component configuration
    // 2. Prepare the execution environment
    // 3. Call your tool (e.g., via exec.Command or SDK)
    // 4. Handle output and errors

    return fmt.Errorf("%w: yourtype execution not yet implemented", errUtils.ErrComponentExecutionFailed)
}

// GenerateArtifacts generates configuration files needed by the component.
func (p *YourTypeProvider) GenerateArtifacts(ctx component.ExecutionContext) error {
    // Generate any files your tool needs:
    // - Variable files
    // - Configuration files
    // - Environment files
    // - etc.

    return nil
}

// GetAvailableCommands returns the commands this component type supports.
func (p *YourTypeProvider) GetAvailableCommands() []string {
    return []string{
        "plan",
        "apply",
        "destroy",
        "validate",
        // Add your component-specific commands
    }
}

// parseConfig converts raw config map to typed Config struct.
func parseConfig(raw any) (Config, error) {
    var config Config

    decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
        Result:           &config,
        WeaklyTypedInput: true,
        TagName:          "mapstructure",
    })
    if err != nil {
        return Config{}, fmt.Errorf("%w: failed to create config decoder: %v", errUtils.ErrComponentConfigInvalid, err)
    }

    if err := decoder.Decode(raw); err != nil {
        return Config{}, fmt.Errorf("%w: failed to decode config: %v", errUtils.ErrComponentConfigInvalid, err)
    }

    return config, nil
}
```

### 4. Write Comprehensive Tests

Create tests with >90% coverage:

```go
// pkg/component/yourtype/yourtype_test.go
package yourtype

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/cloudposse/atmos/pkg/component"
    "github.com/cloudposse/atmos/pkg/schema"
)

func TestYourTypeProvider_GetType(t *testing.T) {
    provider := &YourTypeProvider{}
    assert.Equal(t, "yourtype", provider.GetType())
}

func TestYourTypeProvider_GetGroup(t *testing.T) {
    provider := &YourTypeProvider{}
    assert.Equal(t, "Infrastructure", provider.GetGroup())
}

func TestYourTypeProvider_GetBasePath(t *testing.T) {
    provider := &YourTypeProvider{}

    tests := []struct {
        name         string
        atmosConfig  *schema.AtmosConfiguration
        expectedPath string
    }{
        {
            name: "with configured base_path",
            atmosConfig: &schema.AtmosConfiguration{
                Components: schema.Components{
                    Plugins: map[string]any{
                        "yourtype": map[string]any{
                            "base_path": "custom/yourtype/path",
                        },
                    },
                },
            },
            expectedPath: "custom/yourtype/path",
        },
        {
            name: "with empty Plugins map",
            atmosConfig: &schema.AtmosConfiguration{
                Components: schema.Components{
                    Plugins: map[string]any{},
                },
            },
            expectedPath: "components/yourtype",
        },
        {
            name: "with nil Plugins",
            atmosConfig: &schema.AtmosConfiguration{
                Components: schema.Components{
                    Plugins: nil,
                },
            },
            expectedPath: "components/yourtype",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            path := provider.GetBasePath(tt.atmosConfig)
            assert.Equal(t, tt.expectedPath, path)
        })
    }
}

func TestYourTypeProvider_ListComponents(t *testing.T) {
    provider := &YourTypeProvider{}

    tests := []struct {
        name          string
        stackConfig   map[string]any
        expectedComps []string
        expectedErr   bool
    }{
        {
            name: "with components",
            stackConfig: map[string]any{
                "components": map[string]any{
                    "yourtype": map[string]any{
                        "component1": map[string]any{},
                        "component2": map[string]any{},
                    },
                },
            },
            expectedComps: []string{"component1", "component2"},
            expectedErr:   false,
        },
        {
            name: "without yourtype section",
            stackConfig: map[string]any{
                "components": map[string]any{
                    "terraform": map[string]any{},
                },
            },
            expectedComps: []string{},
            expectedErr:   false,
        },
        {
            name:          "with nil stack config",
            stackConfig:   nil,
            expectedComps: []string{},
            expectedErr:   false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            components, err := provider.ListComponents(context.Background(), "test-stack", tt.stackConfig)

            if tt.expectedErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.ElementsMatch(t, tt.expectedComps, components)
            }
        })
    }
}

func TestYourTypeProvider_ValidateComponent(t *testing.T) {
    provider := &YourTypeProvider{}

    tests := []struct {
        name    string
        config  map[string]any
        wantErr bool
    }{
        {
            name:    "nil config",
            config:  nil,
            wantErr: false,
        },
        {
            name:    "empty config",
            config:  map[string]any{},
            wantErr: false,
        },
        {
            name: "valid config",
            config: map[string]any{
                "vars": map[string]any{
                    "example": "value",
                },
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

func TestDefaultConfig(t *testing.T) {
    config := DefaultConfig()

    assert.Equal(t, "components/yourtype", config.BasePath)
    assert.Equal(t, "/usr/local/bin/yourtool", config.ExecutablePath)
    assert.Equal(t, "latest", config.DefaultVersion)
    assert.Equal(t, 300, config.Timeout)
}

func TestParseConfig(t *testing.T) {
    tests := []struct {
        name        string
        raw         any
        expected    Config
        expectError bool
    }{
        {
            name: "valid config map",
            raw: map[string]any{
                "base_path":       "custom/path",
                "executable_path": "/custom/bin/tool",
                "default_version": "1.0.0",
                "timeout":         600,
            },
            expected: Config{
                BasePath:       "custom/path",
                ExecutablePath: "/custom/bin/tool",
                DefaultVersion: "1.0.0",
                Timeout:        600,
            },
            expectError: false,
        },
        {
            name: "partial config",
            raw: map[string]any{
                "base_path": "custom/path",
            },
            expected: Config{
                BasePath: "custom/path",
            },
            expectError: false,
        },
        {
            name:        "empty map",
            raw:         map[string]any{},
            expected:    Config{},
            expectError: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            config, err := parseConfig(tt.raw)

            if tt.expectError {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected.BasePath, config.BasePath)
                assert.Equal(t, tt.expected.ExecutablePath, config.ExecutablePath)
                assert.Equal(t, tt.expected.DefaultVersion, config.DefaultVersion)
                assert.Equal(t, tt.expected.Timeout, config.Timeout)
            }
        })
    }
}
```

### 5. Register with Root Command (Optional)

If you want your component type to be available globally, import it in `cmd/root.go`:

```go
import (
    _ "github.com/cloudposse/atmos/pkg/component/yourtype"
)
```

The blank import triggers the `init()` function which registers the provider.

## Configuration

### Global Configuration (atmos.yaml)

Add your component type configuration to `atmos.yaml`:

```yaml
components:
  # Built-in types
  terraform:
    base_path: "components/terraform"

  helmfile:
    base_path: "components/helmfile"

  packer:
    base_path: "components/packer"

  # Your plugin type
  yourtype:
    base_path: "components/yourtype"
    executable_path: "/usr/local/bin/yourtool"
    default_version: "1.2.3"
    timeout: 600
```

### Stack Configuration

Components of your type are defined in stack manifests:

```yaml
# stacks/dev.yaml
components:
  yourtype:
    my-component:
      metadata:
        component: "base-component"
        inherits:
          - "common-config"
      vars:
        example_var: "value"
      settings:
        depends_on:
          vpc:
            component: "vpc"
            namespace: "platform"
      # Your component-specific configuration
```

## Configuration Inheritance

Atmos supports three levels of configuration that merge hierarchically:

1. **Global Level** (`atmos.yaml`): Default settings for all components of this type
2. **Stack Level** (stack manifest files): Stack-specific overrides
3. **Component Level** (within stack manifests): Component-specific overrides

Example showing inheritance:

```yaml
# atmos.yaml (global)
components:
  yourtype:
    base_path: "components/yourtype"
    timeout: 300
    tags:
      - global

# stacks/globals.yaml (stack level)
yourtype:
  timeout: 600
  tags:
    - stack-level

# stacks/dev.yaml (component level)
components:
  yourtype:
    my-component:
      vars:
        # Inherits timeout: 600 from stack level
        # Inherits tags: [global, stack-level] (merged)
        custom_setting: "value"
```

## Error Handling

Always use sentinel errors from `errors/errors.go`:

```go
import errUtils "github.com/cloudposse/atmos/errors"

// Wrap errors with sentinel errors
if err != nil {
    return fmt.Errorf("%w: failed to execute component: %v", errUtils.ErrComponentExecutionFailed, err)
}

// Validation errors
if invalid {
    return fmt.Errorf("%w: missing required field 'example'", errUtils.ErrComponentValidationFailed)
}

// Configuration errors
if parseErr != nil {
    return fmt.Errorf("%w: %v", errUtils.ErrComponentConfigInvalid, parseErr)
}
```

## Best Practices

### 1. Thread Safety

The registry is thread-safe, but your provider implementation should also be thread-safe if it maintains state.

### 2. Configuration Parsing

Use `mapstructure` for flexible configuration parsing:

```go
import "github.com/mitchellh/mapstructure"

decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
    Result:           &config,
    WeaklyTypedInput: true,
    TagName:          "mapstructure",
})
```

### 3. Defaults

Always provide sensible defaults via `DefaultConfig()`:

```go
func DefaultConfig() Config {
    return Config{
        BasePath:       "components/yourtype",
        ExecutablePath: "/usr/local/bin/yourtool",
        Timeout:        300,
    }
}
```

### 4. Error Messages

Provide clear, actionable error messages:

```go
// GOOD
return fmt.Errorf("%w: component 'vpc' not found in stack 'dev-us-east-1'", errUtils.ErrComponentValidationFailed)

// BAD
return fmt.Errorf("not found")
```

### 5. Test Coverage

Aim for >90% test coverage:
- Unit tests for all interface methods
- Table-driven tests for configuration parsing
- Integration tests for complete workflows
- Error case testing

### 6. Documentation

Document your provider:
- Package-level documentation
- Config struct field documentation
- Public method documentation
- Usage examples

## Integration with Atmos Commands

Your component provider automatically integrates with Atmos commands:

### `atmos list components`

Lists all components of your type across all stacks.

### `atmos describe component <name> -s <stack>`

Shows the complete configuration for a component, including inherited values.

### `atmos describe affected`

Detects changes to your component type and includes them in affected component analysis.

### `atmos describe stacks`

Includes your component type in stack descriptions.

## Example: Mock Component

See `pkg/component/mock/` for a complete working example. The mock component demonstrates:

- Configuration inheritance (enabled, dry_run, tags, metadata, dependencies)
- Array merging (tags)
- Deep object merging (metadata)
- Cross-component dependencies (dependencies array)
- Complete test coverage (>95%)

## Testing Your Plugin

Run tests with coverage:

```bash
# Unit tests
go test ./pkg/component/yourtype/ -v

# With coverage
go test ./pkg/component/yourtype/ -cover

# Generate coverage report
go test ./pkg/component/yourtype/ -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Troubleshooting

### Provider Not Registered

**Symptom**: Component type not found in `atmos list components`

**Solution**: Ensure your package is imported (even with blank import `_`) in a package that's loaded by Atmos, typically `cmd/root.go`.

### Configuration Not Loading

**Symptom**: Default configuration always used, custom config ignored

**Solution**:
1. Verify your config is in the correct location in `atmos.yaml` under `components.yourtype`
2. Check mapstructure tags match YAML field names (use `yaml:"field_name"`)
3. Ensure `GetComponentConfig()` is called correctly

### Inheritance Not Working

**Symptom**: Component-level config doesn't override stack config

**Solution**: Atmos's stack processing handles inheritance automatically. Ensure you're getting the final merged config from `ExecutionContext.ComponentConfig`, not parsing raw YAML directly.

## Advanced Topics

### Custom Dependency Resolution

Implement custom dependency graph logic for complex multi-component workflows.

### Artifact Generation

Generate tool-specific configuration files (like Terraform `.tfvars` or Helmfile values):

```go
func (p *YourTypeProvider) GenerateArtifacts(ctx component.ExecutionContext) error {
    // Generate variables file
    varsFile := filepath.Join(ctx.ComponentConfig["base_path"].(string), "vars.auto.json")

    varsData, err := json.MarshalIndent(ctx.ComponentConfig["vars"], "", "  ")
    if err != nil {
        return fmt.Errorf("%w: %v", errUtils.ErrComponentArtifactGeneration, err)
    }

    if err := os.WriteFile(varsFile, varsData, 0644); err != nil {
        return fmt.Errorf("%w: %v", errUtils.ErrComponentArtifactGeneration, err)
    }

    return nil
}
```

### SDK vs CLI Execution

Prefer using Go SDKs over shelling out to CLI tools when possible:

```go
// GOOD: Using SDK
import "github.com/hashicorp/terraform-exec/tfexec"

tf, err := tfexec.NewTerraform(workingDir, execPath)
if err != nil {
    return err
}
err = tf.Plan(context.Background())

// ACCEPTABLE: CLI when no SDK exists
cmd := exec.Command("yourtool", "plan")
cmd.Dir = workingDir
output, err := cmd.CombinedOutput()
```

## Migration Path

The component registry pattern is designed to eventually migrate all component types to plugins:

**Phase 1-3 (Current)**: Built-in types (terraform, helmfile, packer) remain static
**Phase 4**: New types use plugin pattern
**Phase 5**: Migrate built-in types to plugin implementations
**Phase 6**: Unified plugin architecture

Your plugin will be compatible with this migration path as long as it implements the `ComponentProvider` interface.

## Getting Help

- Review the mock component implementation: `pkg/component/mock/`
- Check the component registry pattern PRD: `docs/prd/component-registry-pattern.md`
- See interface definition: `pkg/component/provider.go`
- Look at existing built-in types for patterns (though they don't use the registry yet)

## Summary

Creating a component plugin involves:

1. ✅ Define your Config struct with component-specific settings
2. ✅ Implement the ComponentProvider interface (8 methods)
3. ✅ Self-register via init() function
4. ✅ Write comprehensive tests (>90% coverage)
5. ✅ Document your configuration structure
6. ✅ Use sentinel errors for error handling
7. ✅ Import your package to make it available to Atmos

The component registry pattern provides a clean, extensible way to add new infrastructure tools to Atmos while maintaining consistency with existing component types.
