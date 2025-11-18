# Developing Atmos Commands

This guide explains how to create new commands in Atmos using the command registry pattern.

## Overview

Atmos uses a **command registry pattern** for organizing built-in commands. This pattern provides:

- ✅ **Self-registering commands** - Commands register themselves via `init()`
- ✅ **Modular organization** - Each command family in its own package
- ✅ **Type-safe interfaces** - CommandProvider interface for consistency
- ✅ **Plugin readiness** - Foundation for future external plugins
- ✅ **Custom command compatibility** - Works seamlessly with custom commands from `atmos.yaml`

## Quick Start

### Creating a Simple Command

**Step 1: Create the package directory**

```bash
mkdir -p cmd/mycommand
```

**Step 2: Create the command file**

```go
// cmd/mycommand/mycommand.go
package mycommand

import (
    "github.com/spf13/cobra"

    "github.com/cloudposse/atmos/cmd/internal"
    e "github.com/cloudposse/atmos/internal/exec"
)

// mycommandCmd represents the mycommand command.
var mycommandCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Brief description of your command",
    Long:  `Detailed description of what your command does.`,
    RunE: func(cmd *cobra.Command, args []string) error {
        return e.ExecuteMyCommand(cmd, args)
    },
}

func init() {
    // Add flags if needed
    mycommandCmd.Flags().StringP("stack", "s", "", "Atmos stack")

    // Register with the registry
    internal.Register(&MyCommandProvider{})
}

// MyCommandProvider implements the CommandProvider interface.
type MyCommandProvider struct{}

func (m *MyCommandProvider) GetCommand() *cobra.Command {
    return mycommandCmd
}

func (m *MyCommandProvider) GetName() string {
    return "mycommand"
}

func (m *MyCommandProvider) GetGroup() string {
    return "Other Commands" // See "Command Groups" section
}

func (m *MyCommandProvider) GetAliases() []internal.CommandAlias {
    return nil // No aliases (see "Adding Command Aliases" section)
}
```

**Step 3: Add the business logic**

Create the implementation in `internal/exec/`:

```go
// internal/exec/mycommand.go
package exec

import (
    "github.com/spf13/cobra"
)

// ExecuteMyCommand executes the mycommand command.
func ExecuteMyCommand(cmd *cobra.Command, args []string) error {
    // Implement your command logic here
    return nil
}
```

**Step 4: Register the command**

Add a blank import to `cmd/root.go`:

```go
// cmd/root.go
import (
    // ... existing imports ...

    _ "github.com/cloudposse/atmos/cmd/mycommand"
)
```

**Step 5: Add tests**

```go
// cmd/mycommand/mycommand_test.go
package mycommand

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestMyCommandProvider(t *testing.T) {
    provider := &MyCommandProvider{}

    assert.Equal(t, "mycommand", provider.GetName())
    assert.Equal(t, "Other Commands", provider.GetGroup())
    assert.NotNil(t, provider.GetCommand())
}
```

**Done!** Your command is now available as `atmos mycommand`.

---

## Command Patterns

Atmos has three main command patterns:

### Pattern 1: Simple Command

A standalone command with no subcommands.

**Example:** `about`, `version`, `support`

**Structure:**
```
cmd/about/
├── about.go       # Command + CommandProvider
└── about_test.go  # Tests
```

**Implementation:** See "Quick Start" above.

---

### Pattern 2: Command with Static Subcommands

A parent command with predefined subcommands.

**Example:** `describe` (with `component`, `stacks`, `affected`, etc.)

**Structure:**
```
cmd/describe/
├── describe.go       # Parent command + CommandProvider
├── component.go      # Subcommand
├── stacks.go        # Subcommand
├── affected.go      # Subcommand
└── dependents.go    # Subcommand
```

**Implementation:**

```go
// cmd/describe/describe.go
package describe

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

// describeCmd is the parent command.
var describeCmd = &cobra.Command{
    Use:   "describe",
    Short: "Show details about Atmos configurations",
    Long:  `Display configuration details for stacks and components.`,
}

func init() {
    // Add persistent flags (apply to all subcommands)
    describeCmd.PersistentFlags().StringP("query", "q", "",
        "Query results using yq expressions")

    // Attach subcommands
    describeCmd.AddCommand(componentCmd)
    describeCmd.AddCommand(stacksCmd)
    describeCmd.AddCommand(affectedCmd)

    // Register parent command
    internal.Register(&DescribeCommandProvider{})
}

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

// componentCmd is a subcommand (lowercase = package-private).
var componentCmd = &cobra.Command{
    Use:   "component",
    Short: "Show configuration for a component",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return e.ExecuteDescribeComponent(cmd, args)
    },
}

func init() {
    componentCmd.Flags().StringP("stack", "s", "", "Atmos stack")
    componentCmd.MarkFlagRequired("stack")
}
```

**Key Points:**
- Only parent command implements CommandProvider
- Subcommands are package-private (lowercase variables)
- Subcommands attached in parent's `init()`

---

### Pattern 3: Command with Dynamic Subcommands

A parent command with subcommands generated from arrays.

**Example:** `terraform` (with `plan`, `apply`, `destroy`, etc.)

**Structure:**
```
cmd/terraform/
├── terraform.go      # Parent + CommandProvider
├── commands.go       # Dynamic subcommand generator
├── generate/         # Nested static subcommand group
│   ├── generate.go
│   ├── backend.go
│   └── varfile.go
└── utils.go          # Shared utilities
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
    Short:   "Execute Terraform commands",
}

func init() {
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

import "github.com/spf13/cobra"

func getTerraformCommands() []*cobra.Command {
    return []*cobra.Command{
        {
            Use:   "plan",
            Short: "Show changes required",
        },
        {
            Use:   "apply",
            Short: "Apply changes",
        },
        // ... more commands
    }
}

func attachTerraformCommands(parentCmd *cobra.Command) {
    parentCmd.PersistentFlags().Bool("skip-init", false,
        "Skip terraform init")

    for _, cmd := range getTerraformCommands() {
        parentCmd.AddCommand(cmd)
    }
}
```

```go
// cmd/terraform/generate/generate.go
package generate

import "github.com/spf13/cobra"

// GenerateCmd is exported for use by parent terraform command.
var GenerateCmd = &cobra.Command{
    Use:   "generate",
    Short: "Generate Terraform configuration files",
}

func init() {
    GenerateCmd.AddCommand(backendCmd)
    GenerateCmd.AddCommand(varfileCmd)
}
```

**Key Points:**
- Dynamic commands from array
- Nested subcommand groups in sub-packages
- Exported parent commands (uppercase) for cross-package imports

---

### Pattern 4: Deeply Nested Commands

Multiple levels of nesting (grandparent → parent → child).

**Example:** `aws eks update-kubeconfig`

**Structure:**
```
cmd/aws/
├── aws.go                  # Grandparent + CommandProvider
└── eks/
    ├── eks.go              # Parent (exported)
    └── update_kubeconfig.go # Child
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
}

func init() {
    awsCmd.AddCommand(eks.EksCmd)
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

import "github.com/spf13/cobra"

// EksCmd is exported for use by parent aws command.
var EksCmd = &cobra.Command{
    Use:   "eks",
    Short: "AWS EKS commands",
}

func init() {
    EksCmd.AddCommand(updateKubeconfigCmd)
}
```

**Key Points:**
- Only top-level command registers with registry
- Nested packages mirror command hierarchy
- Export intermediate parents (uppercase variables)

---

## Command Groups

Use these standard groups for `GetGroup()`:

| Group | Commands |
|-------|----------|
| **Core Stack Commands** | terraform, helmfile, workflow, packer |
| **Stack Introspection** | describe, list, validate |
| **Configuration Management** | vendor, docs |
| **Cloud Integration** | aws, atlantis |
| **Pro Features** | auth, pro |
| **Other Commands** | about, completion, version, support |

---

## Adding Command Aliases

Command aliases allow your command to be accessible under different parent commands, improving discoverability without code duplication.

### When to Use Aliases

Use aliases when:
- **Improved UX**: A command makes more sense under multiple parents (e.g., `profile list` and `list profiles`)
- **Discoverability**: Users might naturally look for a command in different locations
- **Grouping**: Want to group related operations (e.g., all list operations under `atmos list`)
- **Migration**: Maintaining backwards compatibility while reorganizing commands

### How to Add an Alias

Implement the `GetAliases()` method in your CommandProvider:

```go
// cmd/profile/profile.go
package profile

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

type ProfileCommandProvider struct{}

func (p *ProfileCommandProvider) GetCommand() *cobra.Command {
    return profileCmd
}

func (p *ProfileCommandProvider) GetName() string {
    return "profile"
}

func (p *ProfileCommandProvider) GetGroup() string {
    return "Configuration Management"
}

// GetAliases creates "atmos list profiles" as an alias for "atmos profile list".
func (p *ProfileCommandProvider) GetAliases() []internal.CommandAlias {
    return []internal.CommandAlias{
        {
            Subcommand:    "list",           // Which subcommand to alias
            ParentCommand: "list",           // Under which parent
            Name:          "profiles",       // Alias name
            Short:         "List available configuration profiles",
            Long:          `This is an alias for "atmos profile list".`,
            Example: `# List all available profiles
atmos list profiles

# List profiles in JSON format
atmos list profiles --format json`,
        },
    }
}
```

### Alias Configuration Fields

| Field | Description | Example |
|-------|-------------|---------|
| `Subcommand` | Name of subcommand to alias (empty string for parent) | `"list"` |
| `ParentCommand` | Parent command to add alias under | `"list"` |
| `Name` | Alias command name | `"profiles"` |
| `Short` | Short description | `"List available configuration profiles"` |
| `Long` | Detailed description | `This is an alias for "atmos profile list".` |
| `Example` | Usage examples | Multi-line example string |

### How Aliases Work

Aliases provide **true delegation** to the source command:

1. **Flag Sharing**: Uses `AddFlag` to share the same flag instance
   - Flag values set on alias are visible to source's RunE function
   - No manual flag copying needed
   - Validation happens naturally through shared state

2. **Execution Delegation**:
   - **Args**: Same argument validation
   - **RunE**: Executes same business logic
   - **Completion**: Same shell completion

3. **No Code Duplication**:
   - All logic stays in source command
   - Changes to source automatically apply to aliases

### Examples

**Aliasing a Subcommand:**
```go
// Makes "atmos list profiles" an alias for "atmos profile list"
{
    Subcommand:    "list",
    ParentCommand: "list",
    Name:          "profiles",
    Short:         "List available configuration profiles",
}
```

**Aliasing a Parent Command:**
```go
// Makes "atmos help" an alias for "atmos about"
{
    Subcommand:    "",  // Empty = alias the parent command itself
    ParentCommand: "help",
    Name:          "about",
    Short:         "Show information about Atmos",
}
```

**No Aliases:**
```go
// Most commands don't need aliases
func (m *MyCommandProvider) GetAliases() []internal.CommandAlias {
    return nil
}
```

### Best Practices for Aliases

- **Clear documentation**: Always indicate it's an alias in Long description
- **Consistent naming**: Use plural for collections (`profiles`, `stacks`)
- **Limit aliases**: Don't create too many paths to the same command
- **Test both paths**: Ensure both source and alias work identically
- **Update examples**: Include examples for the alias command

---

## Best Practices

### 1. Command Naming

- **Use lowercase** for command names
- **Use hyphens** for multi-word commands (e.g., `update-kubeconfig`)
- **Be concise** but descriptive
- **Follow existing conventions** in Atmos

### 2. Package Organization

- **One command family per package**
- **Package name matches command name** (e.g., `cmd/terraform/`)
- **Subcommands in same package** as parent
- **Nested subcommand groups** in sub-packages

### 3. Variable Naming

- **Parent commands**: lowercase (package-private) unless exported
- **Exported commands**: Uppercase (e.g., `GenerateCmd`, `EksCmd`)
- **CommandProvider**: `<Name>CommandProvider` (e.g., `AboutCommandProvider`)

### 4. Documentation

- **Short**: One-line description (50-80 chars)
- **Long**: Detailed description with context
- **Examples**: Use embedded markdown files in `cmd/markdown/`

### 5. Flags

- **Common flags** on parent via `PersistentFlags()`
- **Specific flags** on subcommands via `Flags()`
- **Required flags** marked with `MarkFlagRequired()`

### 6. Testing

- **Test CommandProvider implementation**
- **Test command logic** in `internal/exec/`
- **Integration tests** in `tests/`
- **Use table-driven tests** for multiple scenarios

---

## Using Markdown Content

Atmos maintains all markdown content in the centralized `cmd/markdown/` directory. Command packages access this content through the `cmd/markdown` package.

**Step 1: Add your markdown file**

Create your markdown file in `cmd/markdown/`:

```bash
# cmd/markdown/mycommand_description.md
# My Command

Detailed description of what your command does...
```

**Step 2: Export the markdown in `cmd/markdown/content.go`**

```go
// cmd/markdown/content.go
package markdown

import _ "embed"

//go:embed mycommand_description.md
var MyCommandDescription string
```

**Step 3: Use it in your command**

```go
// cmd/mycommand/mycommand.go
package mycommand

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/markdown"
    "github.com/cloudposse/atmos/pkg/utils"
)

var mycommandCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Brief description",
    RunE: func(cmd *cobra.Command, args []string) error {
        utils.PrintfMarkdown("%s", markdown.MyCommandDescription)
        return nil
    },
}
```

**Why This Pattern?**

- ✅ **Single source of truth** - All markdown in one location
- ✅ **No duplicate files** - Avoids copying markdown into each package
- ✅ **Works with Go embed** - Subpackages can't use `..` in embed paths
- ✅ **Easy to maintain** - Update markdown in one place

---

## Integration with Custom Commands

The registry pattern coexists with custom commands from `atmos.yaml`:

**Execution Order:**
1. Built-in commands register (via registry)
2. Custom commands load (from `atmos.yaml`)
3. Custom commands can override built-in commands

**Example custom command override:**

```yaml
# atmos.yaml
commands:
  - name: terraform  # Overrides built-in terraform command
    description: Custom terraform wrapper
    steps:
      - atmos validate component {{ .arguments.component }} -s {{ .arguments.stack }}
      - terraform {{ .arguments.subcommand }}
```

---

## Checklist for New Commands

- [ ] Create package directory: `cmd/[command]/`
- [ ] Implement command with RunE function
- [ ] Implement CommandProvider interface (including `GetAliases()`)
- [ ] Register with `internal.Register()`
- [ ] Add blank import to `cmd/root.go`
- [ ] Add business logic to `internal/exec/`
- [ ] Add command aliases if needed (see "Adding Command Aliases" section)
- [ ] Write unit tests
- [ ] Write integration tests (in `tests/`)
- [ ] Test aliases if added (verify both source and alias paths work)
- [ ] Add Docusaurus documentation in `website/docs/cli/commands/`
- [ ] Build website to verify docs: `cd website && npm run build`
- [ ] Test with custom commands from `atmos.yaml`
- [ ] Update this guide if introducing new patterns

---

## Testing Your Command

```bash
# Build Atmos
go build -o build/atmos .

# Test your command
./build/atmos mycommand --help
./build/atmos mycommand [args]

# Run tests
go test ./cmd/mycommand/...
go test ./internal/exec/...

# Run integration tests
go test ./tests/...
```

---

## Common Issues

### Issue: Command not showing in help

**Solution:** Ensure blank import in `cmd/root.go`:

```go
import (
    _ "github.com/cloudposse/atmos/cmd/mycommand"
)
```

### Issue: Duplicate command registration

**Solution:** Check that old command file doesn't call `RootCmd.AddCommand()`. Mark it deprecated or delete it.

### Issue: Subcommand not found

**Solution:** Ensure subcommand is attached in parent's `init()`:

```go
func init() {
    parentCmd.AddCommand(subCmd)
}
```

### Issue: Import cycle

**Solution:**
- Don't import `cmd` package from other packages
- Move shared logic to `internal/exec/` or `pkg/`
- Export nested parent commands (uppercase) for cross-package imports

---

## Examples

See these commands for reference:

- **Simple command**: `cmd/about/`
- **Static subcommands**: `cmd/describe/`
- **Dynamic subcommands**: `cmd/terraform/`
- **Deeply nested**: `cmd/aws/`

---

## Interactive Prompts (Recommended)

**We recommend using interactive prompts** to make commands more user-friendly. Similar to shell autocomplete, prompts help users discover available options without memorizing values or checking documentation.

Atmos provides built-in interactive selection menus for missing required flags and positional arguments using the Charmbracelet Huh library. This creates a better user experience by:

- **Reducing cognitive load** - Users don't need to remember exact names
- **Preventing typos** - Selection from a list eliminates spelling errors
- **Improving discoverability** - Users see what options are available
- **Graceful degradation** - Automatically disabled in CI/non-TTY environments

Prompts automatically appear when:

1. **TTY is available** (stdin is a terminal)
2. **Not running in CI** (detected automatically)
3. **`--interactive` flag is true** (default: `true`, can be disabled with `--interactive=false` or `ATMOS_INTERACTIVE=false`)

When disabled or unavailable, commands fall back to standard validation errors with helpful hints.

### Use Case 1: Missing Required Positional Arguments

When a command has a required positional argument with completion options, you can configure an interactive prompt:

```go
// cmd/theme/show.go
func init() {
    // Build positional args with prompt
    builder := flags.NewPositionalArgsBuilder()
    builder.AddArg(&flags.PositionalArgSpec{
        Name:           "theme-name",
        Description:    "Theme name to preview",
        Required:       true,
        TargetField:    "ThemeName",
        CompletionFunc: ThemesArgCompletion,  // Provides options
    })
    specs, validator, usage := builder.Build()

    // Create parser with interactive prompt
    themeShowParser = flags.NewStandardFlagParser(
        flags.WithPositionalArgPrompt(
            "theme-name",                      // Arg name
            "Choose a theme to preview",       // Prompt title
            ThemesArgCompletion,               // Completion function
        ),
    )

    // Set positional args
    themeShowParser.SetPositionalArgs(specs, validator, usage)
    themeShowCmd.Use = "show " + usage

    // Register flags (sets prompt-aware validator)
    themeShowParser.RegisterFlags(themeShowCmd)
}
```

**Behavior:**
- `atmos theme show` → Shows interactive selector
- `atmos theme show dracula` → Uses provided value directly
- `atmos theme show --interactive=false` → Shows error if missing

### Use Case 2: Optional Value Flags (Sentinel Pattern)

For flags like `--identity` that can be provided with or without a value:

```go
parser := flags.NewStandardFlagParser(
    flags.WithStringFlag("identity", "i", "", "Identity name (use --identity to select interactively)"),
    flags.WithNoOptDefVal("identity", cfg.IdentityFlagSelectValue),  // Sentinel value
    flags.WithOptionalValuePrompt(
        "identity",
        "Choose an identity",
        IdentitiesCompletion,  // Function that returns available identities
    ),
)
```

**Behavior:**
- `atmos cmd --identity` → Shows interactive selector (flag set to sentinel)
- `atmos cmd --identity=admin` → Uses "admin" directly
- `atmos cmd` → Uses empty/default value

### Use Case 3: Missing Required Flags

For required flags with completion options:

```go
parser := flags.NewStandardFlagParser(
    flags.WithStackFlag(true),  // Required
    flags.WithCompletionPrompt(
        "stack",
        "Choose a stack",
        StacksCompletion,  // Function that returns available stacks
    ),
)
```

**Behavior:**
- `atmos cmd` → Shows interactive selector if no stack provided
- `atmos cmd --stack=prod` → Uses provided value directly
- `ATMOS_STACK=prod atmos cmd` → Uses environment variable

### Completion Functions

Completion functions must match the `CompletionFunc` signature:

```go
type CompletionFunc func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)
```

**Example:**
```go
func ThemesArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    themes := theme.GetRegisteredThemeNames()
    return themes, cobra.ShellCompDirectiveNoFileComp
}
```

### Disabling Interactive Mode

Users can disable interactive prompts:

**Via flag:**
```bash
atmos theme show --interactive=false
```

**Via environment variable:**
```bash
ATMOS_INTERACTIVE=false atmos theme show
```

**In configuration:**
```yaml
# atmos.yaml
interactive: false
```

### Error Handling

When users abort a prompt (Ctrl-C), the error is propagated:

```bash
$ atmos theme show
? Choose a theme to preview
^C
# Error: prompt failed: user aborted
```

When not interactive (CI, piped, disabled), standard validation errors appear:

```bash
$ echo "" | atmos theme show
# Error: invalid positional arguments
## Explanation
Theme name is required
## Hints
💡 Run `atmos list themes` to see all available themes
💡 Browse themes at https://atmos.tools/cli/commands/theme/browse
```

### Best Practices

1. **Always provide completion functions** - Prompts need options to display
2. **Use descriptive prompt titles** - "Choose a theme to preview" not "Select theme"
3. **Handle empty results gracefully** - If completion returns no options, prompt is skipped
4. **Test non-interactive scenarios** - Ensure commands work in CI/pipelines
5. **Document the feature** - Update command help text to mention interactive selection

### Example Commands

See these commands for reference implementations:

- **Positional args**: `cmd/theme/show.go` - Interactive theme selection
- **Optional value flags**: Auth commands with `--identity` flag
- **Required flags**: Commands using `WithCompletionPrompt`

---

## Further Reading

- [I/O and UI Output Guide](io-and-ui-output.md) - **How to handle output in commands**
- [Flag Handler Documentation](.claude/agents/flag-handler.md) - **Complete flag handler guide**
- [Command Registry Pattern PRD](prd/command-registry-pattern.md)
- [Cobra Documentation](https://github.com/spf13/cobra)
- [Atmos Custom Commands](/core-concepts/custom-commands)
