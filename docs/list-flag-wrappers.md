# List Command Flag Wrappers

## Overview

This document explains the flag wrapper pattern used across list commands in `cmd/list/`. The wrapper functions follow the StandardParser pattern from `pkg/flags/` and provide a consistent, reusable way to compose flags for each command.

## Design Principles

### 1. One Function Per Flag

Each flag gets its own wrapper function (not grouped). This provides maximum flexibility for composing only the flags each command needs.

**Example:**
```go
func WithFormatFlag(options *[]flags.Option)
func WithColumnsFlag(options *[]flags.Option)
func WithSortFlag(options *[]flags.Option)
```

### 2. Consistent Naming Convention

All wrapper functions follow the `With{FlagName}Flag` pattern, matching the convention in `pkg/flags/standard_builder.go`.

**Good:**
```go
WithFormatFlag
WithColumnsFlag
WithStackFlag
```

**Bad:**
```go
addFormatFlag      // Wrong prefix
AddFormat          // Missing "Flag" suffix
withFormat         // Not exported
```

### 3. Composable by Design

Commands use `NewListParser()` with only the wrapper functions they need. This is the "options pattern" applied to flag composition.

**Example:**
```go
// Components needs many flags
componentsParser = NewListParser(
    WithFormatFlag,
    WithColumnsFlag,
    WithSortFlag,
    WithFilterFlag,
    WithStackFlag,
    WithTypeFlag,
    WithEnabledFlag,
    WithLockedFlag,
)

// Stacks needs fewer flags
stacksParser = NewListParser(
    WithFormatFlag,
    WithColumnsFlag,
    WithSortFlag,
    WithComponentFlag,
)
```

### 4. Single Source of Truth

Each wrapper function defines:
- Flag name and shorthand
- Default value
- Description
- Environment variable bindings
- Valid values (if applicable)

This ensures consistency across all commands using that flag.

## Available Flag Wrappers

### Universal Flags (Used by Most Commands)

#### `WithFormatFlag`
- **Flag:** `--format` / `-f`
- **Environment:** `ATMOS_LIST_FORMAT`
- **Description:** Output format: table, json, yaml, csv, tsv
- **Used by:** All list commands

#### `WithColumnsFlag`
- **Flag:** `--columns`
- **Environment:** `ATMOS_LIST_COLUMNS`
- **Description:** Columns to display (comma-separated, overrides atmos.yaml)
- **Used by:** components, stacks, workflows, vendor, instances

#### `WithSortFlag`
- **Flag:** `--sort`
- **Environment:** `ATMOS_LIST_SORT`
- **Description:** Sort by column:order (e.g., 'stack:asc,component:desc')
- **Used by:** components, stacks, workflows, vendor, instances

#### `WithStackFlag`
- **Flag:** `--stack` / `-s`
- **Environment:** `ATMOS_STACK`
- **Description:** Filter by stack pattern (glob, e.g., 'plat-*-prod')
- **Used by:** components, vendor, values, vars, metadata, settings, instances

### Filtering Flags

#### `WithFilterFlag`
- **Flag:** `--filter`
- **Environment:** `ATMOS_LIST_FILTER`
- **Description:** Filter expression using YQ syntax
- **Used by:** components, vendor

#### `WithQueryFlag`
- **Flag:** `--query` / `-q`
- **Environment:** `ATMOS_LIST_QUERY`
- **Description:** YQ expression to filter values (e.g., '.vars.region')
- **Used by:** values, vars, metadata, settings

### Component-Specific Flags

#### `WithTypeFlag`
- **Flag:** `--type` / `-t`
- **Environment:** `ATMOS_COMPONENT_TYPE`
- **Description:** Component type: real, abstract, all
- **Default:** `real`
- **Valid Values:** `real`, `abstract`, `all`
- **Used by:** components

#### `WithEnabledFlag`
- **Flag:** `--enabled`
- **Environment:** `ATMOS_COMPONENT_ENABLED`
- **Description:** Filter by enabled status
- **Default:** `false`
- **Used by:** components

#### `WithLockedFlag`
- **Flag:** `--locked`
- **Environment:** `ATMOS_COMPONENT_LOCKED`
- **Description:** Filter by locked status
- **Default:** `false`
- **Used by:** components

#### `WithAbstractFlag`
- **Flag:** `--abstract`
- **Environment:** `ATMOS_ABSTRACT`
- **Description:** Include abstract components in output
- **Default:** `false`
- **Used by:** values, vars

### Stack-Specific Flags

#### `WithComponentFlag`
- **Flag:** `--component` / `-c`
- **Environment:** `ATMOS_COMPONENT`
- **Description:** Filter stacks by component name
- **Used by:** stacks

### Workflow-Specific Flags

#### `WithFileFlag`
- **Flag:** `--file`
- **Environment:** `ATMOS_WORKFLOW_FILE`
- **Description:** Filter workflows by file path
- **Used by:** workflows

### Output Formatting Flags

#### `WithDelimiterFlag`
- **Flag:** `--delimiter`
- **Environment:** `ATMOS_LIST_DELIMITER`
- **Description:** Delimiter for CSV/TSV output
- **Used by:** workflows, vendor, values, vars, metadata, settings, instances

#### `WithMaxColumnsFlag`
- **Flag:** `--max-columns`
- **Environment:** `ATMOS_LIST_MAX_COLUMNS`
- **Description:** Maximum number of columns to display (0 = no limit)
- **Default:** `0`
- **Used by:** values, vars, metadata, settings

### Template Processing Flags

#### `WithProcessTemplatesFlag`
- **Flag:** `--process-templates`
- **Environment:** `ATMOS_PROCESS_TEMPLATES`
- **Description:** Enable/disable Go template processing
- **Default:** `true`
- **Used by:** values, vars, metadata, settings

#### `WithProcessFunctionsFlag`
- **Flag:** `--process-functions`
- **Environment:** `ATMOS_PROCESS_FUNCTIONS`
- **Description:** Enable/disable template function processing
- **Default:** `true`
- **Used by:** values, vars, metadata, settings

### Pro Integration Flags

#### `WithUploadFlag`
- **Flag:** `--upload`
- **Environment:** `ATMOS_UPLOAD`
- **Description:** Upload instances to Atmos Pro API
- **Default:** `false`
- **Used by:** instances

## Usage Patterns

### Pattern 1: Full-Featured Command (Components)

```go
func init() {
    componentsParser = NewListParser(
        WithFormatFlag,      // Output format selection
        WithColumnsFlag,     // Column customization
        WithSortFlag,        // Sorting
        WithFilterFlag,      // YQ filtering
        WithStackFlag,       // Filter by stack
        WithTypeFlag,        // Filter by component type
        WithEnabledFlag,     // Filter by enabled status
        WithLockedFlag,      // Filter by locked status
    )

    componentsParser.RegisterFlags(componentsCmd)
    _ = componentsParser.BindToViper(viper.GetViper())
}
```

### Pattern 2: Simple Command (Stacks)

```go
func init() {
    stacksParser = NewListParser(
        WithFormatFlag,      // Output format
        WithColumnsFlag,     // Column customization
        WithSortFlag,        // Sorting
        WithComponentFlag,   // Filter stacks by component
    )

    stacksParser.RegisterFlags(stacksCmd)
    _ = stacksParser.BindToViper(viper.GetViper())
}
```

### Pattern 3: Complex Filtering Command (Values)

```go
func init() {
    valuesParser = NewListParser(
        WithFormatFlag,              // Output format
        WithDelimiterFlag,           // CSV/TSV delimiter
        WithMaxColumnsFlag,          // Limit columns
        WithQueryFlag,               // YQ expression filtering
        WithStackFlag,               // Filter by stack
        WithAbstractFlag,            // Include abstract components
        WithProcessTemplatesFlag,    // Process templates
        WithProcessFunctionsFlag,    // Process functions
    )

    valuesParser.RegisterFlags(valuesCmd)
    _ = valuesParser.BindToViper(viper.GetViper())
}
```

## Flag Mapping Reference

| Command | Format | Columns | Sort | Filter | Stack | Delimiter | Command-Specific |
|---------|--------|---------|------|--------|-------|-----------|------------------|
| **stacks** | ✓ | ✓ | ✓ | - | - | - | `--component` |
| **components** | ✓ | ✓ | ✓ | ✓ | ✓ | - | `--type`, `--enabled`, `--locked` |
| **workflows** | ✓ | ✓ | ✓ | - | - | ✓ | `--file` |
| **vendor** | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | - |
| **values** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--abstract`, `--process-*` |
| **vars** | ✓ | - | - | - | ✓ | ✓ | Same as values (alias) |
| **metadata** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--process-*` |
| **settings** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--process-*` |
| **instances** | ✓ | ✓ | ✓ | - | ✓ | ✓ | `--upload` |

## Best Practices

### 1. Only Include Flags Your Command Needs

Don't add flags "just in case" - compose only what makes sense for your command.

**Good:**
```go
// Stacks command doesn't need --stack flag (it lists all stacks)
stacksParser = NewListParser(
    WithFormatFlag,
    WithComponentFlag,  // Filter by component makes sense
)
```

**Bad:**
```go
// Don't add flags that don't make sense
stacksParser = NewListParser(
    WithFormatFlag,
    WithStackFlag,    // ❌ Doesn't make sense for listing stacks
    WithLockedFlag,   // ❌ Stacks don't have locked status
)
```

### 2. Follow Alphabetical Ordering (Optional)

For readability, consider ordering wrapper functions alphabetically or by logical grouping.

**Example:**
```go
componentsParser = NewListParser(
    // Output formatting
    WithFormatFlag,
    WithColumnsFlag,
    WithSortFlag,

    // Filtering
    WithFilterFlag,
    WithStackFlag,
    WithTypeFlag,

    // Boolean filters
    WithEnabledFlag,
    WithLockedFlag,
)
```

### 3. Add Comments for Clarity

When composing flags, add comments explaining what each flag does.

**Example:**
```go
componentsParser = NewListParser(
    WithFormatFlag,      // --format (table/json/yaml/csv/tsv)
    WithColumnsFlag,     // --columns (override atmos.yaml)
    WithSortFlag,        // --sort "stack:asc,component:desc"
    WithFilterFlag,      // --filter (YQ expression)
    WithStackFlag,       // --stack "plat-*-prod"
    WithTypeFlag,        // --type real/abstract/all
    WithEnabledFlag,     // --enabled=true
    WithLockedFlag,      // --locked=false
)
```

### 4. Reuse Existing Wrappers

Before creating a new wrapper, check if one already exists in `flag_wrappers.go`. Reuse whenever possible.

### 5. Test Your Flag Composition

Use the test patterns in `flag_wrappers_test.go` to verify your flag composition works correctly.

```go
func TestYourCommand_Flags(t *testing.T) {
    parser := NewListParser(
        WithFormatFlag,
        WithColumnsFlag,
    )

    cmd := &cobra.Command{Use: "test"}
    parser.RegisterFlags(cmd)

    // Verify expected flags exist
    assert.NotNil(t, cmd.Flags().Lookup("format"))
    assert.NotNil(t, cmd.Flags().Lookup("columns"))

    // Verify unwanted flags don't exist
    assert.Nil(t, cmd.Flags().Lookup("stack"))
}
```

## Adding New Flag Wrappers

When you need to add a new flag wrapper:

### 1. Follow the Template

```go
// WithYourFlagNameFlag adds your-flag-name flag with environment variable support.
// Used by: command1, command2.
func WithYourFlagNameFlag(options *[]flags.Option) {
    defer perf.Track(nil, "list.WithYourFlagNameFlag")()

    *options = append(*options,
        flags.WithStringFlag("your-flag-name", "y", "default", "Description"),
        flags.WithEnvVars("your-flag-name", "ATMOS_YOUR_FLAG_NAME"),
    )
}
```

### 2. Add Godoc Comments

Include:
- What the flag does
- Which commands use it
- Example usage (if complex)

### 3. Add Tests

Add test coverage in `flag_wrappers_test.go`:

```go
func TestWithYourFlagNameFlag(t *testing.T) {
    parser := NewListParser(WithYourFlagNameFlag)
    assert.NotNil(t, parser)

    cmd := &cobra.Command{Use: "test"}
    parser.RegisterFlags(cmd)

    flag := cmd.Flags().Lookup("your-flag-name")
    require.NotNil(t, flag, "your-flag-name flag should be registered")
    assert.Equal(t, "y", flag.Shorthand)
    assert.Equal(t, "default", flag.DefValue)
    assert.Contains(t, flag.Usage, "Description")
}
```

### 4. Update Documentation

Add your new flag to this document's "Available Flag Wrappers" section.

## Benefits of This Pattern

### 1. Discoverability

Autocomplete shows all available flag wrappers when typing `With` in your editor.

### 2. Consistency

Each flag has the same configuration across all commands that use it:
- Same description
- Same environment variable
- Same default value
- Same shorthand

### 3. Maintainability

Updating a flag's behavior requires changing only one function, not every command that uses it.

### 4. Testability

Each wrapper can be tested independently, and flag composition can be verified per command.

### 5. Readability

Command initialization clearly shows which flags are supported:

```go
componentsParser = NewListParser(
    WithFormatFlag,
    WithColumnsFlag,
    WithSortFlag,
)
```

This is more readable than:

```go
componentsParser = flags.NewStandardParser(
    flags.WithStringFlag("format", "", "", "Output format: table, json, yaml, csv, tsv"),
    flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
    flags.WithStringSliceFlag("columns", "", nil, "Columns to display"),
    flags.WithEnvVars("columns", "ATMOS_LIST_COLUMNS"),
    flags.WithStringFlag("sort", "", "", "Sort by column:order"),
    flags.WithEnvVars("sort", "ATMOS_LIST_SORT"),
)
```

## Common Mistakes to Avoid

### 1. Don't Modify the Options Slice Incorrectly

**Wrong:**
```go
func WithBadFlag(options *[]flags.Option) {
    options = append(*options, flags.WithStringFlag(...))  // ❌ Missing dereference
}
```

**Correct:**
```go
func WithGoodFlag(options *[]flags.Option) {
    *options = append(*options, flags.WithStringFlag(...))  // ✅ Correct
}
```

### 2. Don't Forget Environment Variable Bindings

**Wrong:**
```go
func WithFormatFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("format", "", "", "Output format"),
        // ❌ Missing WithEnvVars
    )
}
```

**Correct:**
```go
func WithFormatFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("format", "", "", "Output format"),
        flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),  // ✅ Correct
    )
}
```

### 3. Don't Use Inconsistent Naming

**Wrong:**
```go
func AddFormatFlag(...)     // ❌ Wrong prefix
func withColumnsFlag(...)   // ❌ Not exported
func WithFormat(...)        // ❌ Missing "Flag" suffix
```

**Correct:**
```go
func WithFormatFlag(...)    // ✅ Correct
func WithColumnsFlag(...)   // ✅ Correct
```

## Migration Guide

If you have existing list commands using the old pattern, follow these steps:

### Step 1: Identify Flags Used

```go
// Old pattern
cmd.Flags().StringP("format", "", "", "Output format")
cmd.Flags().StringSliceP("columns", "", nil, "Columns")
```

### Step 2: Map to Wrapper Functions

```go
// New pattern
componentsParser = NewListParser(
    WithFormatFlag,
    WithColumnsFlag,
)
```

### Step 3: Update RunE to Use Parser

```go
// Old pattern
RunE: func(cmd *cobra.Command, args []string) error {
    format, _ := cmd.Flags().GetString("format")
    // ...
}

// New pattern
RunE: func(cmd *cobra.Command, args []string) error {
    v := viper.GetViper()
    if err := componentsParser.BindFlagsToViper(cmd, v); err != nil {
        return err
    }

    format := v.GetString("format")
    // ...
}
```

### Step 4: Test

Run tests to ensure flags work as expected:

```bash
go test ./cmd/list -run TestComponents -v
```

## Related Documentation

- `pkg/flags/standard_parser.go` - StandardParser implementation
- `pkg/flags/standard_builder.go` - Builder pattern with With* methods
- `docs/prd/list-commands-ui-overhaul.md` - List commands architecture
- `flag_wrappers_examples.go` - Example usage patterns
- `flag_wrappers_test.go` - Test coverage examples
