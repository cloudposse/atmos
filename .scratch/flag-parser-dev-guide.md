# Developer Guide: Unified Flag Parser

## Overview

The unified flag parser (`pkg/flagparser/`) provides a pure functional, highly testable system for parsing command-line arguments in Atmos. It handles the complex scenario of parsing three concurrent types (Atmos flags, tool flags, positional args) while maintaining clean separation and testability.

---

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "github.com/cloudposse/atmos/pkg/flagparser"
)

func main() {
    // 1. Create flag registry
    registry := flagparser.NewAtmosRegistry()

    // 2. Set parse options
    opts := flagparser.ParseOptions{
        EnableImplicitMode: true,
    }

    // 3. Parse arguments
    args := []string{"plan", "vpc", "--stack", "prod", "-var", "foo=bar"}
    result, err := flagparser.Parse(args, registry, opts)
    if err != nil {
        panic(err)
    }

    // 4. Use parsed results
    stack := result.AtmosFlags["stack"].(string)  // "prod"
    component := result.PositionalArgs[1]          // "vpc"
    toolFlags := result.PassThroughArgs            // ["-var", "foo=bar"]
}
```

---

## Core Concepts

### 1. Three Concurrent Types

The parser handles three types of arguments simultaneously:

#### Atmos Flags (Double-Dash, GNU-style)
```bash
--stack prod
--dry-run
--identity admin
-s prod          # shorthand
```

#### Tool Flags (Single-Dash, POSIX-style)
```bash
-var 'foo=bar'
-var-file=prod.tfvars
-out=plan.tfplan
```

#### Positional Arguments
```bash
plan             # subcommand
vpc              # component name
```

### 2. Two Parsing Modes

**Explicit Mode** (with `--` separator):
```bash
atmos terraform plan vpc --stack prod -- -var foo=bar
                                       ^^ everything after goes to tool
```

**Implicit Mode** (no separator):
```bash
atmos terraform plan vpc --stack prod -var foo=bar
                         ^^^^^^^^^^^ ^^^^^^^^^^^^
                         Atmos       Tool (unknown flags)
```

### 3. Pure Functional Design

All parsing functions are **pure**:
- No I/O operations
- No side effects
- Deterministic output for same inputs
- Easy to test and reason about

```go
// ✅ Pure function - testable
func Parse(args []string, registry *FlagRegistry, opts ParseOptions) (*ParseResult, error)

// ❌ NOT pure - has side effects
func Parse(args []string) (*ParseResult, error) {
    config := readConfigFile()  // I/O!
    log.Info("parsing")          // Side effect!
    return result, nil
}
```

---

## API Reference

### Types

#### ParseResult
```go
type ParseResult struct {
    // AtmosFlags contains parsed Atmos-specific flags
    AtmosFlags map[string]interface{}

    // PositionalArgs contains non-flag arguments
    PositionalArgs []string

    // PassThroughArgs contains flags/args for underlying tool
    PassThroughArgs []string

    // RawArgs is the original unparsed input
    RawArgs []string
}
```

#### FlagDefinition
```go
type FlagDefinition struct {
    Name      string   // Long name (e.g., "stack")
    Shorthand string   // Short name (e.g., "s")
    Type      FlagType // string, bool, int, etc.
    Default   interface{}

    // For optional boolean flags (--upload-status)
    OptionalValue         bool
    DefaultValueWhenAlone interface{}
}
```

#### ParseOptions
```go
type ParseOptions struct {
    // EnableImplicitMode allows parsing without -- separator
    EnableImplicitMode bool

    // StopAtFirstPositional stops parsing flags after first positional arg
    StopAtFirstPositional bool

    // PreserveQuotes maintains quote characters in parsed values
    PreserveQuotes bool
}
```

### Functions

#### Parse
```go
func Parse(args []string, registry *FlagRegistry, opts ParseOptions) (*ParseResult, error)
```

Main entry point for flag parsing. Pure function with no side effects.

**Example**:
```go
registry := flagparser.NewAtmosRegistry()
opts := flagparser.ParseOptions{EnableImplicitMode: true}
result, err := flagparser.Parse(
    []string{"plan", "vpc", "--stack", "prod", "--", "-var", "x=1"},
    registry,
    opts,
)
// result.AtmosFlags: {"stack": "prod"}
// result.PositionalArgs: ["plan", "vpc"]
// result.PassThroughArgs: ["-var", "x=1"]
```

#### NewAtmosRegistry
```go
func NewAtmosRegistry() *FlagRegistry
```

Creates a registry with all standard Atmos flags pre-registered.

**Registered flags**:
- `--stack` / `-s`
- `--identity` / `-i`
- `--dry-run`
- `--skip-init`
- `--upload-status` (optional bool)
- `--from-plan`
- And all other Atmos global flags

---

## Testing Guide

### Using TestKit for Isolated Tests

**ALWAYS use TestKit** when testing flag parsing with commands:

```go
func TestMyCommand(t *testing.T) {
    t := cmd.NewTestKit(t)  // ✅ Automatic cleanup
    // Test code here
    // RootCmd state automatically restored
}
```

**Why TestKit?**
- Prevents test pollution from global `RootCmd` state
- Automatically snapshots and restores flags
- Works with subtests and table-driven tests
- Restores `os.Args` after test

### Testing with Mock Component

Use mock component for comprehensive edge case testing:

```go
func TestFlagParser_MockComponent(t *testing.T) {
    t := cmd.NewTestKit(t)

    registry := flagparser.NewAtmosRegistry()
    opts := flagparser.ParseOptions{EnableImplicitMode: true}

    tests := []struct {
        name    string
        args    []string
        want    *flagparser.ParseResult
        wantErr bool
    }{
        {
            name: "mock component with crazy characters",
            args: []string{"plan", "my-comp/v1.2.3", "--stack", "prod/us-east-1", "-var", "'foo=bar baz'"},
            want: &flagparser.ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack": "prod/us-east-1",
                },
                PositionalArgs:  []string{"plan", "my-comp/v1.2.3"},
                PassThroughArgs: []string{"-var", "'foo=bar baz'"},
            },
        },
        // Add 100+ edge cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := flagparser.Parse(tt.args, registry, opts)

            if tt.wantErr {
                require.Error(t, err)
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tt.want.AtmosFlags, got.AtmosFlags)
            assert.Equal(t, tt.want.PositionalArgs, got.PositionalArgs)
            assert.Equal(t, tt.want.PassThroughArgs, got.PassThroughArgs)
        })
    }
}
```

### Comprehensive Edge Case Test Suite

Create pain-in-the-butt tests that cover every exception:

```go
func TestFlagParser_PainInTheButtEdgeCases(t *testing.T) {
    t := cmd.NewTestKit(t)

    registry := flagparser.NewAtmosRegistry()
    opts := flagparser.ParseOptions{EnableImplicitMode: true}

    painfulTests := []struct {
        name    string
        args    []string
        want    *flagparser.ParseResult
        wantErr bool
        errType error
    }{
        {
            name:    "component name looks like flag",
            args:    []string{"plan", "-s", "--stack", "prod"},
            wantErr: false,  // Should handle gracefully
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "-s"},
                PassThroughArgs: []string{},
            },
        },
        {
            name: "stack name with slashes and colons",
            args: []string{"plan", "vpc", "--stack", "prod/us-east-1:v2"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod/us-east-1:v2"},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{},
            },
        },
        {
            name: "flag value contains equals sign",
            args: []string{"plan", "vpc", "--stack", "prod", "-var", "url=https://example.com?foo=bar"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "url=https://example.com?foo=bar"},
            },
        },
        {
            name: "flag value contains dashes",
            args: []string{"plan", "vpc", "-s", "prod", "-var", "name=my-cool-app"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "name=my-cool-app"},
            },
        },
        {
            name: "mixed quotes in value",
            args: []string{"plan", "vpc", "-s", "prod", "-var", `message="it's working"`},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", `message="it's working"`},
            },
        },
        {
            name: "nested quotes",
            args: []string{"plan", "vpc", "-s", "prod", "-var", `json={"key":"value"}`},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", `json={"key":"value"}`},
            },
        },
        {
            name: "trailing whitespace in value",
            args: []string{"plan", "vpc", "--stack", "prod  ", "-var", "x=1"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod  "},  // Preserve whitespace
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1"},
            },
        },
        {
            name: "file path with spaces",
            args: []string{"plan", "vpc", "-s", "prod", "-var-file", "my file.tfvars"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var-file", "my file.tfvars"},
            },
        },
        {
            name: "unicode in component name",
            args: []string{"plan", "my-cômpönent", "--stack", "prod"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "my-cômpönent"},
                PassThroughArgs: []string{},
            },
        },
        {
            name: "very long argument list",
            args: func() []string {
                args := []string{"plan", "vpc", "--stack", "prod"}
                for i := 0; i < 100; i++ {
                    args = append(args, "-var", fmt.Sprintf("key%d=value%d", i, i))
                }
                return args
            }(),
            want: &flagparser.ParseResult{
                AtmosFlags:     map[string]interface{}{"stack": "prod"},
                PositionalArgs: []string{"plan", "vpc"},
                PassThroughArgs: func() []string {
                    args := []string{}
                    for i := 0; i < 100; i++ {
                        args = append(args, "-var", fmt.Sprintf("key%d=value%d", i, i))
                    }
                    return args
                }(),
            },
        },
        {
            name: "empty flag value",
            args: []string{"plan", "vpc", "--stack", "", "-var", "x=1"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": ""},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1"},
            },
        },
        {
            name: "repeated flags",
            args: []string{"plan", "vpc", "-s", "prod", "-var", "x=1", "-var", "y=2", "-var", "z=3"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{"stack": "prod"},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1", "-var", "y=2", "-var", "z=3"},
            },
        },
        {
            name: "flag that looks like positional",
            args: []string{"plan", "--", "-s"},
            want: &flagparser.ParseResult{
                AtmosFlags:      map[string]interface{}{},
                PositionalArgs:  []string{"plan"},
                PassThroughArgs: []string{"-s"},
            },
        },
        {
            name:    "missing required flag value",
            args:    []string{"plan", "vpc", "--stack"},
            wantErr: true,
            errType: flagparser.ErrMissingFlagValue,
        },
        {
            name:    "invalid boolean value",
            args:    []string{"plan", "vpc", "--dry-run=maybe"},
            wantErr: true,
            errType: flagparser.ErrInvalidBoolValue,
        },
    }

    for _, tt := range painfulTests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := flagparser.Parse(tt.args, registry, opts)

            if tt.wantErr {
                require.Error(t, err)
                if tt.errType != nil {
                    assert.ErrorIs(t, err, tt.errType)
                }
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tt.want.AtmosFlags, got.AtmosFlags)
            assert.Equal(t, tt.want.PositionalArgs, got.PositionalArgs)
            assert.Equal(t, tt.want.PassThroughArgs, got.PassThroughArgs)
        })
    }
}
```

---

## Integration with Existing Code

### Early Flag Extraction for Log Configuration

The parser supports extracting flags **before full config loading** to solve the log level initialization problem:

```go
// cmd/root.go - PersistentPreRun

func (cmd *cobra.Command, args []string) {
    // 1. EARLY: Extract log flags before config loading
    registry := flagparser.NewAtmosRegistry()
    opts := flagparser.ParseOptions{EnableImplicitMode: true}

    result, err := flagparser.Parse(os.Args[1:], registry, opts)
    if err != nil {
        // Handle gracefully - maybe just use defaults
    }

    // 2. Initialize logger with extracted flags
    logLevel := "Info"  // default
    if level, ok := result.AtmosFlags["logs-level"].(string); ok {
        logLevel = level
    } else if envLevel := os.Getenv("ATMOS_LOGS_LEVEL"); envLevel != "" {
        logLevel = envLevel
    }

    log.InitLogger(logLevel)

    // 3. NOW load full config
    config, err := cfg.InitCliConfig(configAndStacksInfo, true)
    // ...
}
```

### Replacing Custom Flag Parsing

Replace manual parsing in `setLogConfig()`:

```go
// OLD (pkg/config/config.go)
func setLogConfig(atmosConfig *schema.AtmosConfiguration) {
    flagKeyValue := parseFlags()  // ❌ Manual parsing
    if v, ok := flagKeyValue["logs-level"]; ok {
        atmosConfig.Logs.Level = v
    }
}

// NEW
func setLogConfig(atmosConfig *schema.AtmosConfiguration, parsedFlags *flagparser.ParseResult) {
    if v, ok := parsedFlags.AtmosFlags["logs-level"].(string); ok {
        atmosConfig.Logs.Level = v
    }
}
```

### Component Registry Integration

Use with mock component:

```go
func TestMockComponentWithFlags(t *testing.T) {
    t := cmd.NewTestKit(t)

    // Parse flags
    registry := flagparser.NewAtmosRegistry()
    result, err := flagparser.Parse(
        []string{"plan", "my-mock-comp", "--stack", "test"},
        registry,
        flagparser.ParseOptions{EnableImplicitMode: true},
    )
    require.NoError(t, err)

    // Execute mock component
    ctx := &component.ExecutionContext{
        ComponentType: "mock",
        Component:     result.PositionalArgs[1],
        Stack:         result.AtmosFlags["stack"].(string),
        Command:       result.PositionalArgs[0],
    }

    provider := &mock.MockComponentProvider{}
    err = provider.Execute(ctx)
    require.NoError(t, err)
}
```

---

## Best Practices

### 1. Always Use TestKit

```go
// ✅ Correct
func TestMyCommand(t *testing.T) {
    t := cmd.NewTestKit(t)
    // ...
}

// ❌ Wrong - global state pollution
func TestMyCommand(t *testing.T) {
    // Modifies global RootCmd
}
```

### 2. Test Pure Functions

```go
// ✅ Correct - pure function
func TestParse(t *testing.T) {
    result, err := flagparser.Parse(args, registry, opts)
    // Easy to test, no mocks needed
}

// ❌ Wrong - side effects
func TestParse(t *testing.T) {
    result := parseAndLoadConfig(args)  // I/O inside!
}
```

### 3. Cover Edge Cases

Write tests for every exception:
- Missing values
- Invalid types
- Special characters
- Unicode
- Very long inputs
- Empty inputs
- Repeated flags
- Conflicting flags

### 4. Use Table-Driven Tests

```go
func TestParse_EdgeCases(t *testing.T) {
    t := cmd.NewTestKit(t)

    tests := []struct {
        name    string
        args    []string
        want    *flagparser.ParseResult
        wantErr bool
    }{
        // 100+ test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test code
        })
    }
}
```

---

## Common Pitfalls

### ❌ Modifying Global State in Tests

```go
// BAD
func TestCommand(t *testing.T) {
    os.Args = []string{"atmos", "test"}  // Pollutes global state!
    // ...
}

// GOOD
func TestCommand(t *testing.T) {
    t := cmd.NewTestKit(t)  // Automatic cleanup
    os.Args = []string{"atmos", "test"}
    // Automatically restored after test
}
```

### ❌ Not Testing with Mock Component

```go
// BAD - only tests with real Terraform
func TestFlagParsing(t *testing.T) {
    // Requires Terraform installed
}

// GOOD - tests with mock component
func TestFlagParsing(t *testing.T) {
    // Works without external dependencies
    // Tests component registry pattern
}
```

### ❌ Forgetting Edge Cases

```go
// BAD - only happy path
func TestParse(t *testing.T) {
    result, _ := flagparser.Parse([]string{"plan", "vpc"}, ...)
    assert.Equal(t, "vpc", result.PositionalArgs[1])
}

// GOOD - includes edge cases
func TestParse(t *testing.T) {
    tests := []struct{...}{
        {"happy path", ...},
        {"component name with dash", ...},
        {"unicode in value", ...},
        {"very long args", ...},
        // ... 100+ cases
    }
}
```

---

## Performance Considerations

- Parser should be <1ms for typical commands
- Benchmark long argument lists (100+ args)
- Profile memory allocation
- Avoid string concatenation in loops
- Reuse slices where possible

```go
func BenchmarkParse(b *testing.B) {
    registry := flagparser.NewAtmosRegistry()
    args := []string{"plan", "vpc", "--stack", "prod", "-var", "foo=bar"}
    opts := flagparser.ParseOptions{EnableImplicitMode: true}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = flagparser.Parse(args, registry, opts)
    }
}
```

---

## Troubleshooting

### Parser Not Recognizing Flag

**Problem**: Flag is being passed through instead of extracted

**Solution**: Check flag is registered in registry

```go
registry := flagparser.NewAtmosRegistry()
// Verify flag exists
def := registry.Lookup("my-flag")
if def == nil {
    // Flag not registered!
}
```

### Test Pollution

**Problem**: Tests fail when run together but pass individually

**Solution**: Use TestKit

```go
func TestMyCommand(t *testing.T) {
    t := cmd.NewTestKit(t)  // ← Add this
    // ...
}
```

### Ambiguous Parsing

**Problem**: Not sure if argument is flag or positional

**Solution**: Use `--` separator for explicit separation

```bash
# Ambiguous
atmos terraform plan -s prod

# Clear
atmos terraform plan -s prod -- <anything here is positional>
```

---

## Examples

See comprehensive examples in:
- `pkg/flagparser/parser_test.go` - Core parsing tests
- `pkg/flagparser/mock_integration_test.go` - Mock component tests
- `cmd/terraform_test.go` - Integration with Terraform
- `.scratch/flag-handling-examples.go` - Additional examples

---

## Support

For questions or issues:
1. Check this guide first
2. Review test cases in `pkg/flagparser/*_test.go`
3. Check PRD: `docs/prd/unified-flag-parsing.md`
4. Create issue with reproduction case
