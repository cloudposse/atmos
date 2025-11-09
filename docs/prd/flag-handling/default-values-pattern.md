# PRD: Default Values for Strongly-Typed Interpreters

## Problem Statement

Flags need default values that work consistently across all layers:
- **CLI flags**: Cobra default (e.g., `--logs-level "Info"`)
- **Environment variables**: Viper binding (e.g., `ATMOS_LOGS_LEVEL`)
- **Config files**: `atmos.yaml` settings (e.g., `logs.level: Debug`)
- **Code defaults**: Go zero values or hardcoded defaults

**Challenge**: How do default values flow through the strongly-typed interpreter system while maintaining precedence?

## Current State

### Flag Type System (Already Implemented)

```go
// pkg/flags/types.go

type Flag interface {
    GetName() string
    GetShorthand() string
    GetDescription() string
    GetDefault() interface{}     // ✅ Already has default!
    IsRequired() bool
    GetNoOptDefVal() string
    GetEnvVars() []string
}

type StringFlag struct {
    Name        string
    Shorthand   string
    Default     string          // ✅ Default value
    Description string
    Required    bool
    NoOptDefVal string
    EnvVars     []string
}

type BoolFlag struct {
    Name        string
    Shorthand   string
    Default     bool            // ✅ Default value
    Description string
    Required    bool
    EnvVars     []string
}

type IntFlag struct {
    Name        string
    Shorthand   string
    Default     int             // ✅ Default value
    Description string
    Required    bool
    EnvVars     []string
}
```

### Example: Global Flags with Defaults (cmd/root.go)

```go
// String flag with default
RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level...")
//                                             ^^^^^^ Default = "Info"

// String flag with default
RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "Log file path...")
//                                            ^^^^^^^^^^^^^ Default = "/dev/stderr"

// Bool flag with default
RootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
//                                        ^^^^^^ Default = false

// Int flag with default
RootCmd.PersistentFlags().Int("profiler-port", profiler.DefaultProfilerPort, "Port...")
//                                            ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^ Default = 6060

// String flag with default
RootCmd.PersistentFlags().String("heatmap-mode", "bar", "Heatmap mode...")
//                                               ^^^^^ Default = "bar"

// Empty string defaults
RootCmd.PersistentFlags().String("chdir", "", "Change working directory...")
//                                       ^^ Default = "" (empty)

RootCmd.PersistentFlags().String("base-path", "", "Base path...")
//                                           ^^ Default = "" (empty)
```

## Goals

1. **Consistent defaults**: Same default value in all representations
2. **Precedence preservation**: CLI > ENV > config > **defaults**
3. **Type safety**: Defaults match field types
4. **DRY**: Define default once, use everywhere
5. **Zero values**: Handle Go zero values appropriately
6. **Documentation**: Defaults visible in help text

## Solution: Four-Layer Default System

### Layer 1: Flag Registration (Cobra Level)

Defaults are registered when flags are created:

```go
// cmd/root.go

func init() {
    // ✅ Defaults specified in flag registration
    RootCmd.PersistentFlags().String("logs-level", "Info", "...")
    RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "...")
    RootCmd.PersistentFlags().Bool("no-color", false, "...")
    RootCmd.PersistentFlags().Int("profiler-port", 6060, "...")
}
```

### Layer 2: FlagRegistry (Parser Level)

Defaults stored in flag definitions:

```go
// pkg/flags/registry.go

func CommonFlags() *FlagRegistry {
    registry := NewFlagRegistry()

    // ✅ Defaults in StringFlag
    registry.Register(&StringFlag{
        Name:        "stack",
        Shorthand:   "s",
        Default:     "",           // Default = empty (required flag)
        Description: "Stack name",
        EnvVars:     []string{"ATMOS_STACK"},
    })

    // ✅ Defaults in BoolFlag
    registry.Register(&BoolFlag{
        Name:        "dry-run",
        Shorthand:   "",
        Default:     false,        // Default = false
        Description: "Dry run mode",
        EnvVars:     []string{"ATMOS_DRY_RUN"},
    })

    return registry
}
```

### Layer 3: Viper (Precedence Resolution)

Viper automatically handles defaults via Cobra binding:

```go
// When you register a flag with Cobra:
cmd.Flags().String("logs-level", "Info", "...")

// And bind to Viper:
viper.BindPFlag("logs-level", cmd.Flags().Lookup("logs-level"))

// Viper AUTOMATICALLY uses the flag's default value as its default!
// No need to call viper.SetDefault() separately
```

**Precedence order (Viper handles automatically):**
1. CLI flag value (if provided)
2. ENV var value (if set)
3. Config file value (if present)
4. **Flag default** (from Cobra registration)
5. Go zero value (if no default)

### Layer 4: GlobalFlags Struct (Interpreter Level)

Defaults flow through to strongly-typed fields:

```go
// pkg/flags/global_flags.go

type GlobalFlags struct {
    // String defaults
    LogsLevel string  // Default: "Info" (from flag registration)
    LogsFile  string  // Default: "/dev/stderr" (from flag registration)
    Chdir     string  // Default: "" (empty)

    // Bool defaults
    NoColor         bool  // Default: false
    ProfilerEnabled bool  // Default: false
    Heatmap         bool  // Default: false

    // Int defaults
    ProfilerPort int  // Default: 6060

    // Complex defaults (special types)
    Identity IdentitySelector  // Default: empty (not provided)
    Pager    PagerSelector     // Default: empty (not provided)
}
```

## Implementation Pattern

### Registration with Defaults

```go
// pkg/flags/global_registry.go (NEW)

// GlobalFlagsRegistry returns a registry with all global flags pre-configured.
func GlobalFlagsRegistry() *FlagRegistry {
    registry := NewFlagRegistry()

    // Working directory flags
    registry.Register(&StringFlag{
        Name:        "chdir",
        Shorthand:   "C",
        Default:     "",  // Default: empty (optional)
        Description: "Change working directory before processing",
        EnvVars:     []string{"ATMOS_CHDIR"},
    })

    registry.Register(&StringFlag{
        Name:        "base-path",
        Shorthand:   "",
        Default:     "",  // Default: empty (discovered from atmos.yaml)
        Description: "Base path for Atmos project",
        EnvVars:     []string{"ATMOS_BASE_PATH"},
    })

    // Logging flags
    registry.Register(&StringFlag{
        Name:        "logs-level",
        Shorthand:   "",
        Default:     "Info",  // Default: Info level
        Description: "Logs level (Trace, Debug, Info, Warning, Off)",
        EnvVars:     []string{"ATMOS_LOGS_LEVEL"},
    })

    registry.Register(&StringFlag{
        Name:        "logs-file",
        Shorthand:   "",
        Default:     "/dev/stderr",  // Default: stderr
        Description: "File to write logs to",
        EnvVars:     []string{"ATMOS_LOGS_FILE"},
    })

    registry.Register(&BoolFlag{
        Name:        "no-color",
        Shorthand:   "",
        Default:     false,  // Default: color enabled
        Description: "Disable color output",
        EnvVars:     []string{"ATMOS_NO_COLOR", "NO_COLOR"},
    })

    // Profiling flags
    registry.Register(&BoolFlag{
        Name:        "profiler-enabled",
        Shorthand:   "",
        Default:     false,  // Default: profiler disabled
        Description: "Enable pprof profiling server",
        EnvVars:     []string{"ATMOS_PROFILER_ENABLED"},
    })

    registry.Register(&IntFlag{
        Name:        "profiler-port",
        Shorthand:   "",
        Default:     6060,  // Default: 6060
        Description: "Port for pprof profiling server",
        EnvVars:     []string{"ATMOS_PROFILER_PORT"},
    })

    registry.Register(&StringFlag{
        Name:        "profiler-host",
        Shorthand:   "",
        Default:     "localhost",  // Default: localhost
        Description: "Host for pprof profiling server",
        EnvVars:     []string{"ATMOS_PROFILER_HOST"},
    })

    // Performance flags
    registry.Register(&BoolFlag{
        Name:        "heatmap",
        Shorthand:   "",
        Default:     false,  // Default: heatmap disabled
        Description: "Show performance heatmap visualization",
        EnvVars:     []string{"ATMOS_HEATMAP"},
    })

    registry.Register(&StringFlag{
        Name:        "heatmap-mode",
        Shorthand:   "",
        Default:     "bar",  // Default: bar mode
        Description: "Heatmap visualization mode (bar, sparkline, table)",
        EnvVars:     []string{"ATMOS_HEATMAP_MODE"},
    })

    // Identity flag (special handling)
    registry.Register(&StringFlag{
        Name:        "identity",
        Shorthand:   "i",
        Default:     "",  // Default: empty (use default identity from config)
        Description: "Identity to assume. Use without value to select interactively.",
        NoOptDefVal: cfg.IdentityFlagSelectValue,  // "__SELECT__"
        EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
    })

    // Pager flag (special handling)
    registry.Register(&StringFlag{
        Name:        "pager",
        Shorthand:   "",
        Default:     "",  // Default: empty (use config default)
        Description: "Enable pager for output",
        NoOptDefVal: "true",  // When used alone, enable with default pager
        EnvVars:     []string{"ATMOS_PAGER"},
    })

    return registry
}
```

### Parser Applies Defaults via Viper

```go
// pkg/flags/parser.go

// parseGlobalFlags extracts global flags with defaults applied.
func (p *baseParser) parseGlobalFlags(cmd *cobra.Command) GlobalFlags {
    // Viper automatically applies precedence:
    // CLI > ENV > config > flag default > zero value

    return GlobalFlags{
        // ✅ Viper returns default if no other value provided
        Chdir:      p.viper.GetString("chdir"),           // Default: ""
        BasePath:   p.viper.GetString("base-path"),       // Default: ""
        LogsLevel:  p.viper.GetString("logs-level"),      // Default: "Info"
        LogsFile:   p.viper.GetString("logs-file"),       // Default: "/dev/stderr"
        NoColor:    p.viper.GetBool("no-color"),          // Default: false

        ProfilerEnabled: p.viper.GetBool("profiler-enabled"),  // Default: false
        ProfilerPort:    p.viper.GetInt("profiler-port"),      // Default: 6060
        ProfilerHost:    p.viper.GetString("profiler-host"),   // Default: "localhost"

        Heatmap:     p.viper.GetBool("heatmap"),              // Default: false
        HeatmapMode: p.viper.GetString("heatmap-mode"),       // Default: "bar"

        Identity: p.parseIdentityFlag(cmd),  // Default: IdentitySelector{provided: false}
        Pager:    p.parsePagerFlag(cmd),     // Default: PagerSelector{provided: false}

        // ... other fields
    }
}
```

### Interpreter Uses Defaults Automatically

```go
// Usage in commands

func terraformRun(cmd *cobra.Command, args []string) error {
    interpreter, err := terraformParser.Parse(ctx, args)
    if err != nil {
        return err
    }

    // ✅ If user didn't provide --logs-level, interpreter.LogsLevel == "Info" (default)
    log.SetLevel(interpreter.LogsLevel)

    // ✅ If user didn't provide --profiler-port, interpreter.ProfilerPort == 6060 (default)
    if interpreter.ProfilerEnabled {
        startProfiler(interpreter.ProfilerHost, interpreter.ProfilerPort)
    }

    // ✅ If user didn't provide --heatmap-mode, interpreter.HeatmapMode == "bar" (default)
    if interpreter.Heatmap {
        showHeatmap(interpreter.HeatmapMode)
    }

    return nil
}
```

## Default Value Types

### 1. Empty String Defaults (Optional Flags)

```go
// Flags where empty is valid (not required)
registry.Register(&StringFlag{
    Name:    "chdir",
    Default: "",  // ✅ Empty means "don't change directory"
})

registry.Register(&StringFlag{
    Name:    "base-path",
    Default: "",  // ✅ Empty means "discover from atmos.yaml"
})

registry.Register(&StringFlag{
    Name:    "stack",
    Default: "",  // ✅ Empty means "no stack specified" (may be required by command)
})
```

### 2. Non-Empty String Defaults (Meaningful Defaults)

```go
// Flags with specific default values
registry.Register(&StringFlag{
    Name:    "logs-level",
    Default: "Info",  // ✅ Info is the default logging level
})

registry.Register(&StringFlag{
    Name:    "logs-file",
    Default: "/dev/stderr",  // ✅ Log to stderr by default
})

registry.Register(&StringFlag{
    Name:    "profiler-host",
    Default: "localhost",  // ✅ Bind to localhost by default
})

registry.Register(&StringFlag{
    Name:    "heatmap-mode",
    Default: "bar",  // ✅ Bar chart is default visualization
})
```

### 3. Boolean Defaults (false = feature disabled)

```go
// Opt-in features (default: false)
registry.Register(&BoolFlag{
    Name:    "no-color",
    Default: false,  // ✅ Color enabled by default
})

registry.Register(&BoolFlag{
    Name:    "profiler-enabled",
    Default: false,  // ✅ Profiler disabled by default
})

registry.Register(&BoolFlag{
    Name:    "heatmap",
    Default: false,  // ✅ Heatmap disabled by default
})

registry.Register(&BoolFlag{
    Name:    "dry-run",
    Default: false,  // ✅ Real execution by default
})
```

### 4. Integer Defaults (Numeric Configuration)

```go
// Numeric configuration with sensible defaults
registry.Register(&IntFlag{
    Name:    "profiler-port",
    Default: 6060,  // ✅ Standard pprof port
})

registry.Register(&IntFlag{
    Name:    "timeout",
    Default: 300,  // ✅ 5 minutes default timeout
})
```

### 5. Special Type Defaults (Complex State)

```go
// IdentitySelector default (not provided)
Identity: IdentitySelector{
    value:    "",
    provided: false,  // ✅ Default: no identity specified
}

// PagerSelector default (not provided)
Pager: PagerSelector{
    value:    "",
    provided: false,  // ✅ Default: use config setting
}
```

## Default Value Sources

### Priority Order (Highest to Lowest)

1. **CLI flag**: `atmos terraform plan --logs-level Debug`
2. **Environment variable**: `ATMOS_LOGS_LEVEL=Debug atmos terraform plan`
3. **Config file**: `logs.level: Debug` in `atmos.yaml`
4. **Flag default**: From flag registration (`"Info"`)
5. **Go zero value**: `""`, `false`, `0` (if no default specified)

### Example: --logs-level Precedence

```go
// Scenario 1: User provides CLI flag
// Command: atmos terraform plan --logs-level Trace
interpreter.LogsLevel == "Trace"  // ✅ From CLI flag

// Scenario 2: User sets ENV var
// Command: ATMOS_LOGS_LEVEL=Debug atmos terraform plan
interpreter.LogsLevel == "Debug"  // ✅ From ENV var

// Scenario 3: User has config file
// atmos.yaml: logs.level: Warning
// Command: atmos terraform plan
interpreter.LogsLevel == "Warning"  // ✅ From config file

// Scenario 4: Nothing specified (use default)
// Command: atmos terraform plan
interpreter.LogsLevel == "Info"  // ✅ From flag default registration

// Scenario 5: No default specified (hypothetical)
// If we had NOT set Default: "Info", then:
interpreter.LogsLevel == ""  // ❌ Go zero value (empty string)
```

## Testing Defaults

### Unit Tests

```go
// pkg/flags/global_flags_test.go

func TestGlobalFlagsDefaults(t *testing.T) {
    tests := []struct {
        name     string
        flags    GlobalFlags
        expected map[string]interface{}
    }{
        {
            name:  "all defaults",
            flags: GlobalFlags{
                LogsLevel:       "Info",
                LogsFile:        "/dev/stderr",
                NoColor:         false,
                ProfilerEnabled: false,
                ProfilerPort:    6060,
                ProfilerHost:    "localhost",
                Heatmap:         false,
                HeatmapMode:     "bar",
            },
            expected: map[string]interface{}{
                "logs-level":       "Info",
                "logs-file":        "/dev/stderr",
                "no-color":         false,
                "profiler-enabled": false,
                "profiler-port":    6060,
                "profiler-host":    "localhost",
                "heatmap":          false,
                "heatmap-mode":     "bar",
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.expected["logs-level"], tt.flags.LogsLevel)
            assert.Equal(t, tt.expected["no-color"], tt.flags.NoColor)
            assert.Equal(t, tt.expected["profiler-port"], tt.flags.ProfilerPort)
            // ... etc
        })
    }
}
```

### Integration Tests

```go
// tests/default_values_test.go

func TestDefaultValuesInCommands(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        expected GlobalFlags
    }{
        {
            name: "no flags provided - use all defaults",
            args: []string{"terraform", "plan", "vpc", "-s", "prod"},
            expected: GlobalFlags{
                LogsLevel:    "Info",      // ✅ Default
                LogsFile:     "/dev/stderr", // ✅ Default
                NoColor:      false,       // ✅ Default
                ProfilerPort: 6060,        // ✅ Default
                HeatmapMode:  "bar",       // ✅ Default
            },
        },
        {
            name: "override one default",
            args: []string{"terraform", "plan", "vpc", "-s", "prod", "--logs-level", "Trace"},
            expected: GlobalFlags{
                LogsLevel:    "Trace",     // ✅ Overridden
                LogsFile:     "/dev/stderr", // ✅ Default
                NoColor:      false,       // ✅ Default
                ProfilerPort: 6060,        // ✅ Default
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            interpreter, err := parser.Parse(context.Background(), tt.args)
            require.NoError(t, err)

            assert.Equal(t, tt.expected.LogsLevel, interpreter.LogsLevel)
            assert.Equal(t, tt.expected.LogsFile, interpreter.LogsFile)
            assert.Equal(t, tt.expected.NoColor, interpreter.NoColor)
            // ... etc
        })
    }
}
```

## Command-Specific Defaults

Commands can also have their own defaults:

```go
// pkg/flags/terraform_registry.go

func TerraformFlagsRegistry() *FlagRegistry {
    registry := CommonFlags()  // Includes stack, identity, dry-run

    // Terraform-specific flags with defaults
    registry.Register(&BoolFlag{
        Name:        "skip-init",
        Default:     false,  // Default: run terraform init
        Description: "Skip terraform init",
        EnvVars:     []string{"ATMOS_TERRAFORM_SKIP_INIT"},
    })

    registry.Register(&BoolFlag{
        Name:        "upload-status",
        Shorthand:   "",
        Default:     true,  // Default: upload status to Atmos Pro
        Description: "Upload plan status to Atmos Pro",
        NoOptDefVal: "true",  // --upload-status alone = true
        EnvVars:     []string{"ATMOS_TERRAFORM_UPLOAD_STATUS"},
    })

    registry.Register(&StringFlag{
        Name:        "from-plan",
        Default:     "",  // Default: no plan file
        Description: "Apply from previously generated plan",
        EnvVars:     []string{"ATMOS_TERRAFORM_FROM_PLAN"},
    })

    return registry
}
```

## Benefits

### 1. DRY (Don't Repeat Yourself)
- ✅ Defaults defined once in flag registration
- ✅ Viper automatically applies defaults
- ✅ No need to set defaults in multiple places

### 2. Consistency
- ✅ Same default everywhere (CLI help, config, code)
- ✅ Precedence always respected
- ✅ Zero values handled correctly

### 3. Documentation
- ✅ Defaults visible in `--help` output
- ✅ Defaults documented in flag description
- ✅ Defaults testable

### 4. Type Safety
- ✅ Defaults match field types
- ✅ Compile-time checks
- ✅ No string conversion errors

### 5. Testability
- ✅ Easy to mock with specific defaults
- ✅ Easy to test precedence
- ✅ Easy to verify default behavior

## Implementation Checklist

- [ ] Create `GlobalFlagsRegistry()` with all global flags and defaults
- [ ] Update `parseGlobalFlags()` to use Viper's default handling
- [ ] Document defaults in flag descriptions
- [ ] Add unit tests for default values
- [ ] Add integration tests for precedence with defaults
- [ ] Verify all 13+ global flags have appropriate defaults
- [ ] Document default value patterns in CLAUDE.md

## Conclusion

The default value system is **already built into the flag type system**. We just need to:

1. **Use it consistently** in flag registrations
2. **Trust Viper** to apply precedence correctly
3. **Document defaults** in flag descriptions
4. **Test defaults** thoroughly

**Key insight**: Viper automatically uses Cobra flag defaults, so we don't need to call `viper.SetDefault()` separately. Just register flags with proper defaults, bind to Viper, and the precedence system "just works"!

**Timeline**: 1-2 days to document, verify, and test all defaults

**Risk**: LOW - System already in place, just need to use it properly
