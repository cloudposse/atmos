# PRD: Global Flags Pattern for Strongly-Typed Interpreters

**Status**: Implemented

**Note**: This document uses "Interpreter" terminology throughout. The actual implementation uses "Options" (e.g., `TerraformOptions`, `StandardOptions`) instead of `TerraformInterpreter`. The pattern and architecture described here remain accurate - just substitute "Options" for "Interpreter" when reading code examples.

## Problem Statement

Atmos has **13+ global flags** defined on `RootCmd.PersistentFlags()` that should be available to all commands:

```go
// Current global flags in cmd/root.go
--chdir, -C           // Change working directory
--redirect-stderr     // Redirect stderr
--version             // Display version
--logs-level          // Logging level
--logs-file           // Log file path
--base-path           // Base path for project
--config              // Config file paths
--config-path         // Config directory paths
--no-color            // Disable color
--pager               // Enable/disable pager
--profiler-enabled    // Enable profiler
--profiler-port       // Profiler port
--profiler-host       // Profiler host
--profile-file        // Profile output file
--profile-type        // Profile type
--heatmap             // Show heatmap
--heatmap-mode        // Heatmap mode
```

**Challenge**: With strongly-typed interpreters, how do we provide these global flags to ALL command interpreters without:
1. Duplicating the same 13+ fields in every interpreter struct
2. Breaking DRY principles
3. Losing type safety
4. Complicating the interpreter structs

## Goals

1. **DRY**: Define global flags ONCE
2. **Type Safety**: Maintain strongly-typed access
3. **Inheritance**: All interpreters automatically get global flags
4. **Simplicity**: Clean, maintainable code
5. **Extensibility**: Easy to add new global flags

## Solution: Embedded GlobalFlags Struct

### Design Pattern

Use **struct embedding** (composition) to provide global flags to all interpreters:

```go
// pkg/flags/global_flags.go

// Flags contains all persistent flags available to every command.
// These flags are inherited from RootCmd.PersistentFlags() and should be embedded
// in all command interpreters using Go struct embedding.
type Flags struct {
    // Working directory and path configuration.
    Chdir      string
    BasePath   string
    Config     []string // Config file paths.
    ConfigPath []string // Config directory paths.

    // Logging configuration.
    LogsLevel string
    LogsFile  string
    NoColor   bool

    // Terminal and I/O configuration.
    ForceColor bool // Force color output even when not a TTY (--force-color).
    ForceTTY   bool // Force TTY mode with sane defaults (--force-tty).
    Mask       bool // Enable automatic masking of sensitive data (--mask).

    // Output configuration.
    Pager PagerSelector

    // Authentication.
    Identity IdentitySelector

    // Profiling configuration.
    ProfilerEnabled bool
    ProfilerPort    int
    ProfilerHost    string
    ProfileFile     string
    ProfileType     string

    // Performance visualization.
    Heatmap     bool
    HeatmapMode string

    // System configuration.
    RedirectStderr string
    Version        bool
}

// PagerSelector handles the pager flag which has three states:
// 1. Not set (use config/env default)
// 2. Set without value (enable with default pager)
// 3. Set with value (use specific pager or disable)
type PagerSelector struct {
    value    string
    provided bool
}

// IsEnabled returns true if pager should be enabled.
func (p PagerSelector) IsEnabled() bool {
    if !p.provided {
        return false // Not set, use default from config
    }
    return p.value != "false"
}

// Pager returns the pager command to use.
// Returns empty string if using default or if disabled.
func (p PagerSelector) Pager() string {
    if p.value == "true" || p.value == "" {
        return "" // Use default pager
    }
    return p.value // Specific pager (e.g., "less")
}

// IsProvided returns true if the flag was explicitly set.
func (p PagerSelector) IsProvided() bool {
    return p.provided
}
```

### Usage: Embedding in Command Interpreters

Every command interpreter embeds `GlobalFlags`:

```go
// pkg/flags/terraform_interpreter.go

type TerraformInterpreter struct {
    GlobalFlags // Embedded - provides all global flags

    // Terraform-specific flags
    Stack        string
    Identity     IdentitySelector
    DryRun       bool
    SkipInit     bool
    FromPlan     string
    UploadStatus bool

    // Parsed structure
    Subcommand string
    Component  string

    // Pass-through args
    positionalArgs  []string
    passThroughArgs []string
}

// Usage example:
interpreter.LogsLevel      // ✅ From GlobalFlags
interpreter.Pager          // ✅ From GlobalFlags
interpreter.Stack          // ✅ From TerraformInterpreter
interpreter.DryRun         // ✅ From TerraformInterpreter
```

```go
// pkg/flags/helmfile_interpreter.go

type HelmfileInterpreter struct {
    GlobalFlags // Embedded - same global flags

    // Helmfile-specific flags
    Stack     string
    Identity  IdentitySelector
    DryRun    bool
    Component string

    positionalArgs  []string
    passThroughArgs []string
}

// Usage example:
interpreter.LogsLevel      // ✅ From GlobalFlags
interpreter.NoColor        // ✅ From GlobalFlags
interpreter.Stack          // ✅ From HelmfileInterpreter
```

```go
// pkg/flags/custom_interpreter.go

type CustomCommandInterpreter struct {
    GlobalFlags // Embedded - same global flags

    // Dynamic flags from atmos.yaml
    Flags map[string]interface{}

    positionalArgs  []string
    passThroughArgs []string
}

// Usage example:
interpreter.LogsLevel                  // ✅ From GlobalFlags
interpreter.Flags["environment"]       // ✅ From CustomCommandInterpreter
```

### Base Interface

Remove `BaseInterpreter` struct, use interface instead:

```go
// pkg/flags/interpreter.go

// CommandInterpreter is the base interface all interpreters implement.
type CommandInterpreter interface {
    // Access to global flags (all interpreters have these)
    GetGlobalFlags() *GlobalFlags

    // Access to positional and pass-through args
    GetPositionalArgs() []string
    GetPassThroughArgs() []string
}

// All interpreters implement this automatically via embedding.
```

### Parser Implementation

Parser constructs `GlobalFlags` once, shares across all interpreters:

```go
// pkg/flags/parser.go

type baseParser struct {
    registry *FlagRegistry
    viper    *viper.Viper
    cmd      *cobra.Command
}

// parseGlobalFlags extracts global flags with precedence.
// This is called ONCE per command execution.
func (p *baseParser) parseGlobalFlags() GlobalFlags {
    return GlobalFlags{
        Chdir:      p.viper.GetString("chdir"),
        BasePath:   p.viper.GetString("base-path"),
        Config:     p.viper.GetStringSlice("config"),
        ConfigPath: p.viper.GetStringSlice("config-path"),

        LogsLevel: p.viper.GetString("logs-level"),
        LogsFile:  p.viper.GetString("logs-file"),
        NoColor:   p.viper.GetBool("no-color"),

        Pager: p.parsePagerFlag(),

        ProfilerEnabled: p.viper.GetBool("profiler-enabled"),
        ProfilerPort:    p.viper.GetInt("profiler-port"),
        ProfilerHost:    p.viper.GetString("profiler-host"),
        ProfileFile:     p.viper.GetString("profile-file"),
        ProfileType:     p.viper.GetString("profile-type"),

        Heatmap:     p.viper.GetBool("heatmap"),
        HeatmapMode: p.viper.GetString("heatmap-mode"),

        RedirectStderr: p.viper.GetString("redirect-stderr"),
        Version:        p.viper.GetBool("version"),
    }
}

// parsePagerFlag handles the special pager flag with NoOptDefVal.
func (p *baseParser) parsePagerFlag() PagerSelector {
    flag := p.cmd.Flags().Lookup("pager")
    if flag == nil {
        return PagerSelector{}
    }

    // Check if flag was explicitly set
    if p.cmd.Flags().Changed("pager") {
        value := p.viper.GetString("pager")
        return PagerSelector{value: value, provided: true}
    }

    // Fall back to env/config
    if p.viper.IsSet("pager") {
        value := p.viper.GetString("pager")
        return PagerSelector{value: value, provided: true}
    }

    return PagerSelector{provided: false}
}
```

```go
// pkg/flags/terraform_parser.go

func (p *TerraformParser) Parse(ctx context.Context, args []string) (*TerraformInterpreter, error) {
    // Step 1: Parse global flags (inherited from RootCmd)
    globalFlags := p.parseGlobalFlags()

    // Step 2: Extract CLI flags and args
    positionals, passthrough, err := p.extractArgs(args)
    if err != nil {
        return nil, err
    }

    // Step 3: Universal precedence resolution for Terraform-specific flags
    resolvedFlags := p.parseWithPrecedence(p.registry, p.viper)

    // Step 4: Build strongly-typed interpreter
    interpreter := &TerraformInterpreter{
        GlobalFlags: globalFlags, // Embed global flags

        // Terraform-specific fields
        Stack:        resolvedFlags["stack"].(string),
        Identity:     p.parseIdentityFlag(resolvedFlags),
        DryRun:       resolvedFlags["dry-run"].(bool),
        SkipInit:     resolvedFlags["skip-init"].(bool),
        FromPlan:     resolvedFlags["from-plan"].(string),
        UploadStatus: resolvedFlags["upload-status"].(bool),

        Subcommand: positionals[0],
        Component:  positionals[1],

        positionalArgs:  positionals,
        passThroughArgs: passthrough,
    }

    return interpreter, nil
}
```

### Command Usage

Commands access both global and command-specific flags naturally:

```go
// cmd/terraform_utils.go

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
    interpreter, err := terraformParser.Parse(ctx, args)
    if err != nil {
        return err
    }

    // ✅ Access global flags
    if interpreter.Version {
        return showVersion()
    }

    if interpreter.NoColor {
        disableColorOutput()
    }

    setupLogger(interpreter.LogsLevel, interpreter.LogsFile)

    // ✅ Access Terraform-specific flags
    info.Stack = interpreter.Stack
    info.DryRun = interpreter.DryRun

    // ✅ Handle identity
    if interpreter.Identity.IsInteractiveSelector() {
        handleInteractiveIdentitySelection(&info)
    }

    // ✅ Access global profiling flags
    if interpreter.ProfilerEnabled {
        startProfiler(interpreter.ProfilerHost, interpreter.ProfilerPort)
    }

    return executor.Execute(ctx, info, interpreter.passThroughArgs)
}
```

### Benefits

1. **DRY**: Global flags defined ONCE in `GlobalFlags` struct
2. **Zero Duplication**: All interpreters get global flags via embedding
3. **Type Safety**: `interpreter.LogsLevel` not `interpreter.GetString("logs-level")`
4. **IDE Support**: Autocomplete shows all available flags (global + command-specific)
5. **Easy Extension**: Add new global flag in one place, available everywhere
6. **Clear Separation**: Global vs. command-specific flags are obvious in code
7. **Testing**: Easy to mock - just populate `GlobalFlags{}`

### Testing

```go
// pkg/flags/terraform_interpreter_test.go

func TestTerraformInterpreter_GlobalFlags(t *testing.T) {
    interpreter := &TerraformInterpreter{
        GlobalFlags: GlobalFlags{
            LogsLevel: "Debug",
            NoColor:   true,
            Heatmap:   true,
        },
        Stack:  "test-stack",
        DryRun: true,
    }

    // ✅ Test global flags
    assert.Equal(t, "Debug", interpreter.LogsLevel)
    assert.True(t, interpreter.NoColor)
    assert.True(t, interpreter.Heatmap)

    // ✅ Test command-specific flags
    assert.Equal(t, "test-stack", interpreter.Stack)
    assert.True(t, interpreter.DryRun)
}
```

### Adding New Global Flags

**Step 1**: Add to `GlobalFlags` struct:

```go
type GlobalFlags struct {
    // ... existing fields ...

    // NEW: Add global timeout flag
    Timeout time.Duration
}
```

**Step 2**: Update `parseGlobalFlags()`:

```go
func (p *baseParser) parseGlobalFlags() GlobalFlags {
    return GlobalFlags{
        // ... existing fields ...

        Timeout: p.viper.GetDuration("timeout"),
    }
}
```

**Step 3**: Register flag in `cmd/root.go`:

```go
RootCmd.PersistentFlags().Duration("timeout", 0, "Global timeout for all operations")
```

**Done!** All interpreters now have `interpreter.Timeout` available.

## Alternative: Interface with Getters (Rejected)

**Why not this approach?**

```go
// ❌ Too verbose, loses type safety benefits
type GlobalFlagsProvider interface {
    GetLogsLevel() string
    GetLogsFile() string
    GetNoColor() bool
    GetPager() PagerSelector
    // ... 13+ getter methods
}
```

**Problems:**
- Verbose: 13+ getter methods
- Boilerplate: Every interpreter needs to implement all getters
- Delegation: Each getter just calls `p.globalFlags.Field`
- No IDE benefit: Autocomplete shows methods, not fields
- Testing harder: Must mock all getter methods

**Embedding wins**: Direct field access, zero boilerplate, full type safety.

## Alternative: Helper Functions (Rejected)

**Why not this approach?**

```go
// ❌ Loses encapsulation, breaks type safety
func GetLogsLevel(interpreter CommandInterpreter) string {
    // ... reflection or type assertion
}
```

**Problems:**
- Runtime overhead (reflection or type assertions)
- No compile-time safety
- Breaks encapsulation
- Awkward API: `GetLogsLevel(interpreter)` vs `interpreter.LogsLevel`

## Implementation Checklist

Phase 1: Create Infrastructure
- [ ] Create `pkg/flags/global_flags.go`
- [ ] Define `GlobalFlags` struct
- [ ] Define `PagerSelector` type
- [ ] Add `parseGlobalFlags()` to base parser
- [ ] Add unit tests for GlobalFlags

Phase 2: Update Interpreters
- [ ] Update `TerraformInterpreter` to embed `GlobalFlags`
- [ ] Update `HelmfileInterpreter` to embed `GlobalFlags`
- [ ] Update `PackerInterpreter` to embed `GlobalFlags`
- [ ] Update `CustomCommandInterpreter` to embed `GlobalFlags`
- [ ] Update all standard command interpreters

Phase 3: Update Command Code
- [ ] Update terraform command to use `interpreter.GlobalFlags`
- [ ] Update helmfile command to use `interpreter.GlobalFlags`
- [ ] Update packer command to use `interpreter.GlobalFlags`
- [ ] Update all standard commands

Phase 4: Testing
- [ ] Unit tests for global flag parsing
- [ ] Integration tests with global + command flags
- [ ] Verify precedence works correctly
- [ ] Test PagerSelector behavior

Phase 5: Documentation
- [ ] Update PRD with global flags pattern
- [ ] Add examples to CLAUDE.md
- [ ] Document PagerSelector usage
- [ ] Add testing examples

## Example: Full Terraform Interpreter

```go
type TerraformInterpreter struct {
    // ✅ Global flags (13+ fields) - zero duplication!
    GlobalFlags

    // ✅ Terraform-specific flags (6 fields)
    Stack        string
    Identity     IdentitySelector
    DryRun       bool
    SkipInit     bool
    FromPlan     string
    UploadStatus bool

    // ✅ Parsed structure
    Subcommand string
    Component  string

    // ✅ Args
    positionalArgs  []string
    passThroughArgs []string
}

// ✅ Usage in command
func terraformRun(cmd *cobra.Command, args []string) error {
    interpreter, _ := parser.Parse(ctx, args)

    // Global flags
    log.SetLevel(interpreter.LogsLevel)
    log.SetOutput(interpreter.LogsFile)
    if interpreter.NoColor {
        color.Disable()
    }

    // Terraform flags
    info := schema.ConfigAndStacksInfo{
        Stack:  interpreter.Stack,
        DryRun: interpreter.DryRun,
    }

    // Identity handling
    if interpreter.Identity.IsInteractiveSelector() {
        selectIdentity(&info)
    }

    return execute(ctx, info, interpreter.passThroughArgs)
}
```

## Conclusion

**Recommendation**: Use **struct embedding** for global flags.

**Why:**
- ✅ DRY: Define once, use everywhere
- ✅ Type safety: Compile-time checks
- ✅ Zero boilerplate: No getter methods needed
- ✅ IDE friendly: Autocomplete works perfectly
- ✅ Simple: Easy to understand and maintain
- ✅ Extensible: Add global flags in one place

**Timeline**: 1-2 days to implement across all interpreters

**Risk**: LOW - Non-breaking change, backward compatible
