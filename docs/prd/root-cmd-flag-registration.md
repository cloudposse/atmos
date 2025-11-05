# PRD: Root Command Flag Registration with Builder Pattern

## Problem Statement

Currently, `cmd/root.go` manually registers global flags using direct Cobra/Viper API calls with hardcoded defaults. This creates several issues:

1. **Duplicate defaults** - Defaults exist in both `NewGlobalFlags()` and flag registration
2. **No precedence testing** - Cannot unit test flag precedence without invoking RootCmd
3. **Global mutable state** - RootCmd is a package-level global variable
4. **Inconsistent with patterns** - Every other command uses builders/parsers, but root doesn't

### Current Implementation

```go
// cmd/root.go - Manual flag registration with hardcoded defaults
RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level...")
RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write logs to...")
RootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
// ... 20+ more flags with hardcoded defaults

// Separate manual Viper bindings
viper.BindEnv("force-tty", "ATMOS_FORCE_TTY")
viper.BindEnv("force-color", "ATMOS_FORCE_COLOR", "CLICOLOR_FORCE")
// ... more manual bindings
```

### Issues

**Duplicate Defaults:**
```go
// pkg/flags/global_flags.go
func NewGlobalFlags() GlobalFlags {
    return GlobalFlags{
        LogsLevel: "Warning",  // Default defined here
        LogsFile: "/dev/stderr",
        // ...
    }
}

// cmd/root.go
RootCmd.PersistentFlags().String("logs-level", "Info", "...")  // DUPLICATE default!
```

When we changed LogsLevel default from "Info" to "Warning", we had to update TWO places.

**No Testability:**
```go
// Cannot unit test this without invoking RootCmd
func TestGlobalFlagPrecedence(t *testing.T) {
    // How do we test that --logs-level overrides ATMOS_LOGS_LEVEL?
    // How do we test that ATMOS_LOGS_LEVEL overrides atmos.yaml?
    // Can't - would need to invoke actual RootCmd which is integration test
}
```

**Inconsistent Patterns:**
```go
// Every other command uses builder pattern
terraformParser := flags.NewTerraformOptionsBuilder().
    WithStack(true).
    WithDryRun().
    Build()

// But RootCmd uses manual registration
RootCmd.PersistentFlags().String("logs-level", "Info", "...")
```

## Proposed Solution

Create a `GlobalOptionsBuilder` that follows the same pattern as all other command parsers, allowing RootCmd to use the builder pattern for flag registration.

### Architecture

```go
// pkg/flags/global_builder.go
type GlobalOptionsBuilder struct {
    options []Option
}

func NewGlobalOptionsBuilder() *GlobalOptionsBuilder {
    // Get defaults from NewGlobalFlags() to avoid duplication
    defaults := NewGlobalFlags()

    builder := &GlobalOptionsBuilder{options: []Option{}}

    // Logging configuration
    builder.options = append(builder.options,
        WithStringFlag("logs-level", "", defaults.LogsLevel, "Logs level..."))
    builder.options = append(builder.options,
        WithEnvVars("logs-level", "ATMOS_LOGS_LEVEL"))

    builder.options = append(builder.options,
        WithStringFlag("logs-file", "", defaults.LogsFile, "File to write logs to..."))
    builder.options = append(builder.options,
        WithEnvVars("logs-file", "ATMOS_LOGS_FILE"))

    // ... all other global flags

    return builder
}

func (b *GlobalOptionsBuilder) Build() *StandardFlagParser {
    return NewStandardFlagParser(b.options...)
}
```

### Usage in root.go

```go
// cmd/root.go
var globalParser = flags.NewGlobalOptionsBuilder().Build()

func init() {
    // Register all global flags using builder pattern
    globalParser.RegisterFlags(RootCmd)

    // Bind to viper for precedence handling
    if err := globalParser.BindToViper(viper.GetViper()); err != nil {
        log.Error("Failed to bind global flags to viper", "error", err)
    }
}
```

### Testing

```go
// pkg/flags/global_builder_test.go
func TestGlobalFlagPrecedence(t *testing.T) {
    cmd := &cobra.Command{}
    v := viper.New()

    parser := NewGlobalOptionsBuilder().Build()
    parser.RegisterFlags(cmd)
    parser.BindToViper(v)

    // Test CLI > env > config precedence
    t.Run("CLI overrides everything", func(t *testing.T) {
        v.Set("logs-level", "Debug")  // CLI flag
        v.SetDefault("logs-level", "Warning")  // Default

        flags := ParseGlobalFlags(cmd, v)
        assert.Equal(t, "Debug", flags.LogsLevel)
    })

    t.Run("default used when nothing set", func(t *testing.T) {
        v.SetDefault("logs-level", "Warning")

        flags := ParseGlobalFlags(cmd, v)
        assert.Equal(t, "Warning", flags.LogsLevel)
    })
}
```

## Implementation Blockers

### 1. String Slice Flags Not Supported

The options system currently lacks `WithStringSliceFlag()`:

```go
// Doesn't exist yet - BLOCKER
builder.options = append(builder.options,
    WithStringSliceFlag("config", "", defaults.Config, "Paths to config files"))
```

**Required Work:**
- Add `WithStringSliceFlag()` to `pkg/flags/options.go`
- Update `StandardFlagParser.RegisterFlags()` to handle string slice flags
- Add tests for string slice flag precedence

### 2. Special Flag Handling

Some global flags have special requirements:

```go
// Chdir has shorthand "C"
WithStringFlag("chdir", "C", defaults.Chdir, "...")

// Pager and Identity use NoOptDefVal pattern
WithStringFlag("pager", "", defaults.Pager.Value(), "...")
WithNoOptDefVal("pager", "__AUTO__")

WithStringFlag("identity", "", defaults.Identity.Value(), "...")
WithNoOptDefVal("identity", "__SELECT__")
```

**Required Work:**
- Ensure builder supports shorthand flags
- Ensure NoOptDefVal works with builder pattern
- Handle PagerSelector and IdentitySelector properly

## Implementation Plan

### Phase 1: Extend Options System
1. Add `WithStringSliceFlag()` to options.go
2. Update StandardFlagParser to register string slice flags
3. Add tests for string slice flag handling

### Phase 2: Create GlobalOptionsBuilder
1. Implement `pkg/flags/global_builder.go`
2. Add all global flag definitions using defaults from NewGlobalFlags()
3. Handle special cases (NoOptDefVal, shorthand, selectors)

### Phase 3: Update root.go
1. Replace manual flag registration with builder usage
2. Remove duplicate environment variable bindings (handled by builder)
3. Remove hardcoded defaults (use NewGlobalFlags() values)

### Phase 4: Testing
1. Add unit tests for GlobalOptionsBuilder
2. Add precedence tests (CLI > env > config > default)
3. Verify all global flags work correctly with builder

## Benefits

1. **Single source of truth** - Defaults only in NewGlobalFlags()
2. **Testable** - Can unit test flag precedence without invoking RootCmd
3. **Consistent** - Same pattern as all other commands
4. **Type-safe** - Compiler catches missing flags
5. **DRY** - No duplicate env var bindings

## Acceptance Criteria

- [ ] GlobalOptionsBuilder implemented with all global flags
- [ ] root.go uses builder pattern instead of manual registration
- [ ] No duplicate defaults between NewGlobalFlags() and flag registration
- [ ] Unit tests verify flag precedence (CLI > env > config > default)
- [ ] All global flags work correctly after migration
- [ ] Documentation updated to explain builder pattern for root flags

## Future Considerations

This pattern should be the standard for ALL command flag registration in Atmos. Any new global flags should be added to GlobalOptionsBuilder, not manually registered in root.go.

## References

- Existing builder implementations: `TerraformOptionsBuilder`, `HelmfileOptionsBuilder`, `EditorConfigOptionsBuilder`
- Global flags definition: `pkg/flags/global_flags.go`
- Global flags parsing: `pkg/flags/global_parser.go`
- Current root.go implementation: `cmd/root.go` lines 678-718
