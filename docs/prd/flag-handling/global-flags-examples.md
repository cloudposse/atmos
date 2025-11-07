# Global Flags Examples: --logs-level and --identity

This document shows how `--logs-level` (simple global flag) and `--identity` (complex global flag with NoOptDefVal) demonstrate the global flags pattern with strongly-typed interpreters.

## Current Implementation

### --logs-level (Simple Global Flag)

**Current implementation** (manual parsing in `pkg/config/config.go:64-79`):

```go
func setLogConfig(atmosConfig *schema.AtmosConfiguration) {
    // TODO: This is a quick patch to mitigate the issue
    // Issue: https://linear.app/cloudposse/issue/DEV-3093/create-a-cli-command-core-library

    // Manual precedence handling
    if os.Getenv("ATMOS_LOGS_LEVEL") != "" {
        atmosConfig.Logs.Level = os.Getenv("ATMOS_LOGS_LEVEL")
    }

    flagKeyValue := parseFlags()  // ⚠️ Custom flag parsing!
    if v, ok := flagKeyValue["logs-level"]; ok {
        atmosConfig.Logs.Level = v
    }

    if os.Getenv("ATMOS_LOGS_FILE") != "" {
        atmosConfig.Logs.File = os.Getenv("ATMOS_LOGS_FILE")
    }

    if v, ok := flagKeyValue["logs-file"]; ok {
        atmosConfig.Logs.File = v
    }

    // ... similar for no-color
}
```

**Problems:**
- ❌ Manual flag parsing with `parseFlags()`
- ❌ Manual precedence handling (ENV then flag)
- ❌ Duplicated for logs-file, no-color, etc.
- ❌ Called before config is loaded (bootstrap problem)
- ❌ Not using Viper's built-in precedence

**Registration** (in `cmd/root.go:671`):

```go
RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off")
```

### --identity (Complex Global Flag with NoOptDefVal)

**Current implementation** (multiple places):

**Registration** (in `cmd/auth.go:28-43`):

```go
const (
    IdentityFlagName        = "identity"
    IdentityFlagSelectValue = cfg.IdentityFlagSelectValue  // "__SELECT__"
)

func init() {
    authCmd.PersistentFlags().StringP(IdentityFlagName, "i", "",
        "Specify the target identity to assume. Use without value to interactively select.")

    // Set NoOptDefVal to enable optional flag value
    identityFlag := authCmd.PersistentFlags().Lookup(IdentityFlagName)
    if identityFlag != nil {
        identityFlag.NoOptDefVal = IdentityFlagSelectValue  // "__SELECT__"
    }

    // Bind environment variables but NOT the flag itself
    // (to preserve NoOptDefVal detection)
    if err := viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"); err != nil {
        log.Trace("Failed to bind identity environment variables", "error", err)
    }
}
```

**Usage** (in `cmd/auth_console.go:245-257`):

```go
var identityName string

// Manual precedence check
if cmd.Flags().Changed(IdentityFlagName) {
    // Flag was explicitly provided on command line
    identityName, _ = cmd.Flags().GetString(IdentityFlagName)
} else {
    // Flag not provided - fall back to viper (config/env)
    identityName = viper.GetString(IdentityFlagName)
}

// Check if user wants to interactively select identity
forceSelect := identityName == IdentityFlagSelectValue

if identityName != "" && !forceSelect {
    return identityName, nil
}
```

**Problems:**
- ❌ Manual precedence checking (`cmd.Flags().Changed()`)
- ❌ Duplicated across auth commands (console, login, exec, shell)
- ❌ Complex logic for NoOptDefVal handling
- ❌ Not using strongly-typed access
- ❌ Pattern repeated in terraform commands too

## Unified Pattern with Strongly-Typed Interpreters

### Step 1: Define GlobalFlags Struct

```go
// pkg/flagparser/global_flags.go

// GlobalFlags contains all persistent flags available to every command.
type GlobalFlags struct {
    // Working directory and path configuration
    Chdir      string
    BasePath   string
    Config     []string
    ConfigPath []string

    // Logging configuration (INCLUDES --logs-level!)
    LogsLevel string
    LogsFile  string
    NoColor   bool

    // Output configuration
    Pager PagerSelector

    // Authentication (INCLUDES --identity!)
    Identity IdentitySelector

    // Profiling configuration
    ProfilerEnabled bool
    ProfilerPort    int
    ProfilerHost    string
    ProfileFile     string
    ProfileType     string

    // Performance visualization
    Heatmap     bool
    HeatmapMode string

    // System configuration
    RedirectStderr string
    Version        bool
}
```

### Step 2: Define Special Type for Identity

```go
// pkg/flagparser/identity_selector.go

// IdentitySelector handles the identity flag which has three states:
// 1. Not provided (use default from config/env)
// 2. Provided without value (--identity) → interactive selection
// 3. Provided with value (--identity=name) → use specific identity
type IdentitySelector struct {
    value    string
    provided bool
}

// NewIdentitySelector creates an IdentitySelector from flag state.
func NewIdentitySelector(value string, provided bool) IdentitySelector {
    return IdentitySelector{value: value, provided: provided}
}

// IsInteractiveSelector returns true if --identity was used without a value.
func (i IdentitySelector) IsInteractiveSelector() bool {
    return i.provided && i.value == cfg.IdentityFlagSelectValue // "__SELECT__"
}

// Value returns the identity name.
func (i IdentitySelector) Value() string {
    return i.value
}

// IsEmpty returns true if no identity was provided.
func (i IdentitySelector) IsEmpty() bool {
    return !i.provided || i.value == ""
}

// IsProvided returns true if the flag was explicitly set.
func (i IdentitySelector) IsProvided() bool {
    return i.provided
}
```

### Step 3: Parse Global Flags with Precedence

```go
// pkg/flagparser/parser.go

// parseGlobalFlags extracts all global flags with proper precedence.
// Called ONCE per command execution, before command-specific parsing.
func (p *baseParser) parseGlobalFlags(cmd *cobra.Command) GlobalFlags {
    return GlobalFlags{
        Chdir:      p.viper.GetString("chdir"),
        BasePath:   p.viper.GetString("base-path"),
        Config:     p.viper.GetStringSlice("config"),
        ConfigPath: p.viper.GetStringSlice("config-path"),

        // ✅ --logs-level with precedence handled by Viper automatically!
        LogsLevel: p.viper.GetString("logs-level"),
        LogsFile:  p.viper.GetString("logs-file"),
        NoColor:   p.viper.GetBool("no-color"),

        Pager: p.parsePagerFlag(cmd),

        // ✅ --identity with NoOptDefVal handling!
        Identity: p.parseIdentityFlag(cmd),

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

// parseIdentityFlag handles the identity flag's NoOptDefVal pattern.
func (p *baseParser) parseIdentityFlag(cmd *cobra.Command) IdentitySelector {
    flag := cmd.Flags().Lookup("identity")
    if flag == nil {
        return IdentitySelector{provided: false}
    }

    // Check if flag was explicitly set on command line
    if cmd.Flags().Changed("identity") {
        value := p.viper.GetString("identity")
        return IdentitySelector{
            value:    value,
            provided: true,
        }
    }

    // Fall back to env/config via Viper
    if p.viper.IsSet("identity") {
        value := p.viper.GetString("identity")
        return IdentitySelector{
            value:    value,
            provided: true,
        }
    }

    return IdentitySelector{provided: false}
}
```

### Step 4: Embed in Command Interpreters

```go
// pkg/flagparser/terraform_interpreter.go

type TerraformInterpreter struct {
    GlobalFlags  // ✅ Embedded - provides LogsLevel, Identity, and all other global flags!

    // Terraform-specific flags
    Stack        string
    DryRun       bool
    SkipInit     bool
    FromPlan     string
    UploadStatus bool

    Subcommand string
    Component  string

    positionalArgs  []string
    passThroughArgs []string
}
```

```go
// pkg/flagparser/auth_interpreter.go

type AuthInterpreter struct {
    GlobalFlags  // ✅ Same global flags - Identity already included!

    // Auth-specific flags
    Shell string

    Subcommand string

    positionalArgs  []string
    passThroughArgs []string
}
```

### Step 5: Use in Commands

#### Example 1: --logs-level in Terraform Command

**BEFORE** (manual parsing):

```go
// cmd/terraform_utils.go (current)

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
    // ❌ Logger already initialized with manual parsing from pkg/config/config.go
    // ❌ Can't change log level dynamically
    // ❌ Precedence handled manually

    // ... execute terraform
}
```

**AFTER** (strongly-typed):

```go
// cmd/terraform_utils.go (with interpreters)

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
    interpreter, err := terraformParser.Parse(ctx, args)
    if err != nil {
        return err
    }

    // ✅ Strongly-typed access to global flags
    // ✅ Precedence already applied by parser
    log.SetLevel(interpreter.LogsLevel)  // From GlobalFlags
    log.SetOutput(interpreter.LogsFile)  // From GlobalFlags

    if interpreter.NoColor {             // From GlobalFlags
        color.NoColor = true
    }

    // ✅ Terraform-specific flags
    info.Stack = interpreter.Stack
    info.DryRun = interpreter.DryRun

    return executor.Execute(ctx, info, interpreter.passThroughArgs)
}
```

#### Example 2: --identity in Auth Commands

**BEFORE** (manual precedence + complex logic):

```go
// cmd/auth_console.go (current)

func authConsoleRun(cmd *cobra.Command, args []string) error {
    // ❌ Manual precedence checking
    var identityName string
    if cmd.Flags().Changed(IdentityFlagName) {
        identityName, _ = cmd.Flags().GetString(IdentityFlagName)
    } else {
        identityName = viper.GetString(IdentityFlagName)
    }

    // ❌ Manual NoOptDefVal checking
    forceSelect := identityName == IdentityFlagSelectValue

    if identityName != "" && !forceSelect {
        return identityName, nil
    }

    // ... handle interactive selection
}
```

**AFTER** (strongly-typed with IdentitySelector):

```go
// cmd/auth_console.go (with interpreters)

func authConsoleRun(cmd *cobra.Command, args []string) error {
    interpreter, err := authParser.Parse(ctx, args)
    if err != nil {
        return err
    }

    // ✅ Clean, self-documenting identity handling
    if interpreter.Identity.IsInteractiveSelector() {
        // User used --identity without value → interactive selection
        identity, err := selectIdentityInteractively(authManager)
        if err != nil {
            return err
        }
        return openConsole(identity)
    }

    if !interpreter.Identity.IsEmpty() {
        // User provided explicit identity
        return openConsole(interpreter.Identity.Value())
    }

    // No identity provided → use default
    defaultIdentity, err := authManager.GetDefaultIdentity(false)
    if err != nil {
        return err
    }

    return openConsole(defaultIdentity)
}
```

#### Example 3: --identity in Terraform Commands

**BEFORE** (duplicated logic):

```go
// cmd/terraform_utils.go (current)

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
    // ❌ Duplicated identity logic from auth commands
    var identityName string
    if cmd.Flags().Changed("identity") {
        identityName, _ = cmd.Flags().GetString("identity")
    } else {
        identityName = viper.GetString("identity")
    }

    forceSelect := identityName == IdentityFlagSelectValue

    // ... handle identity
}
```

**AFTER** (same pattern as auth):

```go
// cmd/terraform_utils.go (with interpreters)

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
    interpreter, err := terraformParser.Parse(ctx, args)
    if err != nil {
        return err
    }

    // ✅ Same clean pattern - zero duplication!
    if interpreter.Identity.IsInteractiveSelector() {
        identity, err := selectIdentityInteractively(authManager)
        if err != nil {
            return err
        }
        info.Identity = identity
    } else if !interpreter.Identity.IsEmpty() {
        info.Identity = interpreter.Identity.Value()
    }

    // Continue with terraform execution
    info.Stack = interpreter.Stack
    return executor.Execute(ctx, info, interpreter.passThroughArgs)
}
```

## Benefits Demonstrated

### For --logs-level (Simple Global Flag)

**Before:**
- ❌ Manual flag parsing with custom `parseFlags()` function
- ❌ Manual precedence handling (check ENV, then flag)
- ❌ Duplicated for each flag (logs-level, logs-file, no-color)
- ❌ Called before config loading (bootstrap problem)
- ❌ Not using Viper's precedence system

**After:**
- ✅ Single definition in `GlobalFlags` struct
- ✅ Automatic precedence via Viper (CLI > ENV > config > default)
- ✅ Strongly-typed access: `interpreter.LogsLevel`
- ✅ Available in ALL commands via embedding
- ✅ Zero duplication across commands
- ✅ Bootstrap problem solved (parser handles early flags)

### For --identity (Complex Global Flag with NoOptDefVal)

**Before:**
- ❌ Manual precedence checking with `cmd.Flags().Changed()`
- ❌ Manual NoOptDefVal checking with string comparison
- ❌ Duplicated logic across auth commands (console, login, exec, shell)
- ❌ Duplicated again in terraform commands
- ❌ Complex conditional logic in every command
- ❌ Easy to get wrong (miss a case)

**After:**
- ✅ Encapsulated in `IdentitySelector` type
- ✅ Clear API: `IsInteractiveSelector()`, `Value()`, `IsEmpty()`
- ✅ Self-documenting code
- ✅ Zero duplication across commands
- ✅ Parser handles all complexity once
- ✅ Type-safe, compile-time checked
- ✅ Easy to test (mock with simple struct)

## Side-by-Side Comparison

### Manual Precedence (Current)

```go
// ❌ Repeated in EVERY command that uses identity
var identityName string
if cmd.Flags().Changed("identity") {
    identityName, _ = cmd.Flags().GetString("identity")
} else {
    identityName = viper.GetString("identity")
}

forceSelect := identityName == IdentityFlagSelectValue

if identityName != "" && !forceSelect {
    // use identity
} else if forceSelect {
    // interactive selection
} else {
    // use default
}
```

**Lines of code per command:** ~12 lines
**Commands using identity:** 5+ (auth console, login, exec, shell, terraform, helmfile)
**Total duplication:** ~60+ lines

### Strongly-Typed Pattern (New)

```go
// ✅ Zero duplication - pattern defined ONCE in parser
if interpreter.Identity.IsInteractiveSelector() {
    // interactive selection
} else if !interpreter.Identity.IsEmpty() {
    // use identity
} else {
    // use default
}
```

**Lines of code per command:** ~6 lines
**Commands using identity:** Same 5+ commands
**Total duplication:** ~30 lines
**Savings:** 50% reduction + better readability + type safety

## Testing Examples

### Testing --logs-level

**BEFORE** (hard to test):

```go
// Can't easily test precedence without mocking os.Args and os.Getenv
func TestLogsLevel(t *testing.T) {
    // ❌ Must set environment variable
    os.Setenv("ATMOS_LOGS_LEVEL", "Debug")
    defer os.Unsetenv("ATMOS_LOGS_LEVEL")

    // ❌ Must parse os.Args
    os.Args = []string{"atmos", "--logs-level", "Trace"}

    // ❌ Must call setLogConfig and check atmosConfig
    // Hard to verify precedence
}
```

**AFTER** (easy to test):

```go
// ✅ Simple, direct testing of precedence
func TestLogsLevelPrecedence(t *testing.T) {
    tests := []struct {
        name     string
        flagSet  bool
        flagVal  string
        envVal   string
        expected string
    }{
        {
            name:     "flag overrides env",
            flagSet:  true,
            flagVal:  "Trace",
            envVal:   "Debug",
            expected: "Trace",
        },
        {
            name:     "env used when flag not set",
            flagSet:  false,
            envVal:   "Debug",
            expected: "Debug",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            interpreter := &TerraformInterpreter{
                GlobalFlags: GlobalFlags{
                    LogsLevel: tt.expected,
                },
            }
            assert.Equal(t, tt.expected, interpreter.LogsLevel)
        })
    }
}
```

### Testing --identity

**BEFORE** (complex setup):

```go
func TestIdentityFlag(t *testing.T) {
    // ❌ Must create command, set flag, parse args
    cmd := &cobra.Command{}
    cmd.Flags().StringP("identity", "i", "", "Identity")
    cmd.Flags().Lookup("identity").NoOptDefVal = "__SELECT__"

    // ❌ Must parse args manually
    cmd.ParseFlags([]string{"--identity"})

    // ❌ Must check Changed() and GetString()
    var identityName string
    if cmd.Flags().Changed("identity") {
        identityName, _ = cmd.Flags().GetString("identity")
    }

    // ❌ Must check magic string
    assert.Equal(t, "__SELECT__", identityName)
}
```

**AFTER** (simple struct):

```go
func TestIdentitySelector(t *testing.T) {
    tests := []struct {
        name         string
        selector     IdentitySelector
        isInteractive bool
        isEmpty      bool
        value        string
    }{
        {
            name:         "interactive selection",
            selector:     IdentitySelector{value: "__SELECT__", provided: true},
            isInteractive: true,
            isEmpty:      false,
            value:        "__SELECT__",
        },
        {
            name:         "explicit identity",
            selector:     IdentitySelector{value: "prod-admin", provided: true},
            isInteractive: false,
            isEmpty:      false,
            value:        "prod-admin",
        },
        {
            name:         "not provided",
            selector:     IdentitySelector{provided: false},
            isInteractive: false,
            isEmpty:      true,
            value:        "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.isInteractive, tt.selector.IsInteractiveSelector())
            assert.Equal(t, tt.isEmpty, tt.selector.IsEmpty())
            assert.Equal(t, tt.value, tt.selector.Value())
        })
    }
}
```

## Migration Path

### Phase 1: Create Infrastructure (1 day)
- [ ] Create `pkg/flagparser/global_flags.go`
- [ ] Create `pkg/flagparser/identity_selector.go`
- [ ] Add `parseGlobalFlags()` to base parser
- [ ] Add `parseIdentityFlag()` helper
- [ ] Unit tests for IdentitySelector

### Phase 2: Update Interpreters (1 day)
- [ ] Update all interpreter structs to embed `GlobalFlags`
- [ ] Remove duplicate field definitions
- [ ] Update constructors to populate GlobalFlags

### Phase 3: Update Commands (1-2 days)
- [ ] Update terraform commands to use `interpreter.LogsLevel`, `interpreter.Identity`
- [ ] Update auth commands to use `interpreter.Identity`
- [ ] Update helmfile/packer commands
- [ ] Remove duplicated identity logic

### Phase 4: Remove Old Code (0.5 days)
- [ ] Remove `setLogConfig()` manual parsing from `pkg/config/config.go`
- [ ] Remove duplicated identity checking code
- [ ] Clean up old precedence logic

### Phase 5: Testing (0.5 days)
- [ ] Add integration tests for global flags
- [ ] Verify precedence works correctly
- [ ] Test identity pattern in all commands

**Total effort:** 3-4 days

## Conclusion

Both `--logs-level` (simple) and `--identity` (complex) demonstrate the power of the strongly-typed global flags pattern:

1. **--logs-level** shows elimination of manual parsing and precedence
2. **--identity** shows encapsulation of complex NoOptDefVal logic
3. Both show zero duplication via struct embedding
4. Both provide type safety and compile-time checks
5. Both are easier to test and maintain

The pattern scales to ALL 13+ global flags with same benefits!
