# PRD: Type-Safe Positional Arguments System

## Problem Statement

Currently, Atmos commands that accept positional arguments (e.g., `atmos terraform deploy component1`) suffer from several issues:

1. **Error-Prone Manual Configuration**: Developers must manually:
   - Set `cmd.Args = cobra.ArbitraryArgs` (or forget and break validation)
   - Update `cmd.Use` string to show expected arguments (or leave it incorrect)
   - Write validation logic in `RunE` to check argument count (or omit it)
   - Extract positional args from the `args []string` slice (error-prone indexing)

2. **Regression: Unknown Command Errors**: When `DisableFlagParsing=true`, Cobra bypasses argument validation. The custom `UsageFunc` then treats positional arguments as unknown subcommands, causing errors like:
   ```
   Unknown command component1 for atmos terraform deploy
   ```

3. **Inconsistent Implementation**: Different commands handle positional args differently:
   - Some use `cobra.ArbitraryArgs`
   - Some use `cobra.MinimumNArgs(1)`
   - Some forget to validate at all
   - Some have incorrect `Use` strings

4. **No Type Safety**: Positional arguments are extracted via array indexing (`args[0]`), with no compile-time guarantees.

## Requirements

### Functional Requirements

**FR1: Type-Safe Positional Argument Definition**
- Commands MUST be able to declare expected positional arguments using a strongly-typed builder pattern
- The system MUST prevent runtime errors from incorrect positional argument access
- Positional argument names MUST be semantic and domain-specific (e.g., `component`, `workflow-name`)

**FR2: Automatic Cobra Configuration**
- The builder MUST automatically set `cmd.Args` validator based on positional argument specifications
- The builder MUST automatically generate the correct `cmd.Use` suffix showing expected arguments
- Developers MUST NOT manually configure `cmd.Args` or update `cmd.Use` strings

**FR3: Support for Required and Optional Arguments**
- The system MUST support required positional arguments (e.g., `<component>`)
- The system MUST support optional positional arguments (e.g., `[component]`)
- Required arguments MUST be validated at runtime before command execution

**FR4: Domain-Specific Builders**
- Terraform commands MUST use `TerraformPositionalArgsBuilder` with methods like `WithComponent(required bool)`
- Helmfile commands MUST use `HelmfilePositionalArgsBuilder` with methods like `WithComponent(required bool)`
- Workflow commands MUST use `WorkflowPositionalArgsBuilder` with methods like `WithWorkflowName(required bool)`
- Each domain builder MUST provide semantic method names specific to that domain

**FR5: Parser Compatibility**
- The system MUST work with `AtmosFlagParser` (used by terraform commands with compatibility flags)
- The system MUST work with `StandardParser` (used by helmfile, packer, workflow, validate, generate commands)
- The positional args API MUST be identical regardless of underlying parser type

**Note**: `PassThroughFlagParser` was deleted on 2025-11-06. Terraform now uses `AtmosFlagParser` with compatibility translator.

**FR6: Struct-Based Storage (Consistent with Flag Parser)**
- Positional arguments MUST be stored as fields in Options structs (e.g., `TerraformOptions.Component`)
- Fields MUST use primitive types: `string`, `bool`, `int` (same as flags)
- Access MUST use dot notation: `opts.Component`, NOT `GetComponent(args)`
- The pattern MUST match existing flag parser (e.g., `opts.Stack`, `opts.DryRun`)

**FR7: No Manual Array Indexing**
- Extraction MUST NOT require manual array indexing (no `args[0]`)
- Positional args MUST be parsed into struct fields automatically
- Invalid positional arguments MUST be detected before struct population

**FR8: Fix UsageFunc Regression**
- The custom `UsageFunc` MUST respect `cmd.Args` validators from builders
- Positional arguments MUST NOT be treated as unknown subcommands
- The error "Unknown command component1 for atmos terraform deploy" MUST be eliminated

### Non-Functional Requirements

**NFR1: Maintainability**
- The implementation MUST follow the same patterns as the existing flag parser system
- Code MUST be self-documenting through semantic method names
- All builder methods MUST include comprehensive documentation

**NFR2: Testability**
- All builder methods MUST be independently unit testable
- Generated validators MUST be testable in isolation
- Usage string generation MUST be verifiable through tests
- Test coverage MUST exceed 80%

**NFR3: Performance**
- Positional argument validation MUST occur before command execution (fail-fast)
- The system MUST NOT introduce measurable performance overhead
- Builder pattern MUST NOT require reflection or runtime code generation

**NFR4: Backward Compatibility**
- Existing command behavior MUST NOT change
- Existing positional argument extraction patterns MUST continue to work during migration
- Migration MUST be incremental (command-by-command)

**NFR5: Consistency**
- All terraform commands accepting components MUST use identical builder patterns
- All helmfile commands accepting components MUST use identical builder patterns
- All workflow commands accepting workflow names MUST use identical builder patterns

### Constraints

**C1: No New Positional Arguments**
- This system only formalizes EXISTING positional arguments
- No new positional arguments will be added as part of this work
- Focus is on current usage: `<component>` and `<workflow-name>`

**C2: Cobra Compatibility**
- The system MUST work within Cobra's existing framework
- No forking or patching of Cobra library
- Must use standard Cobra `cmd.Args` validators

**C3: DisableFlagParsing Requirement**
- The system MUST work when `DisableFlagParsing=true` is set
- This is required for manual flag parsing with Viper precedence

## Current State

### Commands Using Positional Arguments

**Terraform commands** (use AtmosFlagParser with compatibility flags):
- `terraform plan <component>`
- `terraform apply <component>`
- `terraform deploy <component>`
- `terraform destroy <component>`
- `terraform workspace <component>`
- `terraform shell <component>`
- All other native terraform commands that operate on components

**Helmfile commands** (use StandardParser):
- `helmfile apply <component>`
- `helmfile destroy <component>`
- `helmfile diff <component>`
- `helmfile sync <component>`

**Workflow commands** (use StandardParser):
- `workflow <workflow-name>`

**Validate commands** (use StandardParser):
- `validate component <component>`

**Generate commands** (use StandardParser):
- `terraform generate varfile <component>`
- `terraform generate backend <component>`
- `terraform generate planfile <component>`

**Migration Complete (2025-11-06)**: PassThroughFlagParser deleted. All commands now use either `AtmosFlagParser` (terraform) or `StandardParser` (everything else).

### Example of Current Error-Prone Pattern

```go
// BEFORE: Manual, error-prone configuration
{
    Use:   "deploy",  // WRONG: Doesn't show <component>
    Short: "Deploy infrastructure",
    Args:  cobra.ArbitraryArgs,  // WRONG: Too permissive
    RunE: func(cmd *cobra.Command, args []string) error {
        // WRONG: Manual validation, easy to forget
        if len(args) == 0 {
            return errors.New("component required")
        }
        component := args[0]  // WRONG: No type safety
        // ...
    },
}
```

## Proposed Solution

### Builder Pattern for Positional Arguments

Apply the same **builder pattern philosophy** used for flags to positional arguments:

1. **Domain-Specific Builders**: Each command type (Terraform, Helmfile, Workflow) gets its own builder
2. **Semantic Method Names**: `WithComponent(required)` instead of `WithRequiredArg("component", "...")`
3. **Auto-Configuration**: Builders automatically set `cmd.Args` and generate `cmd.Use` suffix
4. **Type Safety**: Strongly-typed extraction methods
5. **Testability**: Each builder method is independently testable

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  PositionalArgsBuilder                      │
│              (Low-level, generic builder)                   │
│  - WithRequiredArg(name, desc)                             │
│  - WithOptionalArg(name, desc)                             │
│  - WithValidatedArg(name, desc, validator)                 │
│  - Build() → (parser, cobraValidator, usageString)         │
└─────────────────────────────────────────────────────────────┘
                           ▲
                           │ delegates to
                           │
    ┌──────────────────────┴──────────────────────┐
    │                                              │
┌───▼──────────────────┐           ┌──────────────▼──────────┐
│ TerraformPositional  │           │ WorkflowPositional      │
│ ArgsBuilder          │           │ ArgsBuilder             │
│                      │           │                         │
│ - WithComponent(req) │           │ - WithWorkflowName(req) │
│ - WithOriginalPlan() │           │                         │
│ - WithNewPlan()      │           │                         │
└──────────────────────┘           └─────────────────────────┘
```

### Implementation

#### 0. Update TerraformOptions to Include Component Field

```go
// pkg/flags/terraform_options.go

type TerraformOptions struct {
    GlobalFlags // Embedded global flags (chdir, logs-level, identity, etc.)

    // Common flags (shared with Helmfile, Packer).
    Stack    string // --stack/-s: Target stack name.
    Identity IdentitySelector
    DryRun   bool // --dry-run: Perform dry run without making actual changes.

    // NEW: Positional argument - component name
    // Populated automatically by parser when component is provided as positional arg
    Component string // Component name from positional arg (e.g., "vpc" in: atmos terraform plan vpc)

    // Terraform-specific flags.
    UploadStatus bool   // --upload-status: Upload plan status to Atmos Pro.
    SkipInit     bool   // --skip-init: Skip terraform init before running command.
    FromPlan     string // --from-plan: Apply from previously generated plan file.

    // Positional and pass-through arguments.
    positionalArgs  []string // e.g., ["plan", "vpc"] in: atmos terraform plan vpc
    passThroughArgs []string // e.g., ["-var", "foo=bar"] in: atmos terraform plan -- -var foo=bar
}
```

#### 1. Update WorkflowOptions to Include WorkflowName Field

```go
// pkg/flags/workflow_options.go

type WorkflowOptions struct {
    StandardOptions // Embedded standard flags (stack, file, dry-run, identity, etc.)

    // NEW: Positional argument - workflow name
    // Populated automatically by parser when workflow name is provided as positional arg
    WorkflowName string // Workflow name from positional arg (e.g., "deploy-all" in: atmos workflow deploy-all)

    // FromStep specifies the step to resume workflow execution from.
    FromStep string
}
```

#### 2. Generic Low-Level Builder

```go
// pkg/flags/positional_args.go

// PositionalArgsBuilder provides low-level positional argument building.
type PositionalArgsBuilder struct {
    args []PositionalArgSpec
}

type PositionalArgSpec struct {
    Name        string
    Description string
    Required    bool
    TargetField string // Field name in Options struct to populate (e.g., "Component")
    Validator   func(string) error
}

func NewPositionalArgsBuilder() *PositionalArgsBuilder

func (b *PositionalArgsBuilder) WithRequiredArg(name, description, targetField string) *PositionalArgsBuilder
func (b *PositionalArgsBuilder) WithOptionalArg(name, description, targetField string) *PositionalArgsBuilder

// Build returns:
//  1. List of arg specs (used by parser to populate struct fields)
//  2. Cobra validator function (sets cmd.Args automatically)
//  3. Usage string suffix (updates cmd.Use automatically)
func (b *PositionalArgsBuilder) Build() ([]PositionalArgSpec, cobra.PositionalArgs, string)
```

#### 3. Domain-Specific Terraform Builder

```go
// pkg/flags/terraform_positional.go

// TerraformPositionalArgsBuilder provides domain-specific methods for Terraform commands.
type TerraformPositionalArgsBuilder struct {
    inner *PositionalArgsBuilder
}

func NewTerraformPositionalArgsBuilder() *TerraformPositionalArgsBuilder

// WithComponent adds the component positional argument.
// Automatically populates TerraformOptions.Component field.
//
// Example:
//   - WithComponent(true)  → "deploy <component>" → opts.Component = "vpc"
//   - WithComponent(false) → "deploy [component]" → opts.Component = "" (if not provided)
func (b *TerraformPositionalArgsBuilder) WithComponent(required bool) *TerraformPositionalArgsBuilder {
    if required {
        b.inner.WithRequiredArg("component", "Atmos component name", "Component")
    } else {
        b.inner.WithOptionalArg("component", "Atmos component name", "Component")
    }
    return b
}

// Build delegates to the inner builder.
// Returns arg specs that TerraformParser uses to populate TerraformOptions.Component.
func (b *TerraformPositionalArgsBuilder) Build() ([]PositionalArgSpec, cobra.PositionalArgs, string) {
    return b.inner.Build()
}
```

#### 4. Domain-Specific Workflow Builder

```go
// pkg/flags/workflow_positional.go

type WorkflowPositionalArgsBuilder struct {
    inner *PositionalArgsBuilder
}

func NewWorkflowPositionalArgsBuilder() *WorkflowPositionalArgsBuilder

// WithWorkflowName adds the workflow-name positional argument.
// Automatically populates WorkflowOptions.WorkflowName field.
//
// Example:
//   - WithWorkflowName(true) → "workflow <workflow-name>" → opts.WorkflowName = "deploy-all"
func (b *WorkflowPositionalArgsBuilder) WithWorkflowName(required bool) *WorkflowPositionalArgsBuilder {
    if required {
        b.inner.WithRequiredArg("workflow-name", "Workflow name to execute", "WorkflowName")
    } else {
        b.inner.WithOptionalArg("workflow-name", "Workflow name to execute", "WorkflowName")
    }
    return b
}

// Build delegates to the inner builder.
// Returns arg specs that WorkflowParser uses to populate WorkflowOptions.WorkflowName.
func (b *WorkflowPositionalArgsBuilder) Build() ([]PositionalArgSpec, cobra.PositionalArgs, string) {
    return b.inner.Build()
}
```

#### 4. Domain-Specific Helmfile Builder

```go
// pkg/flags/helmfile_positional.go

type HelmfilePositionalArgsBuilder struct {
    inner *PositionalArgsBuilder
}

func NewHelmfilePositionalArgsBuilder() *HelmfilePositionalArgsBuilder

// WithComponent adds the component positional argument for Helmfile commands.
func (b *HelmfilePositionalArgsBuilder) WithComponent(required bool) *HelmfilePositionalArgsBuilder

func (b *HelmfilePositionalArgsBuilder) Build() (*PositionalArgsParser, cobra.PositionalArgs, string)
```

### Usage Examples

#### Terraform Deploy Command

```go
// In cmd/terraform_commands.go

// Define positional args specs for deploy command
_, deployValidator, deployUsage := flags.NewTerraformPositionalArgsBuilder().
    WithComponent(true).  // Component is required, populates TerraformOptions.Component
    Build()

commands := []*cobra.Command{
    {
        Use:   "deploy " + deployUsage,  // Auto-generates: "deploy <component>"
        Short: "Deploy the specified infrastructure using Terraform",
        Args:  deployValidator,           // Auto-configured validator
        RunE: func(cmd *cobra.Command, args []string) error {
            handleHelpRequest(cmd, args)

            // Parse flags AND positional args
            // Parser automatically populates opts.Component from positional arg
            ctx := cmd.Context()
            opts, err := terraformParser.Parse(ctx, args)
            if err != nil {
                return err
            }

            // Access component via dot notation - just like flags!
            // opts.Component is populated from positional arg "vpc" in: atmos terraform deploy vpc
            fmt.Printf("Deploying component: %s\n", opts.Component)
            fmt.Printf("In stack: %s\n", opts.Stack)

            return terraformRun(parentCmd, cmd, opts)
        },
    },
}
```

#### Workflow Command

```go
// In cmd/workflow.go

_, workflowValidator, workflowUsage := flags.NewWorkflowPositionalArgsBuilder().
    WithWorkflowName(true).
    Build()

var workflowCmd = &cobra.Command{
    Use:   "workflow " + workflowUsage,  // "workflow <workflow-name>"
    Short: "Execute an Atmos workflow",
    Args:  workflowValidator,
    RunE: func(cmd *cobra.Command, args []string) error {
        // Parse flags AND positional args
        ctx := cmd.Context()
        opts, err := workflowParser.Parse(ctx, args)
        if err != nil {
            return err
        }

        // Access workflow name via dot notation - just like flags!
        // opts.WorkflowName is populated from positional arg "deploy-all" in: atmos workflow deploy-all
        fmt.Printf("Executing workflow: %s\n", opts.WorkflowName)

        return executeWorkflow(cmd, opts)
    },
}
```

#### Helmfile Apply Command

```go
// In cmd/helmfile_apply.go

_, applyValidator, applyUsage := flags.NewHelmfilePositionalArgsBuilder().
    WithComponent(true).
    Build()

var helmfileApplyCmd = &cobra.Command{
    Use:   "apply " + applyUsage,  // "apply <component>"
    Short: "Apply Helmfile releases",
    Args:  applyValidator,
    RunE: func(cmd *cobra.Command, args []string) error {
        // Parse flags AND positional args
        ctx := cmd.Context()
        opts, err := helmfileParser.Parse(ctx, args)
        if err != nil {
            return err
        }

        // Access component via dot notation
        // opts.Component is populated from positional arg
        return helmfileRun(cmd, "apply", opts)
    },
}
```

### Integration with AtmosFlagParser and StandardParser

**Key Requirement**: The positional args system must work with BOTH parser types:

1. **AtmosFlagParser** (terraform commands only)
   - Handles compatibility flags (-s → --stack, -i → --identity)
   - Uses `CompatibilityFlagsTranslator` before parsing
   - Positional args extracted from processed `args []string`

2. **StandardParser** (helmfile, packer, workflow, validate, describe, generate commands)
   - Standard flag parsing without compatibility layer
   - Positional args extracted similarly

**Solution**: Both parsers provide `GetPositionalArgs()` method that returns positional arguments AFTER flag parsing. The PositionalArgsParser.Parse() method accepts this cleaned args slice.

**Note**: `PassThroughFlagParser` was deleted on 2025-11-06.

```go
// Works with both AtmosFlagParser and StandardParser

// In RunE:
ctx := cmd.Context()
opts, err := parser.Parse(ctx, args)  // parser can be TerraformParser, HelmfileParser, etc.
if err != nil {
    return err
}

// Extract positional args from parsed options
positionalArgs := opts.GetPositionalArgs()

// Parse positional args using builder-generated parser
component, err := positionalArgsParser.GetComponent(positionalArgs)
if err != nil {
    return err
}
```

### UsageFunc Fix

Update the custom `UsageFunc` in `cmd/root.go` to respect `cmd.Args` validator:

```go
// In RootCmd.SetUsageFunc (line 743-756 in cmd/root.go):
RootCmd.SetUsageFunc(func(c *cobra.Command) error {
    if c.Use == "atmos" {
        return b.UsageFunc(c)
    }

    // When DisableFlagParsing=true, c.Flags().Args() returns empty.
    // Fall back to os.Args to get the actual arguments passed to the command.
    arguments := c.Flags().Args()
    if len(arguments) == 0 && c.DisableFlagParsing {
        // Extract args from os.Args based on command path depth
        arguments = os.Args[len(strings.Split(c.CommandPath(), " ")):]
    }

    // NEW: Check if command has Args validator from builder
    if c.Args != nil {
        // Validate args using the builder-generated validator
        if err := c.Args(c, arguments); err == nil {
            // Args are valid according to validator, this is a usage request not an unknown command
            showErrorExampleFromMarkdown(c, "")
            return nil
        }
        // If Args validator fails, fall through to showUsageAndExit
    }

    showUsageAndExit(c, arguments)
    return nil
})
```

## Benefits

1. **Zero Manual Configuration**
   - `cmd.Args` is set automatically by builder
   - `cmd.Use` suffix is generated automatically
   - No more forgetting to validate arguments

2. **Type Safety**
   - Compile-time guarantee that positional args are defined
   - `GetComponent()` returns string, not `args[0]` indexing
   - Can't access undefined positional arguments

3. **Self-Documenting**
   - `WithComponent(true)` is clearer than `WithRequiredArg("component", "Atmos component name")`
   - Method names encode semantic meaning
   - Consistent naming across all commands

4. **Testability**
   - Each builder method can be unit tested independently
   - Generated validators can be tested in isolation
   - Usage string generation can be verified

5. **Consistent Implementation**
   - All terraform commands use same pattern
   - All helmfile commands use same pattern
   - All workflow commands use same pattern
   - No more inconsistent `Use` strings or validators

6. **Works with Both Parser Types**
   - Integrates seamlessly with `AtmosFlagParser` (terraform with compatibility flags)
   - Integrates seamlessly with `StandardParser` (all other commands)
   - Same API regardless of underlying parser

**Migration Note**: `PassThroughFlagParser` was deleted on 2025-11-06.

## Testing Strategy

### Unit Tests

```go
// pkg/flags/positional_args_test.go

func TestPositionalArgsBuilder_SingleRequired(t *testing.T) {
    parser, validator, usage := NewPositionalArgsBuilder().
        WithRequiredArg("component", "Component name").
        Build()

    assert.Equal(t, "<component>", usage)
    assert.NoError(t, validator(nil, []string{"vpc"}))
    assert.Error(t, validator(nil, []string{}))

    parsed, err := parser.Parse([]string{"vpc"})
    assert.NoError(t, err)
    assert.Equal(t, "vpc", parsed["component"])
}

func TestTerraformPositionalArgsBuilder_WithComponent_Required(t *testing.T) {
    _, validator, usage := NewTerraformPositionalArgsBuilder().
        WithComponent(true).
        Build()

    assert.Equal(t, "<component>", usage)
    assert.NoError(t, validator(nil, []string{"vpc"}))
    assert.Error(t, validator(nil, []string{}))
}

func TestTerraformPositionalArgsBuilder_WithComponent_Optional(t *testing.T) {
    _, validator, usage := NewTerraformPositionalArgsBuilder().
        WithComponent(false).
        Build()

    assert.Equal(t, "[component]", usage)
    assert.NoError(t, validator(nil, []string{}))  // Empty OK for optional
    assert.NoError(t, validator(nil, []string{"vpc"}))
}
```

### Integration Tests

```go
// tests/test-cases/terraform-deploy-positional-args.yaml

name: "terraform deploy with positional component arg"
command: "terraform deploy component1 -s stack1"
expected:
  exit_code: 0
  # Should not show "Unknown command component1"

---
name: "terraform deploy missing component arg"
command: "terraform deploy -s stack1"
expected:
  exit_code: 1
  stderr_contains: "missing required argument: component"

---
name: "workflow with positional workflow-name arg"
command: "workflow deploy-all -s stack1"
expected:
  exit_code: 0
```

## Implementation Plan

### Phase 1: Core Infrastructure (Week 1)
1. ✅ Create `pkg/flags/positional_args.go` with low-level builder
2. ✅ Implement `PositionalArgsBuilder` with `WithRequiredArg`, `WithOptionalArg`
3. ✅ Implement `PositionalArgsParser` with `Parse()` method
4. ✅ Add comprehensive unit tests

### Phase 2: Domain-Specific Builders (Week 1)
1. ✅ Create `pkg/flags/terraform_positional.go` with `TerraformPositionalArgsBuilder`
2. ✅ Create `pkg/flags/workflow_positional.go` with `WorkflowPositionalArgsBuilder`
3. ✅ Create `pkg/flags/helmfile_positional.go` with `HelmfilePositionalArgsBuilder`
4. ✅ Add unit tests for each domain builder

### Phase 3: Terraform Commands Integration (Week 2)
1. ✅ Update `cmd/terraform_commands.go` to use `TerraformPositionalArgsBuilder`
2. ✅ Update all terraform subcommands (plan, apply, deploy, destroy, etc.)
3. ✅ Add integration tests for terraform commands
4. ✅ Verify `ComponentsArgCompletion` still works

### Phase 4: Helmfile Commands Integration (Week 2)
1. ✅ Update `cmd/helmfile_apply.go`, `helmfile_destroy.go`, etc.
2. ✅ Add integration tests for helmfile commands

### Phase 5: Workflow Commands Integration (Week 2)
1. ✅ Update `cmd/workflow.go` to use `WorkflowPositionalArgsBuilder`
2. ✅ Add integration tests for workflow commands

### Phase 6: UsageFunc Fix (Week 3)
1. ✅ Update `cmd/root.go` UsageFunc to respect `cmd.Args` validator
2. ✅ Add regression tests for "Unknown command" error
3. ✅ Verify all commands show correct usage

### Phase 7: Documentation (Week 3)
1. ✅ Update `docs/developing-atmos-commands.md`
2. ✅ Add examples to CLAUDE.md
3. ✅ Update command documentation with correct `Use` strings

## Success Criteria

1. **All terraform commands** use `TerraformPositionalArgsBuilder`
2. **All helmfile commands** use `HelmfilePositionalArgsBuilder`
3. **All workflow commands** use `WorkflowPositionalArgsBuilder`
4. **Zero manual `cmd.Args` configuration** in command definitions
5. **Zero manual `cmd.Use` suffix generation** in command definitions
6. **All positional args** extracted via type-safe methods
7. **Integration tests pass** for all commands with positional args
8. **No regression** of "Unknown command component1" error
9. **Test coverage** >80% for positional args builder package

## Non-Goals

1. **Not replacing flags system**: This only handles positional arguments
2. **Not changing command structure**: Commands still use same RunE pattern
3. **Not adding new positional arguments**: Only formalizing existing ones
4. **Not supporting variadic args**: Focus on fixed positional args (component, workflow-name)

## Related Work

- **Flag Parser System**: `pkg/flags/standard_builder.go`, `pkg/flags/terraform_builder.go`
- **AtmosFlagParser**: `pkg/flags/terraform/parser.go` (terraform with compatibility flags)
- **StandardParser**: `pkg/flags/standard.go` (all other commands)
- **CompatibilityFlagsTranslator**: `pkg/flags/compatibility_flags.go` (handles -s → --stack)
- **Command Registry Pattern**: `docs/prd/command-registry-pattern.md`

**Note**: `PassThroughFlagParser` deleted on 2025-11-06.

## References

- Original error: "Unknown command component1 for atmos terraform deploy"
- Current manual pattern: `cmd/terraform_commands.go:308` (`cmd.Args = cobra.ArbitraryArgs`)
- UsageFunc logic: `cmd/root.go:743-756`
- Workflow positional args: `internal/exec/workflow.go:41-55`
- Validate component positional args: `internal/exec/validate_component.go:29-34`
