# List Command Flag Wrappers - Implementation Guide

## Answers to Your Questions

### 1. Should I create one wrapper function per flag?

**YES - One function per flag is the correct approach.**

**Rationale:**
- Maximum flexibility - each command composes only the flags it needs
- Follows the established `With*` naming convention from `pkg/flags/standard_builder.go`
- Better discoverability via autocomplete
- Easier to test individually
- Single source of truth for each flag's configuration

**Example:**
```go
func WithFormatFlag(options *[]flags.Option)
func WithColumnsFlag(options *[]flags.Option)
func WithStackFlag(options *[]flags.Option)
```

**NOT grouped functions like:**
```go
func WithFilterFlags(options *[]flags.Option)  // ❌ Too broad
func WithOutputFlags(options *[]flags.Option)  // ❌ Not granular enough
```

### 2. What naming convention should I use?

**Use: `With{FlagName}Flag` pattern**

**Examples:**
- `WithFormatFlag` - for `--format` flag
- `WithColumnsFlag` - for `--columns` flag
- `WithStackFlag` - for `--stack` flag
- `WithEnabledFlag` - for `--enabled` flag

**Key rules:**
1. **Prefix:** `With` (capitalized, exported)
2. **Middle:** Flag name in PascalCase (e.g., `MaxColumns` for `--max-columns`)
3. **Suffix:** `Flag` (makes it clear this is about a flag)

**Consistency with pkg/flags/:**
This follows the same pattern as `pkg/flags/standard_builder.go`:
- `WithStack(bool)` - builder method for stack flag
- `WithFormat([]string, string)` - builder method for format flag

Our list-specific wrappers extend this pattern:
- `WithStackFlag(*[]flags.Option)` - wrapper that appends stack flag options
- `WithFormatFlag(*[]flags.Option)` - wrapper that appends format flag options

### 3. How do I handle flags only needed by specific commands?

**Create command-specific flag wrappers and only use them where needed.**

**Example - Components-only flags:**

```go
// cmd/list/flag_wrappers.go
func WithTypeFlag(options *[]flags.Option) {
    // Component type filter (real/abstract/all)
    // ONLY used by: components command
}

func WithEnabledFlag(options *[]flags.Option) {
    // Enabled filter
    // ONLY used by: components command
}

func WithLockedFlag(options *[]flags.Option) {
    // Locked filter
    // ONLY used by: components command
}
```

**Usage in components.go:**
```go
func init() {
    componentsParser = NewListParser(
        WithFormatFlag,      // Universal flag
        WithColumnsFlag,     // Universal flag
        WithSortFlag,        // Universal flag
        WithStackFlag,       // Universal flag
        WithTypeFlag,        // ✓ Components-specific
        WithEnabledFlag,     // ✓ Components-specific
        WithLockedFlag,      // ✓ Components-specific
    )
}
```

**Usage in stacks.go (doesn't need component-specific flags):**
```go
func init() {
    stacksParser = NewListParser(
        WithFormatFlag,      // Universal flag
        WithColumnsFlag,     // Universal flag
        WithSortFlag,        // Universal flag
        WithComponentFlag,   // ✓ Stacks-specific (filter stacks by component)
        // NO type, enabled, locked - they don't apply to stacks
    )
}
```

**Benefits:**
- Each command composes ONLY the flags it needs
- No unused flags registered
- Clear which flags apply to which commands
- Prevents user confusion (e.g., `--locked` doesn't appear on `atmos list stacks --help`)

### 4. Should wrapper functions set default values?

**YES - Wrapper functions should set default values.**

Each wrapper is the single source of truth for that flag's configuration, including:
- Default value
- Description
- Environment variable bindings
- Shorthand
- Valid values (if applicable)

**Example:**
```go
func WithTypeFlag(options *[]flags.Option) {
    *options = append(*options,
        flags.WithStringFlag("type", "t", "real", "Component type: real, abstract, all"),
        //                                  ^^^^^ Default value set here
        flags.WithEnvVars("type", "ATMOS_COMPONENT_TYPE"),
        flags.WithValidValues("type", "real", "abstract", "all"),
    )
}
```

**Why this matters:**
- **Consistency:** Same default everywhere the flag is used
- **Maintainability:** Change default in one place
- **Documentation:** Default is self-documenting in the code

**Commands can override if needed:**
Commands receive parsed values through Viper and can apply their own logic:

```go
// In command RunE:
v := viper.GetViper()
componentType := v.GetString("type")

// Apply command-specific logic if needed
if componentType == "" {
    componentType = "real"  // Override if you need different default
}
```

But this is rare - usually the wrapper's default is correct for all uses.

### 5. How do commands compose these wrappers?

**Use `NewListParser()` with variadic builder functions.**

**Pattern:**
```go
// cmd/list/components.go
var componentsParser *flags.StandardParser

func init() {
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

    componentsParser.RegisterFlags(componentsCmd)
    _ = componentsParser.BindToViper(viper.GetViper())
}
```

**How it works:**
1. `NewListParser()` creates empty options slice
2. Each `With*Flag` function appends to the slice
3. Returns `*flags.StandardParser` configured with those flags

**Implementation:**
```go
// cmd/list/flag_wrappers.go
func NewListParser(builders ...func(*[]flags.Option)) *flags.StandardParser {
    options := []flags.Option{}

    // Apply each builder function
    for _, builder := range builders {
        builder(&options)  // Builder appends its flags to options
    }

    return flags.NewStandardParser(options...)
}
```

## Command-Specific Examples

### Example 1: Components (Full-Featured)

```go
// cmd/list/components.go
func init() {
    componentsParser = NewListParser(
        // Universal flags
        WithFormatFlag,      // --format table/json/yaml/csv/tsv
        WithColumnsFlag,     // --columns (override atmos.yaml)
        WithSortFlag,        // --sort "stack:asc,component:desc"
        WithFilterFlag,      // --filter (YQ expression)
        WithStackFlag,       // --stack "plat-*-prod"

        // Component-specific flags
        WithTypeFlag,        // --type real/abstract/all
        WithEnabledFlag,     // --enabled=true
        WithLockedFlag,      // --locked=false
    )

    componentsParser.RegisterFlags(componentsCmd)
    _ = componentsParser.BindToViper(viper.GetViper())
}
```

**Flags available:**
- `--format` / `-f` - Output format
- `--columns` - Column selection
- `--sort` - Sorting
- `--filter` - YQ filter
- `--stack` / `-s` - Stack pattern
- `--type` / `-t` - Component type (real/abstract/all)
- `--enabled` - Filter by enabled status
- `--locked` - Filter by locked status

### Example 2: Stacks (Simple)

```go
// cmd/list/stacks.go
func init() {
    stacksParser = NewListParser(
        WithFormatFlag,      // --format
        WithColumnsFlag,     // --columns
        WithSortFlag,        // --sort
        WithComponentFlag,   // --component (filter stacks by component)
    )

    stacksParser.RegisterFlags(stacksCmd)
    _ = stacksParser.BindToViper(viper.GetViper())
}
```

**Flags available:**
- `--format` / `-f` - Output format
- `--columns` - Column selection
- `--sort` - Sorting
- `--component` / `-c` - Filter stacks by component

### Example 3: Values (Complex)

```go
// cmd/list/values.go
func init() {
    valuesParser = NewListParser(
        WithFormatFlag,              // --format
        WithDelimiterFlag,           // --delimiter (CSV/TSV)
        WithMaxColumnsFlag,          // --max-columns
        WithQueryFlag,               // --query (YQ expression)
        WithStackFlag,               // --stack
        WithAbstractFlag,            // --abstract
        WithProcessTemplatesFlag,    // --process-templates
        WithProcessFunctionsFlag,    // --process-functions
    )

    valuesParser.RegisterFlags(valuesCmd)
    _ = valuesParser.BindToViper(viper.GetViper())
}
```

**Flags available:**
- `--format` / `-f` - Output format
- `--delimiter` - CSV/TSV delimiter
- `--max-columns` - Limit columns displayed
- `--query` / `-q` - YQ expression
- `--stack` / `-s` - Stack pattern
- `--abstract` - Include abstract components
- `--process-templates` - Process Go templates
- `--process-functions` - Process template functions

## Flag Mapping Matrix

| Command | Format | Columns | Sort | Filter | Stack | Delimiter | Command-Specific |
|---------|--------|---------|------|--------|-------|-----------|------------------|
| **components** | ✓ | ✓ | ✓ | ✓ | ✓ | - | `--type`, `--enabled`, `--locked` |
| **stacks** | ✓ | ✓ | ✓ | - | - | - | `--component` |
| **workflows** | ✓ | ✓ | ✓ | - | - | ✓ | `--file` |
| **vendor** | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | - |
| **values** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--abstract`, `--process-*` |
| **vars** | ✓ | - | - | - | ✓ | ✓ | Same as values (alias) |
| **metadata** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--process-*` |
| **settings** | ✓ | - | - | - | ✓ | ✓ | `--max-columns`, `--query`, `--process-*` |
| **instances** | ✓ | ✓ | ✓ | - | ✓ | ✓ | `--upload` |

## Environment Variable Bindings

All flags support environment variable configuration:

| Flag | Environment Variable |
|------|---------------------|
| `--format` | `ATMOS_LIST_FORMAT` |
| `--columns` | `ATMOS_LIST_COLUMNS` |
| `--sort` | `ATMOS_LIST_SORT` |
| `--filter` | `ATMOS_LIST_FILTER` |
| `--stack` | `ATMOS_STACK` |
| `--delimiter` | `ATMOS_LIST_DELIMITER` |
| `--type` | `ATMOS_COMPONENT_TYPE` |
| `--enabled` | `ATMOS_COMPONENT_ENABLED` |
| `--locked` | `ATMOS_COMPONENT_LOCKED` |
| `--component` | `ATMOS_COMPONENT` |
| `--query` | `ATMOS_LIST_QUERY` |
| `--max-columns` | `ATMOS_LIST_MAX_COLUMNS` |

**Usage:**
```bash
# Set default format for all list commands
export ATMOS_LIST_FORMAT=json

# Set default component type filter
export ATMOS_COMPONENT_TYPE=real

# Now all list commands use these defaults
atmos list components
atmos list stacks
```

## Best Practices Summary

### DO ✅
- Create one wrapper function per flag
- Follow `With{FlagName}Flag` naming convention
- Set default values in the wrapper
- Compose only the flags your command needs
- Add comprehensive godoc comments
- Add unit tests for each wrapper
- Include environment variable bindings

### DON'T ❌
- Group multiple flags in one wrapper function
- Use inconsistent naming (`addFlag`, `withFlag`, `FlagHelper`)
- Leave default values to command implementation
- Add flags "just in case" - only add what makes sense
- Forget to add tests
- Skip environment variable bindings
- Duplicate flag configuration across commands

## Files Created

1. **`cmd/list/flag_wrappers.go`** - All wrapper functions + `NewListParser()`
2. **`cmd/list/flag_wrappers_test.go`** - Comprehensive test coverage
3. **`cmd/list/flag_wrappers_examples.go`** - Example usage for each command
4. **`cmd/list/FLAG_WRAPPERS.md`** - Complete reference documentation
5. **`cmd/list/IMPLEMENTATION_GUIDE.md`** - This guide (answers to your questions)

## Next Steps

1. **Update existing commands** to use the new wrappers:
   ```go
   // OLD (in components.go)
   componentsParser = flags.NewStandardParser(
       flags.WithStringFlag("stack", "s", "", "Filter by stack"),
       flags.WithEnvVars("stack", "ATMOS_STACK"),
   )

   // NEW
   componentsParser = NewListParser(
       WithStackFlag,  // Much cleaner!
   )
   ```

2. **Add command-specific wrappers** as needed:
   - Create new `With*Flag` functions in `flag_wrappers.go`
   - Add tests in `flag_wrappers_test.go`
   - Update documentation in `FLAG_WRAPPERS.md`

3. **Verify all tests pass:**
   ```bash
   go test ./cmd/list -v
   ```

4. **Build documentation:**
   ```bash
   cd website && npm run build
   ```

## Additional Resources

- **PRD:** `/Users/erik/conductor/atmos/.conductor/lincoln/docs/prd/list-commands-ui-overhaul.md`
- **StandardParser:** `/Users/erik/conductor/atmos/.conductor/lincoln/pkg/flags/standard_parser.go`
- **StandardBuilder:** `/Users/erik/conductor/atmos/.conductor/lincoln/pkg/flags/standard_builder.go`
- **Flag Types:** `/Users/erik/conductor/atmos/.conductor/lincoln/pkg/flags/types.go`
