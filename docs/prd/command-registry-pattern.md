# Command Registry Pattern

## Overview

This document describes the command registry pattern for Atmos, which provides a modular, self-registering architecture for built-in commands while maintaining full compatibility with custom commands and command aliases defined in `atmos.yaml`.

## Problem Statement

### Current State

Atmos has 116+ Go files in a flat `cmd/` directory structure:

```
cmd/
├── about.go
├── atlantis.go
├── atlantis_generate.go
├── atlantis_generate_repo_config.go
├── aws.go
├── aws_eks.go
├── aws_eks_update_kubeconfig.go
├── describe.go
├── describe_affected.go
├── describe_component.go
├── describe_config.go
├── describe_dependents.go
├── describe_stacks.go
├── describe_workflows.go
├── list.go
├── list_components.go
├── list_stacks.go
├── terraform.go
├── terraform_commands.go
├── terraform_generate.go
├── terraform_generate_backend.go
... (100+ more files)
```

### Challenges

1. **Difficult navigation** - Hard to find related command files
2. **No clear organization** - Commands and subcommands mixed together
3. **Scalability issues** - Adding more commands increases complexity
4. **Plugin readiness** - No foundation for external plugins
5. **Inconsistent patterns** - No standard way to organize command families

### Custom Commands Consideration

Atmos supports **user-defined custom commands** in `atmos.yaml`:

```yaml
commands:
  - name: hello
    description: This command says Hello world
    steps:
      - "echo Hello world!"

  - name: terraform
    description: Custom terraform wrapper
    commands:
      - name: provision
        description: Provision infrastructure
        steps:
          - atmos terraform plan {{ .arguments.component }} -s {{ .arguments.stack }}
          - atmos terraform apply {{ .arguments.component }} -s {{ .arguments.stack }}
```

**Critical requirement:** The registry pattern must coexist with custom commands without conflicts or breaking changes.

## Solution: Command Registry Pattern

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                       Atmos CLI                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────┐             │
│  │         Command Registry                   │             │
│  │  (cmd/internal/registry.go)                │             │
│  └────────────────────────────────────────────┘             │
│           │                        │                         │
│           │                        │                         │
│           ▼                        ▼                         │
│  ┌─────────────────┐    ┌──────────────────────┐           │
│  │  Built-in       │    │  Custom Commands     │           │
│  │  Commands       │    │  (from atmos.yaml)   │           │
│  │                 │    │                      │           │
│  │  - terraform    │    │  - Dynamically       │           │
│  │  - helmfile     │    │    generated from    │           │
│  │  - describe     │    │    config            │           │
│  │  - list         │    │                      │           │
│  │  - validate     │    │  - Can extend or     │           │
│  │  - vendor       │    │    override built-in │           │
│  │  - about        │    │    commands          │           │
│  └─────────────────┘    └──────────────────────┘           │
│                                                              │
│  Execution Order:                                            │
│  1. Load built-in commands via registry                     │
│  2. Load custom commands from atmos.yaml                     │
│  3. Custom commands can override built-in                    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Coexistence** - Built-in and custom commands work together
2. **Override capability** - Custom commands can override built-in commands
3. **Self-registration** - Commands register themselves via `init()`
4. **Modular organization** - Command families in separate packages
5. **Zero breaking changes** - Existing functionality preserved
6. **Plugin readiness** - Foundation for future external plugins

## Implementation

### 1. CommandProvider Interface

```go
// cmd/internal/command.go
package internal

import "github.com/spf13/cobra"

// CommandProvider is the interface that built-in command packages implement
// to register themselves with the Atmos command registry.
type CommandProvider interface {
    // GetCommand returns the cobra.Command for this provider.
    GetCommand() *cobra.Command

    // GetName returns the command name (e.g., "about", "terraform").
    GetName() string

    // GetGroup returns the command group for help organization.
    // Examples: "Core Stack Commands", "Stack Introspection",
    //          "Configuration Management", "Cloud Integration"
    GetGroup() string
}
```

### 2. Command Registry

```go
// cmd/internal/registry.go
package internal

import (
    "fmt"
    "sync"

    "github.com/spf13/cobra"
)

var (
    // Global registry instance.
    registry = &CommandRegistry{
        providers: make(map[string]CommandProvider),
    }
)

// CommandRegistry manages built-in command registration.
// Note: This registry is for BUILT-IN commands only.
// Custom commands from atmos.yaml are processed separately.
type CommandRegistry struct {
    mu        sync.RWMutex
    providers map[string]CommandProvider
}

// Register adds a built-in command provider to the registry.
// This is called during package init() for built-in commands.
func Register(provider CommandProvider) {
    registry.mu.Lock()
    defer registry.mu.Unlock()

    name := provider.GetName()
    if _, exists := registry.providers[name]; exists {
        // Allow re-registration for testing and plugin override
        // Custom commands will be processed after built-in commands
        // and can override via processCustomCommands()
    }

    registry.providers[name] = provider
}

// RegisterAll registers all built-in commands with the root command.
// Custom commands are processed separately via processCustomCommands().
func RegisterAll(root *cobra.Command) error {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    for name, provider := range registry.providers {
        cmd := provider.GetCommand()
        if cmd == nil {
            return fmt.Errorf("command provider %s returned nil command", name)
        }
        root.AddCommand(cmd)
    }

    return nil
}

// GetProvider returns a built-in command provider by name.
func GetProvider(name string) (CommandProvider, bool) {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    provider, ok := registry.providers[name]
    return provider, ok
}

// ListProviders returns all registered providers grouped by category.
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

// Count returns the number of registered built-in providers.
func Count() int {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    return len(registry.providers)
}

// Reset clears the registry (for testing only).
func Reset() {
    registry.mu.Lock()
    defer registry.mu.Unlock()

    registry.providers = make(map[string]CommandProvider)
}
```

### 3. Example: About Command

```go
// cmd/about/about.go
package about

import (
    "github.com/spf13/cobra"

    "github.com/cloudposse/atmos/cmd/internal"
    e "github.com/cloudposse/atmos/internal/exec"
)

// aboutCmd represents the about command.
var aboutCmd = &cobra.Command{
    Use:   "about",
    Short: "Show information about Atmos",
    Long:  `Display information about the Atmos CLI, including version, license, and project details.`,
    RunE: func(cmd *cobra.Command, args []string) error {
        return e.ExecuteAboutCmd(cmd, args)
    },
}

func init() {
    // Register this built-in command with the registry.
    // This happens during package initialization via blank import.
    internal.Register(&AboutCommandProvider{})
}

// AboutCommandProvider implements the CommandProvider interface.
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

### 4. Markdown Content Management

All markdown content is centralized in `cmd/markdown/` and accessed through the `cmd/markdown` package.

**Why This Pattern?**

Go's `//go:embed` directive cannot reference parent directories using `..` when used from subpackages. To maintain a single source of truth for markdown content while supporting the package-per-command structure, we:

1. Store all markdown files in `cmd/markdown/`
2. Export them via `cmd/markdown/content.go`
3. Import them in command packages

**Example:**

```go
// cmd/markdown/content.go
package markdown

import _ "embed"

// AboutMarkdown contains the content for the about command.
//
//go:embed about.md
var AboutMarkdown string
```

```go
// cmd/about/about.go
package about

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/markdown"
    "github.com/cloudposse/atmos/pkg/utils"
)

var aboutCmd = &cobra.Command{
    Use:   "about",
    Short: "Learn about Atmos",
    RunE: func(cmd *cobra.Command, args []string) error {
        utils.PrintfMarkdown("%s", markdown.AboutMarkdown)
        return nil
    },
}
```

**Benefits:**

- ✅ Single source of truth for all markdown content
- ✅ No duplicate files across command packages
- ✅ Compatible with Go's embed restrictions
- ✅ Easy to maintain and update content

### 5. Updated Root Command

```go
// cmd/root.go
package cmd

import (
    // ... existing imports ...

    "github.com/cloudposse/atmos/cmd/internal"

    // Import built-in command packages for side-effect registration.
    // The init() function in each package registers the command.
    // TODO: Gradually migrate all commands to this pattern
    _ "github.com/cloudposse/atmos/cmd/about"

    // Future imports as commands are migrated:
    // _ "github.com/cloudposse/atmos/cmd/terraform"
    // _ "github.com/cloudposse/atmos/cmd/helmfile"
    // _ "github.com/cloudposse/atmos/cmd/describe"
    // _ "github.com/cloudposse/atmos/cmd/list"
    // _ "github.com/cloudposse/atmos/cmd/validate"
)

func init() {
    // ... existing persistent flags setup ...

    // Register built-in commands from the registry.
    // This must happen BEFORE custom commands are processed.
    if err := internal.RegisterAll(RootCmd); err != nil {
        log.Error("Failed to register built-in commands", "error", err)
    }

    // ... rest of existing init code ...
}

func Execute() error {
    // ... existing config loading ...

    var err error

    // Process custom commands from atmos.yaml.
    // Custom commands are processed AFTER built-in commands,
    // allowing them to extend or override built-in functionality.
    if initErr == nil {
        err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
        if err != nil {
            return err
        }

        err = processCommandAliases(atmosConfig, atmosConfig.CommandAliases, RootCmd, true)
        if err != nil {
            return err
        }
    }

    // ... existing execution code ...
}
```

## Integration with Custom Commands

### Execution Flow

```
1. Application Start
   ├── Load atmos.yaml configuration
   │
2. Built-in Command Registration (init() functions)
   ├── Blank imports trigger init() in each command package
   ├── Each command registers via internal.Register()
   ├── internal.RegisterAll() adds commands to RootCmd
   │
3. Custom Command Processing (Execute() function)
   ├── processCustomCommands() reads atmos.yaml
   ├── Generates cobra.Command for each custom command
   ├── Adds custom commands to RootCmd
   │   ├── If custom command name conflicts with built-in:
   │   │   └── Custom command REPLACES built-in command
   │   └── Otherwise:
   │       └── Custom command EXTENDS available commands
   │
4. Command Execution
   └── User runs: atmos <command> <args>
       ├── Cobra finds command (custom or built-in)
       └── Executes appropriate handler
```

### Custom Command Override Behavior

**Scenario 1: Custom command with same name as built-in (top-level command)**

```yaml
# atmos.yaml
commands:
  - name: terraform  # Same name as built-in command
    description: Custom terraform wrapper with extra validation
    steps:
      - echo "Running custom terraform validation..."
      - atmos validate component {{ .arguments.component }} -s {{ .arguments.stack }}
      - terraform {{ .arguments.subcommand }}
```

**Result:** Custom `terraform` command **reuses** the built-in `terraform` command and **replaces its behavior**.

**Scenario 2: Custom command extending built-in with subcommands**

```yaml
# atmos.yaml
commands:
  - name: terraform  # Same name as built-in command
    commands:  # Add subcommands
      - name: custom-plan
        description: Custom plan with extra validation
        steps:
          - echo "Running custom validation..."
          - terraform plan
```

**Result:** Custom `custom-plan` subcommand is **added** to the built-in `terraform` command. The built-in command still works, but now has additional subcommands.

**Scenario 3: Custom command with new name**

```yaml
# atmos.yaml
commands:
  - name: deploy  # New command, not built-in
    description: Deploy infrastructure end-to-end
    steps:
      - atmos terraform plan {{ .arguments.component }} -s {{ .arguments.stack }}
      - atmos terraform apply {{ .arguments.component }} -s {{ .arguments.stack }}
```

**Result:** Custom `deploy` command is **added** alongside built-in commands.

### Why This Works

The existing `processCustomCommands()` function (in `cmd/cmd_utils.go`) handles command extension:

```go
// From cmd/cmd_utils.go (simplified)
func processCustomCommands(
    atmosConfig schema.AtmosConfiguration,
    commands []schema.Command,
    parentCommand *cobra.Command,
    topLevel bool,
) error {
    existingTopLevelCommands := make(map[string]*cobra.Command)

    if topLevel {
        // Get all existing commands registered by the registry
        existingTopLevelCommands = getTopLevelCommands()
    }

    for _, commandCfg := range commands {
        var command *cobra.Command

        // Check if command already exists (from registry)
        if _, exist := existingTopLevelCommands[commandCfg.Name]; exist && topLevel {
            // REUSE the existing command - this allows extending it
            command = existingTopLevelCommands[commandCfg.Name]
        } else {
            // CREATE new custom command
            command = &cobra.Command{ /* ... */ }
            parentCommand.AddCommand(command)
        }

        // Recursively process nested commands (subcommands)
        processCustomCommands(atmosConfig, commandCfg.Commands, command, false)
    }
}
```

**Key behaviors:**
- **Top-level custom command with existing name** → reuses registry command, can replace behavior or add subcommands
- **Top-level custom command with new name** → creates new command
- **Nested custom commands** → always added as subcommands to parent

**No changes needed to custom command processing** - the registry pattern coexists perfectly.

## Migration Guide

### Prerequisites

Before migrating a command:
- ✅ Command must have tests
- ✅ Command should be self-contained (minimal dependencies on other cmd/ files)
- ✅ Understand command's relationship to custom commands (could it be overridden?)

### Migration Steps

#### Step 1: Create Command Package Directory

```bash
mkdir -p cmd/[command-family]
```

Examples:
- `cmd/terraform/` for terraform commands
- `cmd/describe/` for describe commands
- `cmd/list/` for list commands

#### Step 2: Move Command Files

```bash
# Move implementation
mv cmd/[command]*.go cmd/[command-family]/

# Move tests
mv cmd/[command]*_test.go cmd/[command-family]/
```

Example:
```bash
mkdir -p cmd/about
mv cmd/about.go cmd/about/about.go
mv cmd/about_test.go cmd/about/about_test.go
```

#### Step 3: Implement CommandProvider Interface

Add to the main command file:

```go
// cmd/[command-family]/[command].go
package [command-family]

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

var [command]Cmd = &cobra.Command{
    // ... existing command definition ...
}

func init() {
    // Register with the registry
    internal.Register(&[Command]CommandProvider{})
}

// [Command]CommandProvider implements the CommandProvider interface.
type [Command]CommandProvider struct{}

func (c *[Command]CommandProvider) GetCommand() *cobra.Command {
    return [command]Cmd
}

func (c *[Command]CommandProvider) GetName() string {
    return "[command]"
}

func (c *[Command]CommandProvider) GetGroup() string {
    return "[Command Group Name]"
}
```

#### Step 4: Update root.go Imports

Add blank import to `cmd/root.go`:

```go
import (
    // ... existing imports ...

    _ "github.com/cloudposse/atmos/cmd/[command-family]"
)
```

#### Step 5: Remove Old Files

```bash
# After verifying tests pass
rm cmd/[command].go
rm cmd/[command]_test.go
```

#### Step 6: Run Tests

```bash
go test ./cmd/[command-family]/...
go test ./cmd/...
make testacc
```

#### Step 7: Update Imports (if needed)

If other packages imported the old command:

```bash
# Find references
grep -r "github.com/cloudposse/atmos/cmd" --include="*.go"

# Update to new path (usually not needed for cmd/ files)
```

### Command Groups

Use these standard groups for consistency:

| Group Name | Commands |
|-----------|----------|
| **Core Stack Commands** | terraform, helmfile, workflow, packer |
| **Stack Introspection** | describe, list, validate |
| **Configuration Management** | vendor, docs |
| **Cloud Integration** | aws, atlantis |
| **Pro Features** | auth, pro |
| **Other Commands** | about, completion, version, support |

### Example: Migrating "list" Command Family

**Before:**
```
cmd/
├── list.go
├── list_components.go
├── list_stacks.go
├── list_workflows.go
├── list_instances.go
├── list_metadata.go
├── list_settings.go
├── list_values.go
├── list_vendor.go
└── ... (other files)
```

**After:**
```
cmd/
├── list/
│   ├── list.go              # Base command + CommandProvider
│   ├── components.go
│   ├── stacks.go
│   ├── workflows.go
│   ├── instances.go
│   ├── metadata.go
│   ├── settings.go
│   ├── values.go
│   └── vendor.go
└── ... (other commands)
```

**list.go implementation:**

```go
// cmd/list/list.go
package list

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

// listCmd represents the base list command.
var listCmd = &cobra.Command{
    Use:   "list",
    Short: "List components, stacks, and other resources",
    Long:  `Display lists of Atmos resources.`,
}

func init() {
    // Add subcommands
    listCmd.AddCommand(componentsCmd)
    listCmd.AddCommand(stacksCmd)
    listCmd.AddCommand(workflowsCmd)
    listCmd.AddCommand(instancesCmd)
    listCmd.AddCommand(metadataCmd)
    listCmd.AddCommand(settingsCmd)
    listCmd.AddCommand(valuesCmd)
    listCmd.AddCommand(vendorCmd)

    // Register with registry
    internal.Register(&ListCommandProvider{})
}

// ListCommandProvider implements the CommandProvider interface.
type ListCommandProvider struct{}

func (l *ListCommandProvider) GetCommand() *cobra.Command {
    return listCmd
}

func (l *ListCommandProvider) GetName() string {
    return "list"
}

func (l *ListCommandProvider) GetGroup() string {
    return "Stack Introspection"
}
```

## Nested Command Patterns

The registry pattern supports **three types of command nesting** found in Atmos:

### Pattern 1: Static Subcommands (describe, list, validate)

**Characteristics:**
- Parent command is a grouping command (no RunE function)
- Subcommands defined in separate files
- Each subcommand manually added to parent via `AddCommand()`

**Example: `describe` command family**

```
atmos describe            # Parent command (no-op, shows help)
atmos describe component  # Subcommand
atmos describe stacks     # Subcommand
atmos describe affected   # Subcommand
```

**Current structure:**
```
cmd/
├── describe.go              # Parent command
├── describe_component.go    # Subcommand
├── describe_stacks.go       # Subcommand
├── describe_affected.go     # Subcommand
├── describe_dependents.go   # Subcommand
└── describe_workflows.go    # Subcommand
```

**Migrated structure:**
```
cmd/describe/
├── describe.go         # Parent + CommandProvider
├── component.go        # Subcommand
├── stacks.go          # Subcommand
├── affected.go        # Subcommand
├── dependents.go      # Subcommand
└── workflows.go       # Subcommand
```

**Implementation:**

```go
// cmd/describe/describe.go
package describe

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

// describeCmd is the parent command for all describe subcommands.
var describeCmd = &cobra.Command{
    Use:   "describe",
    Short: "Show details about Atmos configurations and components",
    Long:  `Display configuration details for Atmos CLI, stacks, and components.`,
    Args:  cobra.NoArgs,
}

func init() {
    // Add persistent flags for all describe subcommands
    describeCmd.PersistentFlags().StringP("query", "q", "",
        "Query the results using yq expressions")

    // Attach all subcommands to parent
    describeCmd.AddCommand(componentCmd)
    describeCmd.AddCommand(stacksCmd)
    describeCmd.AddCommand(affectedCmd)
    describeCmd.AddCommand(dependentsCmd)
    describeCmd.AddCommand(workflowsCmd)
    describeCmd.AddCommand(configCmd)

    // Register parent command with registry
    internal.Register(&DescribeCommandProvider{})
}

// DescribeCommandProvider implements the CommandProvider interface.
type DescribeCommandProvider struct{}

func (d *DescribeCommandProvider) GetCommand() *cobra.Command {
    return describeCmd
}

func (d *DescribeCommandProvider) GetName() string {
    return "describe"
}

func (d *DescribeCommandProvider) GetGroup() string {
    return "Stack Introspection"
}
```

```go
// cmd/describe/component.go
package describe

import (
    "github.com/spf13/cobra"
    e "github.com/cloudposse/atmos/internal/exec"
)

var componentCmd = &cobra.Command{
    Use:   "component",
    Short: "Show configuration details for an Atmos component in a stack",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return e.ExecuteDescribeComponent(cmd, args)
    },
}

func init() {
    componentCmd.Flags().StringP("stack", "s", "", "Atmos stack (required)")
    componentCmd.MarkFlagRequired("stack")
    componentCmd.Flags().String("format", "yaml", "Output format: yaml, json")
    componentCmd.Flags().String("file", "", "Write output to file")
    componentCmd.Flags().Bool("process-templates", true, "Process Go templates")
}
```

**Key Points:**
- ✅ Only **parent command** registers with registry
- ✅ Subcommands are package-private (lowercase variable names)
- ✅ Subcommands attached in parent's `init()`
- ✅ All related code in one package directory

---

### Pattern 2: Dynamic Subcommands (terraform, helmfile, packer)

**Characteristics:**
- Parent command proxies to external tool (terraform, helmfile, etc.)
- Subcommands dynamically generated from array
- Common execution logic shared across subcommands

**Example: `terraform` command family**

```
atmos terraform plan      # Dynamic subcommand
atmos terraform apply     # Dynamic subcommand
atmos terraform destroy   # Dynamic subcommand
atmos terraform workspace # Dynamic subcommand
```

**Current structure:**
```
cmd/
├── terraform.go              # Parent command
├── terraform_commands.go     # Dynamic subcommand generator
├── terraform_generate.go     # Static subcommand group
├── terraform_generate_backend.go
├── terraform_generate_varfile.go
└── terraform_utils.go        # Shared utilities
```

**Migrated structure:**
```
cmd/terraform/
├── terraform.go          # Parent + CommandProvider
├── commands.go           # Dynamic subcommand definitions
├── generate/             # Nested subcommand group
│   ├── generate.go      # Generate parent command
│   ├── backend.go       # Subcommand
│   ├── backends.go      # Subcommand
│   ├── varfile.go       # Subcommand
│   └── varfiles.go      # Subcommand
└── utils.go             # Shared utilities
```

**Implementation:**

```go
// cmd/terraform/terraform.go
package terraform

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
    "github.com/cloudposse/atmos/cmd/terraform/generate"
)

var terraformCmd = &cobra.Command{
    Use:     "terraform",
    Aliases: []string{"tf"},
    Short:   "Execute Terraform commands using Atmos stack configurations",
    Long:    `Execute Terraform commands with Atmos configuration management.`,
}

func init() {
    // Special Terraform settings
    terraformCmd.DisableFlagParsing = true
    terraformCmd.FParseErrWhitelist.UnknownFlags = true

    // Attach dynamic commands
    attachTerraformCommands(terraformCmd)

    // Attach static subcommand groups
    terraformCmd.AddCommand(generate.GenerateCmd)

    // Register with registry
    internal.Register(&TerraformCommandProvider{})
}

type TerraformCommandProvider struct{}

func (t *TerraformCommandProvider) GetCommand() *cobra.Command {
    return terraformCmd
}

func (t *TerraformCommandProvider) GetName() string {
    return "terraform"
}

func (t *TerraformCommandProvider) GetGroup() string {
    return "Core Stack Commands"
}
```

```go
// cmd/terraform/commands.go
package terraform

import (
    "github.com/spf13/cobra"
)

// getTerraformCommands returns dynamically generated Terraform subcommands.
func getTerraformCommands() []*cobra.Command {
    return []*cobra.Command{
        {
            Use:   "plan",
            Short: "Show changes required by the current configuration",
            Long:  "Generate an execution plan.",
        },
        {
            Use:   "apply",
            Short: "Apply changes to infrastructure",
            Long:  "Apply the changes required to reach the desired state.",
        },
        {
            Use:   "destroy",
            Short: "Destroy infrastructure",
            Long:  "Destroy all resources managed by this configuration.",
        },
        // ... more commands
    }
}

// attachTerraformCommands attaches dynamic commands to parent.
func attachTerraformCommands(parentCmd *cobra.Command) {
    // Add persistent flags that apply to all terraform subcommands
    parentCmd.PersistentFlags().Bool("skip-init", false,
        "Skip running terraform init")
    parentCmd.PersistentFlags().StringP("query", "q", "",
        "Filter components using yq expression")

    // Attach each dynamic command
    commands := getTerraformCommands()
    for _, cmd := range commands {
        cmd.DisableFlagParsing = true
        cmd.FParseErrWhitelist.UnknownFlags = true
        parentCmd.AddCommand(cmd)
    }
}
```

```go
// cmd/terraform/generate/generate.go
package generate

import (
    "github.com/spf13/cobra"
)

// GenerateCmd is exported for use by parent terraform command.
var GenerateCmd = &cobra.Command{
    Use:   "generate",
    Short: "Generate Terraform configuration files",
    Long:  `Generate backends, varfiles, and other Terraform artifacts.`,
}

func init() {
    // Attach generate subcommands
    GenerateCmd.AddCommand(backendCmd)
    GenerateCmd.AddCommand(backendsCmd)
    GenerateCmd.AddCommand(varfileCmd)
    GenerateCmd.AddCommand(varfilesCmd)
    GenerateCmd.AddCommand(planfileCmd)
}
```

**Key Points:**
- ✅ Parent command registers with registry
- ✅ Dynamic subcommands generated from array
- ✅ Nested subcommand groups (e.g., `terraform generate`) in sub-packages
- ✅ Exported variables (e.g., `GenerateCmd`) for cross-package visibility
- ✅ Shared utilities in same package

---

### Pattern 3: Deeply Nested Commands (aws, atlantis)

**Characteristics:**
- Multiple levels of nesting (grandparent → parent → child)
- Each level may have its own flags and logic

**Example: `aws` command family**

```
atmos aws                      # Grandparent command
atmos aws eks                  # Parent command
atmos aws eks update-kubeconfig # Child command
```

**Current structure:**
```
cmd/
├── aws.go                      # Grandparent
├── aws_eks.go                  # Parent
└── aws_eks_update_kubeconfig.go # Child
```

**Migrated structure:**
```
cmd/aws/
├── aws.go           # Grandparent + CommandProvider
└── eks/
    ├── eks.go              # Parent command
    └── update_kubeconfig.go # Child command
```

**Implementation:**

```go
// cmd/aws/aws.go
package aws

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
    "github.com/cloudposse/atmos/cmd/aws/eks"
)

var awsCmd = &cobra.Command{
    Use:   "aws",
    Short: "AWS commands",
    Long:  `Execute AWS-related commands.`,
}

func init() {
    // Attach nested command groups
    awsCmd.AddCommand(eks.EksCmd)

    // Register with registry
    internal.Register(&AWSCommandProvider{})
}

type AWSCommandProvider struct{}

func (a *AWSCommandProvider) GetCommand() *cobra.Command {
    return awsCmd
}

func (a *AWSCommandProvider) GetName() string {
    return "aws"
}

func (a *AWSCommandProvider) GetGroup() string {
    return "Cloud Integration"
}
```

```go
// cmd/aws/eks/eks.go
package eks

import (
    "github.com/spf13/cobra"
)

// EksCmd is exported for use by parent aws command.
var EksCmd = &cobra.Command{
    Use:   "eks",
    Short: "AWS EKS commands",
    Long:  `Manage AWS EKS clusters.`,
}

func init() {
    // Attach EKS subcommands
    EksCmd.AddCommand(updateKubeconfigCmd)
}
```

```go
// cmd/aws/eks/update_kubeconfig.go
package eks

import (
    "github.com/spf13/cobra"
    e "github.com/cloudposse/atmos/internal/exec"
)

var updateKubeconfigCmd = &cobra.Command{
    Use:   "update-kubeconfig",
    Short: "Update kubeconfig for EKS cluster",
    RunE: func(cmd *cobra.Command, args []string) error {
        return e.ExecuteAwsEksUpdateKubeconfig(cmd, args)
    },
}

func init() {
    updateKubeconfigCmd.Flags().StringP("stack", "s", "", "Atmos stack")
    updateKubeconfigCmd.MarkFlagRequired("stack")
}
```

**Key Points:**
- ✅ Only **top-level** command registers with registry
- ✅ Nested packages for each level (aws → eks → update-kubeconfig)
- ✅ Exported parent commands for cross-package visibility
- ✅ Clear directory hierarchy mirrors command hierarchy

---

## Nested Command Migration Checklist

When migrating commands with subcommands:

### For Static Subcommands (describe, list):
- [ ] Create package directory: `cmd/[command]/`
- [ ] Move parent command file
- [ ] Move all subcommand files
- [ ] Update parent's `init()` to call `AddCommand()` for each subcommand
- [ ] Implement CommandProvider in parent
- [ ] Register only parent with registry
- [ ] Change subcommand variables to lowercase (package-private)

### For Dynamic Subcommands (terraform, helmfile):
- [ ] Create package directory: `cmd/[command]/`
- [ ] Move parent command file
- [ ] Move command generator file (e.g., `terraform_commands.go` → `commands.go`)
- [ ] Create sub-packages for nested groups (e.g., `generate/`)
- [ ] Export nested parent commands (e.g., `GenerateCmd`)
- [ ] Implement CommandProvider in top-level parent
- [ ] Register only top-level parent with registry

### For Deeply Nested Commands (aws):
- [ ] Create package hierarchy: `cmd/[grandparent]/[parent]/`
- [ ] Move each level to appropriate directory
- [ ] Export intermediate parent commands
- [ ] Import child packages in parent
- [ ] Implement CommandProvider in top-level grandparent
- [ ] Register only top-level grandparent with registry

---

## Testing Strategy

### Unit Tests for Registry

```go
// cmd/internal/registry_test.go
package internal

import (
    "testing"

    "github.com/spf13/cobra"
    "github.com/stretchr/testify/assert"
)

func TestRegister(t *testing.T) {
    Reset() // Clear registry for clean test

    provider := &mockProvider{
        name:  "test",
        group: "Test",
        cmd:   &cobra.Command{Use: "test"},
    }

    Register(provider)

    assert.Equal(t, 1, Count())

    retrieved, ok := GetProvider("test")
    assert.True(t, ok)
    assert.Equal(t, provider, retrieved)
}

func TestNestedCommands(t *testing.T) {
    Reset()

    // Create parent with subcommands
    parentCmd := &cobra.Command{Use: "parent"}
    childCmd := &cobra.Command{Use: "child"}
    parentCmd.AddCommand(childCmd)

    provider := &mockProvider{
        name:  "parent",
        group: "Test",
        cmd:   parentCmd,
    }

    Register(provider)

    // Verify parent is registered
    retrieved, ok := GetProvider("parent")
    assert.True(t, ok)
    assert.True(t, retrieved.GetCommand().HasSubCommands())

    // Verify child is accessible
    subCmd, _, err := retrieved.GetCommand().Find([]string{"child"})
    assert.NoError(t, err)
    assert.Equal(t, "child", subCmd.Use)
}

func TestCustomCommandOverride(t *testing.T) {
    // Test that custom commands can override built-in commands
    // This is handled by Cobra's AddCommand() behavior
    // and processCustomCommands() function
}
```

### Integration Tests

```go
// Test that migrated commands still work with custom commands
func TestMigratedCommandWithCustomCommand(t *testing.T) {
    // Setup: Create atmos.yaml with custom "about" command
    // Execute: Run atmos about
    // Verify: Custom command executes instead of built-in
}

// Test nested command execution
func TestNestedCommandExecution(t *testing.T) {
    // Execute: atmos describe component vpc -s dev
    // Verify: Command executes correctly
}

// Test deeply nested command execution
func TestDeeplyNestedCommandExecution(t *testing.T) {
    // Execute: atmos aws eks update-kubeconfig -s prod
    // Verify: Command executes correctly
}
```

## Future: Plugin Support

The registry pattern provides the foundation for external plugins:

### Plugin Architecture (Future Phase)

```go
// pkg/plugin/plugin.go (Future)
package plugin

// Plugin represents an external Atmos plugin.
type Plugin struct {
    Name     string
    Command  *cobra.Command
    Metadata *PluginMetadata
}

// PluginProvider implements CommandProvider for plugins.
type PluginProvider struct {
    plugin *Plugin
}

func (p *PluginProvider) GetCommand() *cobra.Command {
    return p.plugin.Command
}

func (p *PluginProvider) GetName() string {
    return p.plugin.Name
}

func (p *PluginProvider) GetGroup() string {
    return "Plugins"
}

// RegisterPlugin adds a discovered plugin to the registry.
func RegisterPlugin(plugin *Plugin) {
    internal.Register(&PluginProvider{plugin: plugin})
}
```

**Plugin discovery flow (future):**
1. Atmos starts → loads built-in commands via registry
2. Discovers plugins in `~/.atmos/plugins/`
3. Registers plugins as CommandProviders
4. Processes custom commands from atmos.yaml
5. All three sources coexist:
   - Built-in commands (compiled in)
   - Plugins (external executables)
   - Custom commands (from atmos.yaml)

## Benefits

### Immediate Benefits (Phase 1)

1. ✅ **Better organization** - Related commands grouped in packages
2. ✅ **Easier navigation** - Clear directory structure
3. ✅ **Self-documenting** - Package names show command families
4. ✅ **Consistent pattern** - All commands follow same structure
5. ✅ **Custom command compatibility** - No impact on existing functionality

### Future Benefits (Phase 2+)

6. ✅ **Plugin support** - Foundation for external plugins
7. ✅ **Command marketplace** - Community can share plugins
8. ✅ **Extensibility** - Users can extend Atmos without forking

## Risks & Mitigation

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Custom command conflicts | Low | Medium | Test with custom command examples |
| Import path confusion | Low | Low | Clear documentation, examples |
| Init() ordering issues | Low | Medium | Explicit import ordering, testing |
| Breaking existing workflows | Low | High | Comprehensive testing, gradual rollout |

## Success Criteria

### Phase 1: Foundation (This PR)

- ✅ Registry pattern implemented and tested
- ✅ `about` command migrated successfully
- ✅ All existing tests pass
- ✅ Custom commands still work (verified with test cases)
- ✅ No behavior changes for users
- ✅ Documentation complete (this PRD)

### Future Phases: Full Migration

- ✅ All commands migrated to registry pattern
- ✅ Plugin system implemented
- ✅ Community plugins available

## FAQ

### Q: Will this break custom commands?

**A:** No. Custom commands are processed **after** built-in commands in the `Execute()` function. The registry pattern only affects how built-in commands are organized internally. Custom commands from `atmos.yaml` continue to work exactly as before.

### Q: Can custom commands still override built-in commands?

**A:** Yes. The execution order is:
1. Built-in commands registered via registry
2. Custom commands processed from atmos.yaml
3. If custom command has same name → overrides built-in
4. If custom command has new name → extends available commands

### Q: Do I need to update my atmos.yaml?

**A:** No. The registry pattern is purely internal. User-facing configuration and behavior remain unchanged.

### Q: What happens if I have a custom command named "about"?

**A:** Your custom command will override the built-in `about` command, just like it does today. The registry pattern doesn't change this behavior.

### Q: Can multiple commands register with the same name?

**A:** The registry allows re-registration (last one wins), but in practice:
- Built-in commands should have unique names
- Custom commands override via Cobra's AddCommand()
- Plugins (future) will have conflict resolution

### Q: How do I know which commands are built-in vs custom?

**A:** Use `atmos --help` to see all commands. Custom commands show their description from `atmos.yaml`. Future enhancement: mark custom commands in help output.

### Q: Will command aliases still work?

**A:** Yes. Command aliases are processed separately via `processCommandAliases()` and work with both built-in and custom commands.

## References

- [Cobra Command Documentation](https://github.com/spf13/cobra)
- [Atmos Custom Commands](/core-concepts/custom-commands)
- [Go Init Functions](https://golang.org/doc/effective_go#init)
- [Docker CLI Command Registry](https://github.com/docker/cli/tree/master/cli/command)
- [kubectl Plugin System](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-10-15 | Initial PRD with custom command integration |
