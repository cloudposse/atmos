# Tool Dependencies Integration

**Status**: 🚧 In Progress
**Last Updated**: 2025-10-23

## Overview

Integrate tool dependencies into Atmos workflows, custom commands, and components, enabling automatic tool installation and version management based on declarative configuration.

## Problem Statement

Currently, the toolchain package provides tool installation and execution capabilities, but:

1. **No Automatic Installation**: Users must manually install tools before running commands
2. **No Dependency Declaration**: Cannot declare tool requirements at component, stack, workflow, or command level
3. **No Version Enforcement**: No way to ensure specific tool versions are used for specific contexts
4. **Manual PATH Management**: Users must manage PATH environment variables themselves

## Goals

1. **Declarative Dependencies**: Allow tool dependencies to be declared at multiple levels (stack, component, workflow, command)
2. **Automatic Installation**: Auto-install missing tools before execution
3. **Version Constraints**: Support SemVer constraints with validation
4. **Inheritance**: Tool dependencies inherit through stack imports with deep merge
5. **Seamless Integration**: No changes to existing user workflows - just works

## Non-Goals

- Dependency conflict resolution (install all required versions)
- Tool upgrade management (use declared versions)
- Custom registries (Aqua registry only in v1)

## Architecture

### Configuration Levels

Tool dependencies can be declared at four levels (highest to lowest priority):

1. **Component Instance** - Specific component in a stack file
2. **Component Catalog** - Component definition in catalog
3. **Stack Instance** - Top-level in stack file
4. **Stack Catalog** - Top-level in catalog file

Additionally:

5. **Workflow** - Tools required for workflow execution
6. **Custom Command** - Tools required for command execution

### Schema Structure

```go
// Dependencies declares required tools and their versions.
type Dependencies struct {
    Tools map[string]string `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
}
```

**Usage in YAML:**

```yaml
# Stack-level (applies to all components)
dependencies:
  tools:
    terraform: "~> 1.10.0"  # SemVer constraint
    tflint: "^0.54.0"       # Caret constraint
    aws-cli: "latest"       # Always latest

components:
  terraform:
    vpc:
      # Component-level (overrides stack-level for this component)
      dependencies:
        tools:
          terraform: "1.10.3"  # Specific version (must satisfy stack constraint)
          checkov: "latest"     # Component-specific tool
```

### Resolution Algorithm

```
For component execution (terraform/helmfile/packer):
  1. Load stack configuration with imports
  2. Collect dependencies from all levels:
     - Stack catalog (from imports)
     - Stack instance (top-level dependencies)
     - Component catalog (from imports)
     - Component instance (component.dependencies)
  3. Deep merge with override (child overrides parent)
  4. Validate constraints (child must satisfy parent)
  5. Return merged dependency map

For workflow execution:
  1. Load workflow definition
  2. Collect workflow.dependencies.tools
  3. Return workflow dependency map

For custom command execution:
  1. Load command definition
  2. Collect command.dependencies.tools
  3. Return command dependency map
```

### Auto-Install Hook

Before executing any command, check tool dependencies and auto-install if missing:

```
Before execution:
  1. Resolve dependencies for current context
  2. For each tool in dependencies:
     a. Parse tool@version specifier
     b. Check if version installed (.tools/bin/owner/repo/version/)
     c. If missing: toolchain.InstallExec(tool@version)
  3. Update PATH to include .tools/bin with correct versions
  4. Execute command
```

## Implementation Plan

### Phase 1: Schema Updates

#### 1.1 Add Dependencies Struct

**File**: `pkg/schema/dependencies.go` (new)

```go
package schema

// Dependencies declares required tools and their versions.
type Dependencies struct {
	Tools map[string]string `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
}
```

#### 1.2 Update Existing Schemas

**File**: `pkg/schema/workflow.go`

```go
type WorkflowDefinition struct {
	Description  string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Dependencies *Dependencies  `yaml:"dependencies,omitempty" json:"dependencies,omitempty" mapstructure:"dependencies"`
	Steps        []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
	Stack        string         `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
}
```

**File**: `pkg/schema/command.go`

```go
type Command struct {
	Name            string                 `yaml:"name" json:"name" mapstructure:"name"`
	Description     string                 `yaml:"description" json:"description" mapstructure:"description"`
	Dependencies    *Dependencies          `yaml:"dependencies,omitempty" json:"dependencies,omitempty" mapstructure:"dependencies"`
	Env             []CommandEnv           `yaml:"env" json:"env" mapstructure:"env"`
	// ... existing fields
}
```

**Stack YAML** (no schema change needed - uses `map[string]any`):
- Top-level `dependencies.tools`
- Component-level `components.terraform.vpc.dependencies.tools`

### Phase 2: Dependency Resolution

#### 2.1 Dependency Resolver Package

**File**: `pkg/dependencies/resolver.go` (new)

```go
package dependencies

import (
	"fmt"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Resolver resolves tool dependencies with inheritance and validation.
type Resolver struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewResolver creates a new dependency resolver.
func NewResolver(atmosConfig *schema.AtmosConfiguration) *Resolver

// ResolveComponentDependencies resolves tool dependencies for a component.
// Merges: stack catalog → stack instance → component catalog → component instance
func (r *Resolver) ResolveComponentDependencies(
	componentType string,
	component string,
	stack string,
) (map[string]string, error)

// ResolveWorkflowDependencies resolves tool dependencies for a workflow.
func (r *Resolver) ResolveWorkflowDependencies(
	workflow string,
) (map[string]string, error)

// ResolveCommandDependencies resolves tool dependencies for a custom command.
func (r *Resolver) ResolveCommandDependencies(
	command string,
) (map[string]string, error)

// mergeDependencies merges child dependencies into parent with validation.
func mergeDependencies(
	parent map[string]string,
	child map[string]string,
) (map[string]string, error)

// validateConstraint validates that specific version satisfies constraint.
func validateConstraint(version string, constraint string) error
```

#### 2.2 SemVer Constraint Validation

Use existing `github.com/Masterminds/semver/v3` dependency:

```go
import "github.com/Masterminds/semver/v3"

func validateConstraint(version string, constraint string) error {
	// "latest" always satisfies
	if constraint == "latest" || version == "latest" {
		return nil
	}

	// Parse constraint (e.g., "~> 1.10.0", "^0.54.0")
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return fmt.Errorf("invalid constraint %q: %w", constraint, err)
	}

	// Parse version
	v, err := semver.NewVersion(version)
	if err != nil {
		return fmt.Errorf("invalid version %q: %w", version, err)
	}

	// Validate
	if !c.Check(v) {
		return fmt.Errorf("version %q does not satisfy constraint %q", version, constraint)
	}

	return nil
}
```

### Phase 3: Auto-Install Integration

#### 3.1 Tool Installer Package

**File**: `pkg/dependencies/installer.go` (new)

```go
package dependencies

import (
	"github.com/cloudposse/atmos/toolchain"
)

// Installer handles automatic tool installation.
type Installer struct{}

// NewInstaller creates a new tool installer.
func NewInstaller() *Installer

// EnsureTools ensures all required tools are installed.
// Installs missing tools automatically.
func (i *Installer) EnsureTools(dependencies map[string]string) error {
	for tool, version := range dependencies {
		if err := i.ensureTool(tool, version); err != nil {
			return err
		}
	}
	return nil
}

// ensureTool ensures a specific tool version is installed.
func (i *Installer) ensureTool(tool string, version string) error {
	// Check if already installed
	binaryPath, err := toolchain.FindToolBinary(tool, version)
	if err == nil && binaryPath != "" {
		return nil // Already installed
	}

	// Install missing tool
	toolSpec := fmt.Sprintf("%s@%s", tool, version)
	return toolchain.InstallExec(toolSpec)
}
```

#### 3.2 Component Execution Hook

**File**: `internal/exec/terraform_component_executor.go` (modify existing)

```go
func ExecuteTerraformComponent(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	// ... other params
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformComponent")()

	// Resolve tool dependencies
	resolver := dependencies.NewResolver(atmosConfig)
	deps, err := resolver.ResolveComponentDependencies("terraform", component, stack)
	if err != nil {
		return fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	// Auto-install missing tools
	installer := dependencies.NewInstaller()
	if err := installer.EnsureTools(deps); err != nil {
		return fmt.Errorf("failed to ensure tools: %w", err)
	}

	// Update PATH to include installed tools
	if err := updatePathForTools(deps); err != nil {
		return fmt.Errorf("failed to update PATH: %w", err)
	}

	// Continue with existing execution logic
	// ...
}
```

#### 3.3 Workflow Execution Hook

**File**: `internal/exec/workflow.go` (modify existing)

```go
func ExecuteWorkflow(
	atmosConfig *schema.AtmosConfiguration,
	workflow string,
	// ... other params
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteWorkflow")()

	// Resolve workflow dependencies
	resolver := dependencies.NewResolver(atmosConfig)
	deps, err := resolver.ResolveWorkflowDependencies(workflow)
	if err != nil {
		return fmt.Errorf("failed to resolve workflow dependencies: %w", err)
	}

	// Auto-install missing tools
	installer := dependencies.NewInstaller()
	if err := installer.EnsureTools(deps); err != nil {
		return fmt.Errorf("failed to ensure tools: %w", err)
	}

	// Continue with existing workflow execution
	// ...
}
```

#### 3.4 Custom Command Hook

**File**: `internal/exec/custom_command.go` (modify existing)

Similar pattern to workflow execution.

### Phase 4: PATH Management

**File**: `pkg/dependencies/path.go` (new)

```go
package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// updatePathForTools updates PATH to include tool binaries.
func updatePathForTools(dependencies map[string]string) error {
	toolsDir := os.Getenv("ATMOS_TOOLCHAIN_TOOLS_DIR")
	if toolsDir == "" {
		toolsDir = ".tools"
	}

	var paths []string
	for tool, version := range dependencies {
		// Resolve tool to owner/repo
		owner, repo, err := resolveToolPath(tool)
		if err != nil {
			return err
		}

		// Add versioned bin directory to PATH
		binPath := filepath.Join(toolsDir, "bin", owner, repo, version)
		paths = append(paths, binPath)
	}

	// Prepend to existing PATH
	currentPath := os.Getenv("PATH")
	newPath := strings.Join(append(paths, currentPath), string(os.PathListSeparator))
	os.Setenv("PATH", newPath)

	return nil
}
```

## Testing Strategy

### Unit Tests

1. **Dependency Resolution**
   - Test stack-level inheritance
   - Test component-level inheritance
   - Test deep merge behavior
   - Test constraint validation

2. **SemVer Constraint Validation**
   - Test tilde constraints (~> 1.10.0)
   - Test caret constraints (^0.54.0)
   - Test exact versions
   - Test "latest" handling

3. **Auto-Install Logic**
   - Test tool already installed (skip)
   - Test tool missing (install)
   - Test installation failure handling

### Integration Tests

1. **Component Execution**
   - Component with dependencies → auto-install → execute
   - Component with invalid constraint → error

2. **Workflow Execution**
   - Workflow with dependencies → auto-install → execute

3. **Custom Command Execution**
   - Command with dependencies → auto-install → execute

## Migration Path

### Backward Compatibility

- **No breaking changes**: Tool dependencies are optional
- **Existing workflows continue working**: No dependencies = no auto-install
- **Opt-in adoption**: Users add dependencies when ready

### Migration Guide

**Before** (manual installation):
```bash
atmos toolchain install terraform@1.10.3
atmos terraform plan vpc -s prod
```

**After** (automatic installation):
```yaml
# stacks/prod.yaml
dependencies:
  tools:
    terraform: "1.10.3"

components:
  terraform:
    vpc:
      # ...
```

```bash
atmos terraform plan vpc -s prod  # Auto-installs terraform@1.10.3
```

## Configuration Examples

### Stack-Level Dependencies

```yaml
# stacks/catalog/base.yaml
dependencies:
  tools:
    terraform: "~> 1.10.0"
    tflint: "^0.54.0"
    tfsec: "latest"

components:
  terraform:
    vpc:
      vars:
        name: vpc
```

### Component-Level Dependencies

```yaml
# stacks/catalog/terraform/database.yaml
components:
  terraform:
    rds:
      dependencies:
        tools:
          terraform: "~> 1.9.0"  # Older version for legacy DB
          checkov: "^3.0.0"
      vars:
        engine: postgres
```

### Workflow Dependencies

```yaml
# stacks/workflows/deploy.yaml
workflows:
  deploy-infra:
    description: Deploy infrastructure
    dependencies:
      tools:
        terraform: "~> 1.10.0"
        aws-cli: "^2.0.0"
        jq: "latest"
    steps:
      - name: plan
        command: terraform plan vpc -s prod
      - name: apply
        command: terraform apply vpc -s prod
```

### Custom Command Dependencies

```yaml
# atmos.yaml
commands:
  - name: deploy
    description: Deploy with required tools
    dependencies:
      tools:
        terraform: "~> 1.10.0"
        kubectl: "^1.32.0"
    steps:
      - atmos terraform plan vpc -s {{ .stack }}
      - atmos terraform apply vpc -s {{ .stack }}
```

## Error Handling

### Constraint Validation Errors

```
Error: Tool dependency constraint validation failed
  Component: vpc
  Stack: prod
  Tool: terraform
  Parent constraint: ~> 1.10.0
  Child version: 1.9.8

  The version "1.9.8" does not satisfy parent constraint "~> 1.10.0"
```

### Installation Errors

```
Error: Failed to install required tool
  Tool: terraform@1.10.3
  Reason: Failed to download from GitHub: rate limit exceeded

  Suggestion: Set ATMOS_GITHUB_TOKEN environment variable
```

## Performance Considerations

1. **Tool Check Caching**: Cache "already installed" checks per execution
2. **Parallel Installation**: Install multiple tools concurrently
3. **Registry Caching**: Reuse existing toolchain registry cache

## Security Considerations

1. **Version Pinning**: Encourage specific versions over "latest"
2. **Constraint Validation**: Prevent downgrade attacks via constraints
3. **GitHub Token**: Support authenticated downloads for rate limits

## Metrics and Observability

Track via performance monitoring:
- `dependencies.resolve.component` - Dependency resolution time
- `dependencies.resolve.workflow` - Workflow dependency resolution
- `dependencies.install` - Tool installation time
- `dependencies.validate` - Constraint validation time

## Future Enhancements

1. **Lock Files**: Generate `.tool-versions.lock` for reproducible builds
2. **Custom Registries**: Support private/custom tool registries
3. **Dependency Caching**: Shared cache across projects
4. **Conflict Resolution**: Smart handling of conflicting constraints
5. **Tool Updates**: `atmos toolchain upgrade` command

## References

- **Toolchain PRD**: `docs/prd/toolchain-implementation.md`
- **SemVer Spec**: https://semver.org/
- **Aqua Registry**: https://github.com/aquaproj/aqua-registry

## Open Questions

1. Should we support multiple versions of the same tool in one execution context? (No - use constraints)
2. How to handle PATH priority when multiple components need different versions? (Per-component PATH setup)
3. Should we validate constraints at config load time or execution time? (Execution time - lazy validation)

## Success Criteria

1. ✅ Users can declare tool dependencies at any level (stack/component/workflow/command)
2. ✅ Tools are automatically installed before execution
3. ✅ SemVer constraints are validated and enforced
4. ✅ Dependencies inherit through stack imports
5. ✅ Zero breaking changes to existing workflows
6. ✅ Test coverage ≥ 80% for new code
