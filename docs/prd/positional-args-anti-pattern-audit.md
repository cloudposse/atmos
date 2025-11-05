# Positional Args Anti-Pattern Audit

**Date**: 2025-11-05
**Status**: ✅ Audit Complete
**Affected Commands**: 6 commands

## Anti-Pattern Identified

Commands using **`Args: cobra.ArbitraryArgs` + manual `len()` validation** instead of the **PositionalArgsBuilder pattern**.

### ❌ Commands with Anti-Pattern

1. **`cmd/list_settings.go:41,53`**
   - Uses: `Args: cobra.ArbitraryArgs`
   - Manual validation: `if len(positionalArgs) > 1`
   - Expected args: `[component]` (optional)
   - Fix: Use `ListSettingsPositionalArgsBuilder` with optional component

2. **`cmd/list_values.go:82,98`** (list keys subcommand)
   - Uses: `Args: cobra.ArbitraryArgs`
   - Manual validation: `if len(positionalArgs) != 1`
   - Expected args: `<component>` (required)
   - Fix: Use `ListKeysPositionalArgsBuilder` with required component

3. **`cmd/list_values.go:125,146`** (list components subcommand)
   - Uses: `Args: cobra.ArbitraryArgs`
   - Manual validation: `if len(positionalArgs) != 1`
   - Expected args: `<key>` (required)
   - Fix: Use `ListComponentsPositionalArgsBuilder` with required key

4. **`cmd/validate_schema.go:54,67-72`**
   - Uses: `Args: cobra.ArbitraryArgs`
   - Manual validation: `if len(positionalArgs) > 1`
   - Expected args: `[schemaType]` (optional)
   - Fix: Use `ValidateSchemaPositionalArgsBuilder` with optional schemaType

5. **`cmd/describe_dependents.go:36,60`**
   - Uses: `Args: cobra.ArbitraryArgs`
   - Manual validation: `if len(positionalArgs) != 1`
   - Expected args: `<component>` (required)
   - Fix: Use `DescribeDependentsPositionalArgsBuilder` with required component

6. **`cmd/describe_component.go:37,54`**
   - Uses: `Args: cobra.ArbitraryArgs`
   - Manual validation: `if len(positionalArgs) != 1`
   - Expected args: `<component>` (required)
   - Fix: Use `DescribeComponentPositionalArgsBuilder` with required component

## Why It's an Anti-Pattern

### Current Approach (Anti-Pattern)
```go
var listSettingsCmd = &cobra.Command{
    Use:  "settings [component]",
    Args: cobra.ArbitraryArgs,  // ❌ No validation at Cobra level
    RunE: func(cmd *cobra.Command, args []string) error {
        opts, err := parser.Parse(ctx, args)

        // ❌ Manual validation AFTER parsing
        positionalArgs := opts.GetPositionalArgs()
        if len(positionalArgs) > 1 {
            return fmt.Errorf("invalid arguments...")
        }

        // ❌ Manual extraction from raw array
        component := ""
        if len(args) > 0 {
            component = args[0]
        }
    },
}
```

**Problems:**
1. **No type safety** - accessing `args[0]` can panic
2. **Late validation** - errors happen after flag parsing
3. **Inconsistent** - different from terraform/packer/helmfile commands
4. **Untestable** - hard to unit test positional arg extraction
5. **Manual indexing** - error-prone array access

### Desired Approach (Builder Pattern)
```go
var listSettingsParser = flags.NewStandardOptionsBuilder().
    WithProcessTemplates(true).
    WithPositionalArgs(flags.NewListSettingsPositionalArgsBuilder().
        WithComponent(false).  // ✅ Explicit: optional component
        Build()).
    Build()

var listSettingsCmd = &cobra.Command{
    Use:  "settings [component]",
    Args: cobra.ArbitraryArgs,  // Validation in RunE after parsing
    RunE: func(cmd *cobra.Command, args []string) error {
        opts, err := parser.Parse(ctx, args)  // ✅ Validates + extracts
        if err != nil {
            return err  // ✅ Early error with clear message
        }

        // ✅ Type-safe access
        component := opts.Component
    },
}
```

**Benefits:**
1. **Type safety** - `opts.Component` is always safe to access
2. **Early validation** - errors during Parse() with helpful messages
3. **Consistent** - same pattern as all passthrough commands
4. **Testable** - can mock PositionalArgsBuilder
5. **Clear intent** - builder makes expected args explicit

## Root Cause

**StandardOptionsBuilder lacks `WithPositionalArgs()` method.**

- ✅ **PassThroughOptionsBuilder** HAS `WithPositionalArgs()` - used by terraform/packer/helmfile
- ❌ **StandardOptionsBuilder** LACKS `WithPositionalArgs()` - used by describe/list/validate commands

## Solution: Extend StandardOptionsBuilder

### Required Changes

See `docs/prd/list-settings-positional-args-todo.md` for detailed implementation plan.

**Summary:**
1. Add `WithPositionalArgs()` to StandardOptionsBuilder
2. Add `SetPositionalArgs()` to StandardParser
3. Update `Parse()` to extract positional args using TargetField mapping
4. Create domain-specific builders (ListSettingsPositionalArgsBuilder, etc.)
5. Update each affected command to use builder pattern

### Effort Estimate

**Per command**: 2-3 hours (builder creation + command update + tests)
**Total**: 12-18 hours for all 6 commands

## Priority

**Medium** - Code quality improvement, not a bug.

All commands work correctly today, they just don't follow the preferred builder pattern. This is technical debt, not a functional issue.

## Implementation Order

Suggested order (easiest to hardest):

1. **list settings** - 1 optional arg, simplest case
2. **validate schema** - 1 optional arg, similar to list settings
3. **describe component** - 1 required arg, simple
4. **describe dependents** - 1 required arg, simple
5. **list keys** - 1 required arg, subcommand
6. **list components** - 1 required arg, subcommand

## Testing Strategy

For each command:
1. **Unit test** the positional args builder
2. **Integration test** the command with valid/invalid args
3. **Regression test** ensure existing behavior unchanged

## References

- **Pattern Example**: `cmd/terraform_plan.go` - Uses PassThroughOptionsBuilder
- **Builder Implementation**: `pkg/flags/passthrough_builder.go` - Has WithPositionalArgs
- **Domain Builders**: `pkg/flags/*_positional_args_builder.go` - Domain-specific builders
- **Related PRD**: `docs/prd/list-settings-positional-args-todo.md` - Detailed implementation design

## Next Steps

1. ✅ **Audit complete** - 6 commands identified
2. ⏸️ **Implementation deferred** - Requires StandardOptionsBuilder enhancement
3. 📝 **Document pattern** - Update CLAUDE.md with builder pattern guidance
4. 🔄 **Track in backlog** - Add to technical debt tracking
