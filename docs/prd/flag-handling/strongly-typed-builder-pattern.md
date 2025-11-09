# Strongly-Typed Options Builder Pattern

**Status:** ✅ Implemented
**Package:** `pkg/flags`
**Types:** `StandardOptionsBuilder`, `StandardOptions`

## Problem Statement

Current flag parser implementation has a disconnect between flag definitions and options struct fields:

```go
// Flag definition (strings - not type-safe)
parser := NewStandardParser(
    WithStringFlag("stack", "s", "", "Stack name"),
    WithStringFlag("format", "f", "yaml", "Output format"),
)

// Options usage (disconnected - runtime errors if flag name changes)
opts.Stack   // No compile-time guarantee "stack" flag exists
opts.Format  // Could be renamed independently of flag definition
```

**Problems:**
1. **No compile-time safety**: Typo in flag name = runtime error
2. **Disconnect**: Flag definitions separate from struct fields
3. **Refactoring hazard**: Renaming struct field doesn't update flag name
4. **Testing difficulty**: Need integration tests to verify flag-to-field mapping

## Solution: Builder Pattern with Type-Safe Methods

Connect flag definitions directly to struct fields through strongly-typed builder methods:

```go
// Builder methods guarantee flag name matches struct field
builder := NewStandardOptionsBuilder().
    WithStack(true).        // Adds "stack" flag + maps to .Stack field
    WithFormat("yaml").     // Adds "format" flag + maps to .Format field
    WithQuery()             // Adds "query" flag + maps to .Query field

// Compile-time guarantee: These fields exist
opts.Stack   // ✅ Type-safe
opts.Format  // ✅ Type-safe
opts.Query   // ✅ Type-safe
```

## Benefits

### 1. Compile-Time Type Safety

**Before:**
```go
WithStringFlag("stackk", "s", "", "Stack")  // ✅ Compiles (typo undetected)
opts.Stack  // Runtime: flag "stackk" doesn't map to .Stack
```

**After:**
```go
builder.WithStackk(true)  // ❌ Compile error: method doesn't exist
                          //    Compiler suggests: WithStack
```

### 2. Refactoring Safety

**Before:**
- Rename `Stack` field → flag "stack" still uses old name → runtime error

**After:**
- `WithStack()` method guarantees flag name matches field
- Renaming field requires updating method → compiler catches inconsistency

### 3. Better Testability

```go
// Test individual flag configurations
func TestWithStack(t *testing.T) {
    builder := NewStandardInterpreterBuilder().WithStack(true)

    // Verify flag registered correctly
    assert.True(t, builder.HasFlag("stack"))
    assert.True(t, builder.IsRequired("stack"))
}
```

### 4. Clear Intent

```go
// Explicit: You see exactly what flags this command supports
builder := NewStandardOptionsBuilder().
    WithStack(true).        // Required stack flag
    WithFormat("yaml").     // Optional format flag with default
    WithDryRun()            // Optional boolean flag
```

## Implementation

### StandardOptionsBuilder

```go
type StandardOptionsBuilder struct {
    registry *FlagRegistry
    options  []Option
}

func NewStandardOptionsBuilder() *StandardOptionsBuilder {
    return &StandardOptionsBuilder{
        registry: NewFlagRegistry(),
        options:  []Option{},
    }
}

// WithStack adds required or optional stack flag.
// Maps to StandardOptions.Stack field.
func (b *StandardOptionsBuilder) WithStack(required bool) *StandardOptionsBuilder {
    b.options = append(b.options, WithStringFlag("stack", "s", "", "Atmos stack"))
    if required {
        b.options = append(b.options, WithRequired("stack"))
    }
    return b
}

// WithFormat adds format flag with specified default.
// Maps to StandardOptions.Format field.
func (b *StandardOptionsBuilder) WithFormat(defaultValue string) *StandardOptionsBuilder {
    b.options = append(b.options, WithStringFlag("format", "f", defaultValue, "Output format"))
    return b
}

// Build creates the StandardParser with configured flags.
func (b *StandardOptionsBuilder) Build() *StandardParser {
    return NewStandardParser(b.options...)
}
```

### Usage in Commands

```go
// cmd/describe_component.go
var describeComponentParser *flags.StandardParser

func init() {
    describeComponentParser = flags.NewStandardOptionsBuilder().
        WithStack(true).                // Required
        WithFormat("yaml").             // Default: yaml
        WithFile().                     // Optional
        WithProcessTemplates(true).     // Default: true
        WithProcessFunctions(true).     // Default: true
        WithSkip().                     // Optional
        WithQuery().                    // Optional
        WithProvenance().               // Optional
        Build()
}

func runCommand(cmd *cobra.Command, args []string) error {
    opts, err := describeComponentParser.Parse(ctx, args)
    if err != nil {
        return err
    }

    // Type-safe field access - guaranteed to exist!
    return executeCommand(
        opts.Stack,      // Compile-time guarantee
        opts.Format,     // Compile-time guarantee
        opts.Query,      // Compile-time guarantee
    )
}
```

## Design Principles

### 1. One Method Per Flag

Each common flag gets exactly one builder method:
- `WithStack(required bool)` → `opts.Stack`
- `WithFormat(default string)` → `opts.Format`
- `WithComponent(required bool)` → `opts.Component`
- `WithIdentity()` → `opts.Identity` (from GlobalFlags)

### 2. Method Name Matches Field Name

```go
WithStack()      → .Stack
WithFormat()     → .Format
WithDryRun()     → .DryRun
WithQuery()      → .Query
```

### 3. Type-Safe Parameters

```go
WithStack(required bool)           // Boolean for required vs optional
WithFormat(defaultValue string)    // String for default value
WithProcessTemplates(default bool) // Boolean for default
```

### 4. Builder Methods Return Self

Enables fluent method chaining:
```go
builder.WithStack(true).WithFormat("yaml").WithQuery()
```

## Comparison with Alternatives

### Alternative 1: Struct Tags (Reflection-Based)

```go
type Interpreter struct {
    Stack  string `flag:"stack,s,required" env:"ATMOS_STACK"`
    Format string `flag:"format,f" default:"yaml"`
}
```

**Rejected because:**
- ❌ Runtime reflection overhead
- ❌ No compile-time validation of tag syntax
- ❌ Typos in tags only caught at runtime
- ❌ Harder to test
- ❌ Poor refactoring support

### Alternative 2: String-Based Options (Current)

```go
WithStringFlag("stack", "s", "", "Stack name")
```

**Rejected because:**
- ❌ No connection between flag name and struct field
- ❌ Typos in flag names not caught at compile time
- ❌ Refactoring struct doesn't update flags

## Migration Path

### Phase 1: Create Builder Infrastructure
1. Implement `StandardInterpreterBuilder`
2. Add `WithXxx()` methods for all common flags
3. Add tests for each method

### Phase 2: Migrate One Command (Proof of Concept)
1. Convert `describe component` to use builder
2. Verify type safety works
3. Measure test coverage improvement

### Phase 3: Mass Migration
1. Convert all describe commands
2. Convert all list commands
3. Convert all validate commands
4. Convert all vendor/workflow/aws/pro commands

### Phase 4: Cleanup
1. Remove old string-based helper functions if unused
2. Update documentation
3. Add linting rules to prevent direct Cobra flag access

## Testing Strategy

### Unit Tests for Builder Methods

```go
func TestWithStack(t *testing.T) {
    builder := NewStandardOptionsBuilder().WithStack(true)
    parser := builder.Build()

    // Verify flag exists and is required
    assert.True(t, parser.HasFlag("stack"))
    assert.True(t, parser.IsFlagRequired("stack"))
}

func TestWithFormat(t *testing.T) {
    builder := NewStandardOptionsBuilder().WithFormat("json")
    parser := builder.Build()

    // Verify flag exists with correct default
    assert.True(t, parser.HasFlag("format"))
    assert.Equal(t, "json", parser.GetFlagDefault("format"))
}
```

### Integration Tests

```go
func TestDescribeComponentWithBuilder(t *testing.T) {
    parser := NewStandardOptionsBuilder().
        WithStack(true).
        WithFormat("yaml").
        Build()

    opts, err := parser.Parse(ctx, []string{"--stack", "prod"})
    require.NoError(t, err)

    // Type-safe access
    assert.Equal(t, "prod", opts.Stack)
    assert.Equal(t, "yaml", opts.Format)
}
```

## Success Metrics

1. **Compile-time safety**: 100% of flag-to-field mappings validated at compile time
2. **Test coverage**: Builder methods reach 100% coverage
3. **Refactoring safety**: Renaming struct fields causes compile errors, not runtime errors
4. **Performance**: Zero reflection overhead (measured with benchmarks)
5. **Developer experience**: Developers prefer builder pattern over string-based options

## Future Enhancements

### Custom Validators

```go
WithStack(true, ValidateStackExists)  // Custom validation function
```

### Conditional Flags

```go
builder.When(condition, func(b *Builder) {
    b.WithFormat("json")
})
```

### Flag Groups

```go
builder.WithProcessingFlags()  // Adds process-templates, process-functions, skip
```

## References

- [Fluent Interface Pattern](https://en.wikipedia.org/wiki/Fluent_interface)
- [Builder Pattern](https://refactoring.guru/design-patterns/builder)
- Existing PRDs: `unified-flag-parsing.md`, `strongly-typed-interpreters.md`

## Migration Strategy

### Phase 1: `pkg/flags` Refactoring (Current PR) ✅

**Completed:**
- Renamed `pkg/flags` → `pkg/flags` for clearer naming
- Renamed `*Interpreter` → `*Options` (options are the result of parsing)
- Created `StandardOptionsBuilder` with type-safe builder pattern
- All tests passing with 100% coverage on builder

**For commands NOT yet in command registry:**
```go
// Example: cmd/describe_component.go
import "github.com/cloudposse/atmos/pkg/flags"

parser := flags.NewStandardOptionsBuilder().
    WithStack(true).
    WithFormat("yaml").
    Build()

opts, err := parser.Parse(ctx, args)
```

### Phase 2: Co-locate with Command Registry (Future PRs)

**As commands migrate TO command registry pattern**, move parsers/options INTO command packages:

```
cmd/describe/
├── provider.go          # DescribeCommandProvider
├── component.go         # component subcommand
├── options.go           # ComponentOptions (co-located with command)
└── component_test.go
```

**What stays in `pkg/flags`:**
- Core infrastructure (types, registry, parser interfaces)
- Shared types (GlobalFlags, selectors)
- Reusable builders (StandardOptionsBuilder)

**What moves to command packages:**
- Command-specific parsers (optional - can reuse StandardParser)
- Command-specific options structs

**Example after migration:**
```go
// cmd/describe/component.go
package describe

import "github.com/cloudposse/atmos/pkg/flags"

// ComponentOptions is co-located with the command.
type ComponentOptions struct {
    flags.GlobalFlags
    Stack     string
    Component string
    Format    string
}

var componentParser = flags.NewStandardOptionsBuilder().
    WithStack(true).
    WithFormat("yaml").
    Build()
```

### Current State

**Already in command registry:**
- ✅ `cmd/about/` - AboutCommandProvider
- ✅ `cmd/version/` - VersionCommandProvider (using new options pattern)

**Not yet migrated (~30 commands):**
- `cmd/terraform.go` - Uses `flags.TerraformParser` + `flags.TerraformOptions`
- `cmd/describe_component.go` - Uses Cobra flags directly
- `cmd/vendor_pull.go` - Uses Cobra flags directly
- etc.

**Migration approach:**
1. This PR: `pkg/flags` refactoring complete
2. Future PRs: Migrate commands to registry + co-locate options as needed
