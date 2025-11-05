# List Settings Positional Args - TODO

**Issue Identified**: 2025-11-05
**Status**: Deferred (requires StandardOptionsBuilder enhancement)

## Problem

The `list settings` command currently does NOT use the builder pattern for its optional `[component]` positional argument. It manually extracts the component from the raw `args[]` array:

```go
// cmd/list_settings.go - CURRENT (anti-pattern)
RunE: func(cmd *cobra.Command, args []string) error {
    opts, err := listSettingsParser.Parse(cmd.Context(), args)
    if err != nil {
        return err
    }

    // Validate maximum 1 positional arg (optional component).
    positionalArgs := opts.GetPositionalArgs()
    if len(positionalArgs) > 1 {
        return fmt.Errorf("invalid arguments. The command accepts at most one argument (component)")
    }

    // Manual extraction from args array
    componentFilter := ""
    if len(args) > 0 {
        componentFilter = args[0]
    }

    output, err := listSettings(cmd, componentFilter)
    //...
}
```

## Desired Pattern

Commands like `terraform`, `packer`, and `helmfile` use PositionalArgsBuilder for type-safe, validated positional argument extraction:

```go
// cmd/terraform_plan.go - DESIRED PATTERN
var terraformPlanParser = flags.NewPassThroughOptionsBuilder().
    WithTerraformFlags().
    WithPositionalArgs(flags.NewTerraformPositionalArgsBuilder().
        WithSubcommand(true).
        WithComponent(true).
        Build()).
    Build()

RunE: func(cmd *cobra.Command, args []string) error {
    opts, err := terraformPlanParser.Parse(cmd.Context(), args)
    if err != nil {
        return err
    }

    // Type-safe access to positional args
    subcommand := opts.Subcommand
    component := opts.Component
    //...
}
```

## Root Cause

**StandardOptionsBuilder does not support WithPositionalArgs() method.**

PassThroughOptionsBuilder has this:
```go
// pkg/flags/passthrough_builder.go
type PassThroughOptionsBuilder struct {
    positionalArgs ([]*PositionalArgSpec, cobra.PositionalArgs, string)
}

func (b *PassThroughOptionsBuilder) WithPositionalArgs(specs []*PositionalArgSpec, ...) {
    b.positionalArgs = (specs, validator, usage)
    return b
}
```

StandardOptionsBuilder does NOT have this capability.

## Solution Design

### Phase 1: Add WithPositionalArgs to StandardOptionsBuilder

```go
// pkg/flags/standard_builder.go
type StandardOptionsBuilder struct {
    options        []Option
    positionalArgs ([]*PositionalArgSpec, cobra.PositionalArgs, string)  // ADD THIS
}

// WithPositionalArgs configures positional argument validation.
//
// Parameters:
//   - specs: Positional argument specifications with TargetField mapping
//   - validator: Cobra Args validator function
//   - usage: Usage string (e.g., "[component]")
//
// Example:
//
//	builder := flags.NewStandardOptionsBuilder().
//	    WithPositionalArgs(flags.NewListSettingsPositionalArgsBuilder().
//	        WithComponent(false).  // Optional component arg
//	        Build()).
//	    Build()
func (b *StandardOptionsBuilder) WithPositionalArgs(
    specs []*PositionalArgSpec,
    validator cobra.PositionalArgs,
    usage string,
) *StandardOptionsBuilder {
    b.positionalArgs = (specs, validator, usage)
    return b
}

// Build() must be updated to pass positionalArgs to StandardParser
func (b *StandardOptionsBuilder) Build() *StandardParser {
    // Pass positional args config to parser
    parser := NewStandardParser(b.options...)
    if b.positionalArgs != nil {
        parser.SetPositionalArgs(b.positionalArgs)
    }
    return parser
}
```

### Phase 2: Add SetPositionalArgs to StandardParser

```go
// pkg/flags/standard.go
type StandardParser struct {
    registry       *FlagRegistry
    cmd            *cobra.Command
    viper          *viper.Viper
    positionalArgs ([]*PositionalArgSpec, cobra.PositionalArgs, string)  // ADD THIS
}

// SetPositionalArgs configures positional argument extraction.
func (p *StandardParser) SetPositionalArgs(
    specs []*PositionalArgSpec,
    validator cobra.PositionalArgs,
    usage string,
) {
    p.positionalArgs = (specs, validator, usage)
}

// Parse() must be updated to extract and validate positional args
func (p *StandardParser) Parse(ctx context.Context, args []string) (*StandardOptions, error) {
    // ... existing flag parsing ...

    // Extract positional args if configured
    if p.positionalArgs != nil {
        specs, validator, _ := p.positionalArgs

        // Validate using Cobra validator
        if err := validator(nil, positionalArgs); err != nil {
            return nil, err
        }

        // Extract into opts using TargetField mapping
        for i, spec := range specs {
            if i < len(positionalArgs) {
                // Use reflection to set field by name
                setField(opts, spec.TargetField, positionalArgs[i])
            }
        }
    }

    return opts, nil
}
```

### Phase 3: Create ListSettingsPositionalArgsBuilder

```go
// pkg/flags/list_settings_positional_args_builder.go
type ListSettingsPositionalArgsBuilder struct {
    builder *PositionalArgsBuilder
}

func NewListSettingsPositionalArgsBuilder() *ListSettingsPositionalArgsBuilder {
    return &ListSettingsPositionalArgsBuilder{
        builder: NewPositionalArgsBuilder(),
    }
}

// WithComponent adds optional component positional argument.
func (b *ListSettingsPositionalArgsBuilder) WithComponent(required bool) *ListSettingsPositionalArgsBuilder {
    b.builder.AddArg(&PositionalArgSpec{
        Name:        "component",
        Description: "Component name to filter settings",
        Required:    required,
        TargetField: "Component", // Maps to StandardOptions.Component
    })
    return b
}

func (b *ListSettingsPositionalArgsBuilder) Build() ([]*PositionalArgSpec, cobra.PositionalArgs, string) {
    return b.builder.Build()
}
```

### Phase 4: Update list settings command

```go
// cmd/list_settings.go
var listSettingsParser = flags.NewStandardOptionsBuilder().
    WithProcessTemplates(true).
    WithProcessFunctions(true).
    WithPositionalArgs(flags.NewListSettingsPositionalArgsBuilder().
        WithComponent(false).  // Optional component arg
        Build()).
    Build()

// listSettingsCmd uses the builder pattern
var listSettingsCmd = &cobra.Command{
    Use:   "settings [component]",
    Args:  cobra.ArbitraryArgs,  // Validation done in RunE after parsing
    RunE: func(cmd *cobra.Command, args []string) error {
        opts, err := listSettingsParser.Parse(cmd.Context(), args)
        if err != nil {
            return err
        }

        // Component extracted by PositionalArgsBuilder
        output, err := listSettings(cmd, opts)
        if err != nil {
            return err
        }

        utils.PrintMessage(output)
        return nil
    },
}

// Update function signature to use opts instead of args
func listSettings(cmd *cobra.Command, opts *flags.StandardOptions) (string, error) {
    params, err := initSettingsParams(cmd, opts)
    // ... rest of logic
}

func initSettingsParams(cmd *cobra.Command, opts *flags.StandardOptions) (*SettingsParams, error) {
    // ... existing flag extraction ...

    // Component comes from opts, not manual extraction
    componentFilter := opts.Component

    return &SettingsParams{
        CommonFlags:     commonFlags,
        ProcessingFlags: processingFlags,
        ComponentFilter: componentFilter,
    }, nil
}
```

## Benefits

1. **Type Safety**: `opts.Component` instead of `args[0]`
2. **Validation**: Cobra validator ensures correct arg count
3. **Consistency**: Same pattern as terraform/packer/helmfile commands
4. **Testability**: Can test positional arg extraction independently
5. **Maintainability**: Clear intent, no manual array indexing
6. **Refactoring Safety**: Changing StandardOptions.Component updates all usages

## Effort Estimate

- **Phase 1**: Add WithPositionalArgs to StandardOptionsBuilder (~1 hour)
- **Phase 2**: Add SetPositionalArgs to StandardParser and update Parse() (~2 hours)
- **Phase 3**: Create ListSettingsPositionalArgsBuilder (~30 min)
- **Phase 4**: Update list settings command (~30 min)
- **Testing**: Add unit tests and integration tests (~2 hours)
- **Total**: ~6 hours

## Dependencies

- None - this is a pure enhancement to StandardOptionsBuilder
- Does not affect existing commands (backward compatible)
- PassThroughOptionsBuilder already has this pattern

## Risks

- **Breaking Change**: If StandardOptions field mapping changes, commands break
- **Reflection**: Using reflection to set fields by name can be fragile
- **Complexity**: Adds another layer of abstraction

## Mitigation

- Use compile-time struct tags to verify TargetField mapping
- Add comprehensive unit tests for positional arg extraction
- Document the pattern clearly in CLAUDE.md

## Priority

**Medium** - This is a code quality improvement, not a bug fix.

Current code works correctly, just doesn't follow the preferred builder pattern. The main fix (GlobalFlags in CommonFlags) is complete and resolves the core issue.

## References

- **Current Issue**: Screenshot showing manual `args[0]` extraction
- **Pattern Example**: `cmd/terraform_plan.go` - PassThroughOptionsBuilder with positional args
- **Builder Implementation**: `pkg/flags/passthrough_builder.go` - Has WithPositionalArgs
- **ARG Builder**: `pkg/flags/packer_positional_args_builder.go` - Domain-specific builder
- **Main Fix**: `pkg/flags/registry.go` - CommonFlags() now includes GlobalFlags()
