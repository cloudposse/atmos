# Research: Unified Flag Parsing for Atmos

## Executive Summary

This document summarizes research into creating a unified flag parsing system for Atmos that:
- Works consistently across all commands (Terraform, Auth, Helmfile, Packer, etc.)
- Enforces precedence order: CLI flags → ENV vars → config files → defaults
- Supports double dash separator (`--`) for pass-through commands
- Enables interface-driven testing with 80-90% coverage
- Preserves and augments Cobra/Viper functionality

## Current State Analysis

### Inconsistent Implementations

Atmos currently has **4 different flag parsing patterns**:

1. **Pass-Through Commands** (Terraform, Helmfile, Packer)
   - Use `DisableFlagParsing = true`
   - Custom double dash handling in `extractTrailingArgs()`
   - Manual flag extraction in `processArgsAndFlags()`
   - Files: `cmd/terraform.go`, `cmd/helmfile.go`, `cmd/packer.go`

2. **Auth Command**
   - Custom identity flag handling
   - Manual Viper binding without `BindPFlag`
   - Precedence: flag → Viper → fallback
   - File: `cmd/auth.go`, `cmd/cmd_utils.go:344-382`

3. **Custom Commands**
   - Dynamic flag registration from config
   - Separate trailing args extraction
   - Identity integration
   - File: `cmd/cmd_utils.go:109-133`

4. **Standard Commands**
   - Mix of flag registration patterns
   - Inconsistent Viper integration
   - Various precedence implementations

### Problems

1. **Duplicated Logic**: Flag parsing code repeated in:
   - `cmd/cmd_utils.go` (~400 lines)
   - `internal/exec/cli_utils.go` (~200 lines)
   - `pkg/config/config.go` (~100 lines)

2. **Inconsistent Precedence**: Must manually implement in each command:
   - Some use Viper precedence
   - Some manually check flags → env → config
   - Some only check flags

3. **Testing Difficulty**:
   - Tight coupling between parsing and business logic
   - Hard to mock due to direct `os.Args` access
   - No isolated tests for flag parsing

4. **Maintenance Burden**:
   - New commands must reimplement flag handling
   - Bug fixes must be applied in multiple places
   - Hard to ensure consistency

## Research Findings

### How Major Projects Handle This

#### Docker CLI: TopLevelCommand Pattern

**Pattern**: Encapsulates global flag handling in a dedicated struct.

**Key Insight**: Use `FlagSet.SetInterspersed(false)` to stop parsing at subcommands, enabling global flag extraction before subcommand execution.

**Code Pattern**:
```go
type TopLevelCommand struct {
    cmd     *cobra.Command
    flags   *pflag.FlagSet
    // ... other fields
}

func (tcmd *TopLevelCommand) HandleGlobalFlags(args []string) error {
    // Combine persistent and command flags
    tcmd.flags.AddFlagSet(tcmd.cmd.Flags())
    tcmd.flags.AddFlagSet(tcmd.cmd.PersistentFlags())

    // Parse up to first subcommand
    tcmd.flags.SetInterspersed(false)
    return tcmd.flags.Parse(args)
}
```

**Relevance**: Could use similar pattern for Atmos global flags.

#### Kubectl: PrintFlags Composition Pattern

**Pattern**: Composes flag structs that can be shared across commands.

**Example**:
```go
type PrintFlags struct {
    OutputFormat *string
    // ... other fields
}

func (f *PrintFlags) AddFlags(cmd *cobra.Command) {
    cmd.Flags().StringVarP(f.OutputFormat, "output", "o", "", "output format")
}
```

**Relevance**: Could create reusable flag groups for common Atmos flags.

#### Helm: Ignore Flags for Plugins

**Pattern**: Use `ignoreFlags` switch to skip flag parsing for plugin commands.

**Relevance**: Similar to our pass-through commands needing `DisableFlagParsing`.

### Cobra/Viper Integration: The "Sting of the Viper"

**Source**: [carolynvanslyck.com/blog/2020/08/sting-of-the-viper](https://carolynvanslyck.com/blog/2020/08/sting-of-the-viper/)

**Key Insights**:

1. **Cobra and Viper were never meant to work together** - integration requires explicit orchestration

2. **Use PersistentPreRunE for integration**:
   ```go
   rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
       // Load config file
       if err := viper.ReadInConfig(); err != nil {
           // handle error
       }

       // Bind env vars
       viper.SetEnvPrefix("MYAPP")
       viper.AutomaticEnv()

       // Flags already bound via BindPFlag in init()
       return nil
   }
   ```

3. **BindPFlag is one-way** (flag → Viper):
   ```go
   // In init()
   viper.BindPFlag("port", cmd.Flags().Lookup("port"))
   ```

4. **Always read from Viper**, never from flag variables:
   ```go
   // ✅ Correct - respects precedence
   port := viper.GetInt("port")

   // ❌ Wrong - bypasses precedence
   port := portFlag
   ```

5. **Viper's automatic precedence**:
   - Explicit `viper.Set()` (highest)
   - CLI flags (via `BindPFlag`)
   - Environment variables (via `BindEnv` or `AutomaticEnv`)
   - Config files (via `ReadInConfig`)
   - Defaults (via `SetDefault`) (lowest)

**Recommendation**: Adopt this pattern consistently across all Atmos commands.

### Double Dash Separator: ArgsLenAtDash

**Cobra Feature**: `cmd.ArgsLenAtDash()` returns index of `--` or `-1` if not present.

**Pattern**:
```go
func Run(cmd *cobra.Command, args []string) error {
    dashIndex := cmd.ArgsLenAtDash()

    var cmdArgs, toolArgs []string
    if dashIndex > -1 {
        cmdArgs = args[:dashIndex]
        toolArgs = args[dashIndex:]
    } else {
        cmdArgs = args
    }

    // Process cmdArgs, pass toolArgs to underlying tool
}
```

**Alternative**: `DisableFlagParsing = true` for full pass-through, but then must manually parse Atmos flags.

**Recommendation**: Use `ArgsLenAtDash()` instead of custom `extractTrailingArgs()`.

### Testing: Separation of Concerns

**Source**: [eli.thegreenplace.net/2020/testing-flag-parsing-in-go-programs](https://eli.thegreenplace.net/2020/testing-flag-parsing-in-go-programs/)

**Pattern**: Extract flag values into a Config struct, test parsing separately from business logic.

```go
// Testable - parsing separated from logic
func parseFlags(args []string) (*Config, error) {
    fs := flag.NewFlagSet("myapp", flag.ContinueOnError)
    cfg := &Config{}
    fs.StringVar(&cfg.Output, "output", "stdout", "output destination")
    // ... other flags
    return cfg, fs.Parse(args)
}

// Test just parsing
func TestParseFlags(t *testing.T) {
    cfg, err := parseFlags([]string{"-output", "file.txt"})
    assert.NoError(t, err)
    assert.Equal(t, "file.txt", cfg.Output)
}
```

**Dependency Injection Pattern**:
```go
// Constructor accepts dependencies
func NewCommand(
    loader ConfigLoader,
    executor Executor,
) *cobra.Command {
    // ... command setup
}

// Test with mocks
func TestCommand(t *testing.T) {
    mockLoader := &MockConfigLoader{}
    mockExecutor := &MockExecutor{}

    cmd := NewCommand(mockLoader, mockExecutor)
    cmd.SetArgs([]string{"--flag", "value"})

    err := cmd.Execute()
    assert.NoError(t, err)
}
```

**Recommendation**: Implement both patterns - separation and DI.

### Functional Options Pattern

**Source**: [codingexplorations.com/blog/functional-options-pattern-go](https://www.codingexplorations.com/blog/functional-options-pattern-go)

**Benefits**:
- Avoids parameter drilling (many function parameters)
- Provides sensible defaults
- Extensible without breaking changes
- Self-documenting

**Pattern**:
```go
type Option func(*Config)

func WithTimeout(d time.Duration) Option {
    return func(c *Config) { c.Timeout = d }
}

func NewClient(opts ...Option) *Client {
    cfg := &Config{Timeout: 30 * time.Second} // default
    for _, opt := range opts {
        opt(cfg)
    }
    return &Client{config: cfg}
}

// Usage
client := NewClient(
    WithTimeout(60 * time.Second),
    WithRetries(3),
)
```

**Recommendation**: Use for ConfigLoader initialization.

### Third-Party Libraries

#### go-extras/cobraflags

**Repository**: [github.com/go-extras/cobraflags](https://github.com/go-extras/cobraflags)

**Features**:
- Automatic Cobra/Viper integration
- Typed flag wrappers (`IntFlag`, `StringFlag`, `BoolFlag`)
- Environment variable binding with prefix
- Built-in validation

**Example**:
```go
flag := &cobraflags.StringFlag{
    Name:       "example",
    ViperKey:   "nested.config.key",
    EnvVarName: "MYAPP_EXAMPLE",
    ValidateFunc: func(val string) error {
        if val == "" {
            return errors.New("cannot be empty")
        }
        return nil
    },
}
flag.Register(cmd)
```

**Evaluation**:
- **Pros**: Reduces boilerplate, built-in validation
- **Cons**: External dependency, less control over implementation
- **Recommendation**: Adopt similar patterns, but implement internally to maintain control

## Recommended Approach

### Architecture

#### 1. Interface-Driven Design

Define interfaces for all major components:

```go
// FlagParser handles flag registration and parsing
type FlagParser interface {
    RegisterFlags(cmd *cobra.Command)
    BindToViper(v *viper.Viper) error
    Parse(ctx context.Context, args []string) (*ParsedConfig, error)
}

// ConfigLoader loads configuration with proper precedence
type ConfigLoader interface {
    Load(ctx context.Context, opts ...LoadOption) (*Config, error)
    Reload(ctx context.Context) error
}

// PassThroughHandler separates Atmos flags from tool flags
type PassThroughHandler interface {
    SplitArgs(args []string) (atmosArgs, toolArgs []string, err error)
}
```

#### 2. Middleware Pattern

Use Cobra hooks for configuration pipeline:

```go
type CobraMiddleware func(cmd *cobra.Command, args []string) error

func ComposeMiddleware(middlewares ...CobraMiddleware) CobraMiddleware {
    return func(cmd *cobra.Command, args []string) error {
        for _, mw := range middlewares {
            if err := mw(cmd, args); err != nil {
                return err
            }
        }
        return nil
    }
}

// Usage
cmd.PersistentPreRunE = ComposeMiddleware(
    ConfigMiddleware(loader),
    AuthMiddleware(authMgr),
    ValidationMiddleware(),
)
```

#### 3. Viper as Single Source of Truth

**All configuration reads go through Viper**:

```go
// ✅ Correct
value := viper.GetString("flag-name")

// ❌ Wrong
value := cmd.Flags().GetString("flag-name")
```

**Precedence enforced automatically by Viper**:
1. CLI flags (via `BindPFlag`)
2. Environment variables (via `BindEnv`)
3. Config files (via `ReadInConfig`)
4. Defaults (via `SetDefault`)

#### 4. Dependency Injection

Commands receive dependencies via constructors:

```go
func NewTerraformCmd(
    loader ConfigLoader,
    parser PassThroughFlagParser,
    executor TerraformExecutor,
) *cobra.Command {
    // ... setup
}

// Easy to test with mocks
func TestTerraformCmd(t *testing.T) {
    mockLoader := &MockConfigLoader{}
    mockParser := &MockFlagParser{}
    mockExecutor := &MockTerraformExecutor{}

    cmd := NewTerraformCmd(mockLoader, mockParser, mockExecutor)
    // ... test
}
```

### Implementation Strategy

#### Phase 1: Core Infrastructure

**Package**: `pkg/flagparser/`

1. **FlagParser Interface**:
   - `StandardFlagParser` for typical commands
   - `PassThroughFlagParser` for Terraform/Helmfile/Packer
   - `GlobalFlagParser` for RootCmd

2. **PassThroughHandler**:
   - Replace `extractTrailingArgs()` with interface-based implementation
   - Use `cmd.ArgsLenAtDash()` instead of manual parsing
   - Support both `--` separator and Atmos flag extraction

3. **FlagRegistry**:
   - Central registry of common flags
   - Reusable across commands
   - Type-safe flag definitions

**Package**: `pkg/config/`

4. **ConfigLoader Interface**:
   - `ViperConfigLoader` implementation
   - Encapsulates precedence logic
   - Functional options for configuration

**Package**: `cmd/internal/middleware/`

5. **Middleware Components**:
   - `ConfigMiddleware` - loads config
   - `AuthMiddleware` - handles authentication
   - `ValidationMiddleware` - validates flags
   - `ComposeMiddleware` - chains middleware

#### Phase 2: Command Migration

Migrate commands in order:

1. **Pass-through commands** (Terraform, Helmfile, Packer)
   - Most complex
   - Establish pattern for others

2. **Standard commands** (Validate, Describe, Workflow)
   - Simpler pattern
   - Most common type

3. **Custom commands**
   - Dynamic flag registration
   - Special handling

4. **Root command**
   - Global flags
   - Final integration

#### Phase 3: Cleanup

- Remove duplicated code
- Update documentation
- Comprehensive testing

### Testing Strategy

#### Unit Tests (Target: 90% coverage)

```go
// Test flag parsing in isolation
func TestStandardFlagParser_Parse(t *testing.T) {
    parser := NewStandardFlagParser()
    // ... register flags

    cfg, err := parser.Parse(context.Background(), []string{
        "--flag1", "value1",
        "--flag2", "value2",
    })

    assert.NoError(t, err)
    assert.Equal(t, "value1", cfg.Flag1)
    assert.Equal(t, "value2", cfg.Flag2)
}

// Test precedence order
func TestConfigLoader_Precedence(t *testing.T) {
    // Setup: config file sets value to "config"
    // Setup: env var sets value to "env"
    // Setup: flag sets value to "flag"

    loader := NewViperConfigLoader()
    cfg, err := loader.Load(context.Background())

    assert.NoError(t, err)
    assert.Equal(t, "flag", cfg.Value) // Flag wins
}

// Test double dash separator
func TestPassThroughHandler_SplitArgs(t *testing.T) {
    handler := NewPassThroughHandler()

    atmosArgs, toolArgs, err := handler.SplitArgs([]string{
        "--stack", "prod",
        "--",
        "-auto-approve",
        "-var=foo=bar",
    })

    assert.NoError(t, err)
    assert.Equal(t, []string{"--stack", "prod"}, atmosArgs)
    assert.Equal(t, []string{"-auto-approve", "-var=foo=bar"}, toolArgs)
}
```

#### Integration Tests

```go
// Test full command execution
func TestTerraformCommand_E2E(t *testing.T) {
    // Setup test environment
    tempDir := t.TempDir()
    os.Setenv("ATMOS_BASE_PATH", tempDir)

    // Create test command
    cmd := NewTerraformCmd(
        realConfigLoader,
        realFlagParser,
        mockExecutor,
    )

    cmd.SetArgs([]string{
        "--stack", "test-stack",
        "plan",
        "--",
        "-out=tfplan",
    })

    // Execute
    err := cmd.Execute()
    assert.NoError(t, err)

    // Verify executor received correct args
    assert.Equal(t, "test-stack", mockExecutor.CalledWithStack)
    assert.Equal(t, []string{"-out=tfplan"}, mockExecutor.CalledWithToolArgs)
}
```

#### Backward Compatibility Tests

```go
// Test existing flags still work
func TestBackwardCompatibility_ExistingFlags(t *testing.T) {
    tests := []struct {
        name string
        args []string
        want Config
    }{
        {
            name: "legacy --logs-level flag",
            args: []string{"--logs-level", "Debug"},
            want: Config{LogsLevel: "Debug"},
        },
        {
            name: "legacy ATMOS_LOGS_LEVEL env var",
            args: []string{},
            env:  map[string]string{"ATMOS_LOGS_LEVEL": "Debug"},
            want: Config{LogsLevel: "Debug"},
        },
        // ... more tests
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ... test
        })
    }
}
```

## Benefits

### For Development

1. **Single implementation**: Flag parsing logic in one place
2. **Easy to extend**: Add new flags without duplicating code
3. **Consistent behavior**: All commands work the same way
4. **Better testing**: Mockable interfaces, isolated tests
5. **Clear architecture**: Interface-driven, dependency injection

### For Users

1. **Consistent UX**: All commands behave predictably
2. **Reliable precedence**: Flags always override env vars and config
3. **Better errors**: Clear messages when flags are invalid
4. **No breaking changes**: Existing usage continues to work

### For Maintenance

1. **Less code**: ~500-800 lines removed
2. **Easier debugging**: Single code path to investigate
3. **Simpler reviews**: Changes to flag handling in one place
4. **Better docs**: Clear patterns to document once

## Risks & Mitigations

### Risk: Breaking Existing Functionality

**Likelihood**: Medium
**Impact**: High

**Mitigation**:
- Comprehensive backward compatibility tests
- Gradual migration (one command at a time)
- Beta testing period
- Feature flags for gradual rollout
- Can revert per command if needed

### Risk: Increased Complexity

**Likelihood**: Medium
**Impact**: Medium

**Mitigation**:
- Clear documentation
- Simple, focused interfaces
- Code examples for common patterns
- Consistent architecture across commands

### Risk: Performance Regression

**Likelihood**: Low
**Impact**: Medium

**Mitigation**:
- Benchmark tests in CI
- Heatmap analysis
- Performance profiling
- Optimization if needed

## Timeline

**Total**: ~6 weeks

- **Week 1**: Core infrastructure (interfaces, implementations, tests)
- **Week 2**: Pass-through commands (Terraform, Helmfile, Packer)
- **Week 3**: Standard commands (Validate, Describe, Workflow, etc.)
- **Week 4**: Global flags (RootCmd, propagation)
- **Week 5**: Custom commands (dynamic flags, identity)
- **Week 6**: Cleanup, documentation, final testing

## Next Steps

1. **Review & approve PRD**: Get team alignment on approach
2. **Create POC**: Implement core interfaces with one command
3. **Validate approach**: Ensure pattern works for all command types
4. **Begin migration**: Start with Terraform command
5. **Iterate**: Refine based on learnings

## References

### Key Articles

- [Sting of the Viper](https://carolynvanslyck.com/blog/2020/08/sting-of-the-viper/) - **Essential reading**
- [Testing Flag Parsing in Go](https://eli.thegreenplace.net/2020/testing-flag-parsing-in-go-programs/)
- [Enterprise Guide to Cobra](https://cobra.dev/docs/explanations/enterprise-guide/)
- [Functional Options Pattern](https://www.codingexplorations.com/blog/functional-options-pattern-go)

### Libraries

- [spf13/cobra](https://github.com/spf13/cobra) - CLI framework
- [spf13/viper](https://github.com/spf13/viper) - Configuration
- [go-extras/cobraflags](https://github.com/go-extras/cobraflags) - Integration helper

### Source Code Examples

- [docker/cli](https://github.com/docker/cli/blob/master/cli/cobra.go) - TopLevelCommand pattern
- [kubernetes/kubectl](https://github.com/kubernetes/kubectl) - Global flags
- [helm/helm](https://github.com/helm/helm) - Flag composition

### Internal Docs

- `docs/prd/command-registry-pattern.md` - Command registration
- `docs/prd/testing-strategy.md` - Testing approach
- `CLAUDE.md` - Development guidelines
