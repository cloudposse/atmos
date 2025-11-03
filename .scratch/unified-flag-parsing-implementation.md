# Implementation Plan: Unified Flag Parser

## Philosophy

**Build standalone, test exhaustively, integrate carefully.**

1. **Pure functional design** - No side effects, easy to test
2. **Test-first approach** - Achieve 90% coverage before integration
3. **Baseline establishment** - Capture current behavior before changes
4. **Regression prevention** - Verify no behavior changes during migration

---

## Phase 1: Pure Functional Parser (Week 1)

### Goals

- Build `pkg/flagparser/` as standalone library
- 90% minimum test coverage
- Zero dependencies on Atmos internals
- Pure functions, no I/O, no side effects

### 1.1: Core Data Types (Day 1)

Create pure data structures for flag parsing:

```go
// pkg/flagparser/types.go

package flagparser

// ParseResult represents the output of flag parsing.
type ParseResult struct {
    // AtmosFlags contains parsed Atmos-specific flags
    AtmosFlags map[string]interface{}

    // PositionalArgs contains non-flag arguments (component, subcommand, etc.)
    PositionalArgs []string

    // PassThroughArgs contains flags/args to pass to underlying tool
    PassThroughArgs []string

    // RawArgs is the original unparsed input
    RawArgs []string
}

// FlagDefinition defines an Atmos flag.
type FlagDefinition struct {
    Name      string   // Long name (e.g., "stack")
    Shorthand string   // Short name (e.g., "s")
    Type      FlagType // string, bool, int, etc.
    Default   interface{}

    // OptionalValue indicates flag can appear without value (for booleans)
    OptionalValue bool

    // DefaultValueWhenAlone is used when flag appears without value
    // e.g., --upload-status (alone) defaults to true
    DefaultValueWhenAlone interface{}
}

// FlagType represents the type of a flag value.
type FlagType int

const (
    FlagTypeString FlagType = iota
    FlagTypeBool
    FlagTypeInt
    FlagTypeStringSlice
)

// FlagRegistry holds all known Atmos flag definitions.
type FlagRegistry struct {
    flags          []*FlagDefinition
    byName         map[string]*FlagDefinition
    byShorthand    map[string]*FlagDefinition
}

// ParseOptions configures the parser behavior.
type ParseOptions struct {
    // EnableImplicitMode allows parsing without -- separator
    EnableImplicitMode bool

    // StopAtFirstPositional stops parsing flags after first positional arg
    StopAtFirstPositional bool

    // PreserveQuotes maintains quote characters in parsed values
    PreserveQuotes bool
}
```

**Tests for types.go** (90% coverage):
```go
// pkg/flagparser/types_test.go

func TestFlagRegistry_Register(t *testing.T) { /* ... */ }
func TestFlagRegistry_Lookup(t *testing.T) { /* ... */ }
func TestFlagRegistry_LookupByShorthand(t *testing.T) { /* ... */ }
func TestParseResult_Empty(t *testing.T) { /* ... */ }
```

### 1.2: Pure Parsing Functions (Day 2-3)

Implement core parsing logic as pure functions:

```go
// pkg/flagparser/parser.go

package flagparser

// Parse is the main entry point for flag parsing.
// It is a pure function - no side effects, deterministic output.
func Parse(args []string, registry *FlagRegistry, opts ParseOptions) (*ParseResult, error) {
    result := &ParseResult{
        AtmosFlags:      make(map[string]interface{}),
        PositionalArgs:  []string{},
        PassThroughArgs: []string{},
        RawArgs:         args,
    }

    // Step 1: Split at -- separator if present
    beforeDash, afterDash := splitAtDoubleDash(args)

    if len(afterDash) > 0 {
        // Explicit mode: parse beforeDash, pass afterDash through
        return parseExplicitMode(beforeDash, afterDash, registry, opts)
    }

    if opts.EnableImplicitMode {
        // Implicit mode: extract known flags, pass rest through
        return parseImplicitMode(args, registry, opts)
    }

    // No separator and implicit mode disabled = treat all as pass-through
    result.PassThroughArgs = args
    return result, nil
}

// splitAtDoubleDash splits args at -- separator.
// Pure function - no side effects.
func splitAtDoubleDash(args []string) (before, after []string) {
    for i, arg := range args {
        if arg == "--" {
            return args[:i], args[i+1:]
        }
    }
    return args, nil
}

// parseExplicitMode handles parsing when -- separator is present.
func parseExplicitMode(beforeDash, afterDash []string, registry *FlagRegistry, opts ParseOptions) (*ParseResult, error) {
    result := &ParseResult{
        AtmosFlags:      make(map[string]interface{}),
        PositionalArgs:  []string{},
        PassThroughArgs: afterDash, // Everything after -- passes through
        RawArgs:         append(append([]string{}, beforeDash...), append([]string{"--"}, afterDash...)...),
    }

    // Parse only beforeDash for Atmos flags
    err := extractAtmosFlags(beforeDash, registry, result, opts)
    if err != nil {
        return nil, err
    }

    return result, nil
}

// parseImplicitMode handles parsing without -- separator.
func parseImplicitMode(args []string, registry *FlagRegistry, opts ParseOptions) (*ParseResult, error) {
    result := &ParseResult{
        AtmosFlags:      make(map[string]interface{}),
        PositionalArgs:  []string{},
        PassThroughArgs: []string{},
        RawArgs:         args,
    }

    // Extract known Atmos flags, treat unknown as pass-through
    err := extractAtmosFlagsImplicit(args, registry, result, opts)
    if err != nil {
        return nil, err
    }

    return result, nil
}

// extractAtmosFlags extracts Atmos flags from args.
// Remaining args become positional or pass-through.
func extractAtmosFlags(args []string, registry *FlagRegistry, result *ParseResult, opts ParseOptions) error {
    i := 0
    for i < len(args) {
        arg := args[i]

        // Not a flag? Must be positional arg
        if !strings.HasPrefix(arg, "-") {
            result.PositionalArgs = append(result.PositionalArgs, arg)
            i++
            continue
        }

        // Try to parse as Atmos flag
        consumed, err := tryParseAtmosFlag(args[i:], registry, result, opts)
        if err != nil {
            return err
        }

        if consumed > 0 {
            // Successfully parsed Atmos flag
            i += consumed
            continue
        }

        // Not an Atmos flag - treat as pass-through
        result.PassThroughArgs = append(result.PassThroughArgs, arg)
        i++
    }

    return nil
}

// tryParseAtmosFlag attempts to parse an Atmos flag starting at args[0].
// Returns number of args consumed (0 if not an Atmos flag), or error.
func tryParseAtmosFlag(args []string, registry *FlagRegistry, result *ParseResult, opts ParseOptions) (int, error) {
    if len(args) == 0 {
        return 0, nil
    }

    arg := args[0]

    // Handle --flag=value form
    if strings.Contains(arg, "=") {
        return parseEqualsForm(arg, registry, result, opts)
    }

    // Handle --flag or -f form
    flagName, isShorthand := parseFlagName(arg)

    var def *FlagDefinition
    if isShorthand {
        def = registry.byShorthand[flagName]
    } else {
        def = registry.byName[flagName]
    }

    if def == nil {
        // Not a known Atmos flag
        return 0, nil
    }

    // Handle optional value flags (like --upload-status)
    if def.OptionalValue && (len(args) == 1 || strings.HasPrefix(args[1], "-")) {
        // Flag appears alone or next arg is another flag
        result.AtmosFlags[def.Name] = def.DefaultValueWhenAlone
        return 1, nil
    }

    // Flag requires value
    if len(args) < 2 {
        return 0, fmt.Errorf("flag --%s requires a value", def.Name)
    }

    // Parse value based on type
    value, err := parseValue(args[1], def.Type, opts)
    if err != nil {
        return 0, fmt.Errorf("invalid value for flag --%s: %w", def.Name, err)
    }

    result.AtmosFlags[def.Name] = value
    return 2, nil // Consumed flag + value
}

// parseEqualsForm handles --flag=value format.
func parseEqualsForm(arg string, registry *FlagRegistry, result *ParseResult, opts ParseOptions) (int, error) {
    parts := strings.SplitN(arg, "=", 2)
    flagPart := parts[0]
    valuePart := ""
    if len(parts) > 1 {
        valuePart = parts[1]
    }

    flagName, _ := parseFlagName(flagPart)
    def := registry.byName[flagName]

    if def == nil {
        // Not a known Atmos flag
        return 0, nil
    }

    // Handle empty value (--flag=)
    if valuePart == "" && def.OptionalValue {
        result.AtmosFlags[def.Name] = def.DefaultValueWhenAlone
        return 1, nil
    }

    value, err := parseValue(valuePart, def.Type, opts)
    if err != nil {
        return 0, fmt.Errorf("invalid value for flag --%s: %w", def.Name, err)
    }

    result.AtmosFlags[def.Name] = value
    return 1, nil
}

// parseFlagName extracts flag name from --flag or -f.
// Returns (name, isShorthand).
func parseFlagName(arg string) (string, bool) {
    if strings.HasPrefix(arg, "--") {
        return strings.TrimPrefix(arg, "--"), false
    }
    if strings.HasPrefix(arg, "-") {
        return strings.TrimPrefix(arg, "-"), true
    }
    return arg, false
}

// parseValue converts string to typed value.
func parseValue(s string, typ FlagType, opts ParseOptions) (interface{}, error) {
    if !opts.PreserveQuotes {
        s = strings.Trim(s, "\"'")
    }

    switch typ {
    case FlagTypeString:
        return s, nil
    case FlagTypeBool:
        return parseBool(s)
    case FlagTypeInt:
        return strconv.Atoi(s)
    case FlagTypeStringSlice:
        return []string{s}, nil // Single value for now, accumulate later
    default:
        return nil, fmt.Errorf("unsupported flag type: %v", typ)
    }
}

// parseBool handles various boolean representations.
func parseBool(s string) (bool, error) {
    s = strings.ToLower(strings.TrimSpace(s))
    switch s {
    case "true", "1", "yes", "on", "t", "y":
        return true, nil
    case "false", "0", "no", "off", "f", "n", "":
        return false, nil
    default:
        return false, fmt.Errorf("invalid boolean value: %s", s)
    }
}

// extractAtmosFlagsImplicit handles implicit mode (no -- separator).
func extractAtmosFlagsImplicit(args []string, registry *FlagRegistry, result *ParseResult, opts ParseOptions) error {
    remaining := []string{}

    i := 0
    for i < len(args) {
        arg := args[i]

        // Not a flag? Could be positional arg
        if !strings.HasPrefix(arg, "-") {
            // Add to positional args
            result.PositionalArgs = append(result.PositionalArgs, arg)
            i++
            continue
        }

        // Try to parse as Atmos flag
        consumed, err := tryParseAtmosFlag(args[i:], registry, result, opts)
        if err != nil {
            return err
        }

        if consumed > 0 {
            // Successfully parsed Atmos flag
            i += consumed
            continue
        }

        // Not an Atmos flag - add to pass-through
        remaining = append(remaining, arg)

        // If this flag takes a value (next arg doesn't start with -), include that too
        if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
            remaining = append(remaining, args[i+1])
            i += 2
        } else {
            i++
        }
    }

    result.PassThroughArgs = remaining
    return nil
}
```

**Comprehensive tests** (90% coverage target):

```go
// pkg/flagparser/parser_test.go

package flagparser

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// setupTestRegistry creates a test flag registry.
func setupTestRegistry() *FlagRegistry {
    registry := NewFlagRegistry()

    // Add common Atmos flags
    registry.Register(&FlagDefinition{
        Name:      "stack",
        Shorthand: "s",
        Type:      FlagTypeString,
    })

    registry.Register(&FlagDefinition{
        Name:      "dry-run",
        Shorthand: "",
        Type:      FlagTypeBool,
        Default:   false,
    })

    registry.Register(&FlagDefinition{
        Name:                  "upload-status",
        Shorthand:             "",
        Type:                  FlagTypeBool,
        OptionalValue:         true,
        DefaultValueWhenAlone: true,
    })

    registry.Register(&FlagDefinition{
        Name:      "identity",
        Shorthand: "i",
        Type:      FlagTypeString,
    })

    return registry
}

func TestSplitAtDoubleDash(t *testing.T) {
    tests := []struct {
        name  string
        args  []string
        before []string
        after  []string
    }{
        {
            name:   "with separator",
            args:   []string{"plan", "vpc", "--stack", "prod", "--", "-var", "foo=bar"},
            before: []string{"plan", "vpc", "--stack", "prod"},
            after:  []string{"-var", "foo=bar"},
        },
        {
            name:   "without separator",
            args:   []string{"plan", "vpc", "--stack", "prod"},
            before: []string{"plan", "vpc", "--stack", "prod"},
            after:  nil,
        },
        {
            name:   "separator at start",
            args:   []string{"--", "plan", "vpc"},
            before: []string{},
            after:  []string{"plan", "vpc"},
        },
        {
            name:   "separator at end",
            args:   []string{"plan", "vpc", "--"},
            before: []string{"plan", "vpc"},
            after:  []string{},
        },
        {
            name:   "empty args",
            args:   []string{},
            before: []string{},
            after:  nil,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            before, after := splitAtDoubleDash(tt.args)
            assert.Equal(t, tt.before, before)
            assert.Equal(t, tt.after, after)
        })
    }
}

func TestParse_ExplicitMode(t *testing.T) {
    registry := setupTestRegistry()

    tests := []struct {
        name    string
        args    []string
        opts    ParseOptions
        want    *ParseResult
        wantErr bool
    }{
        {
            name: "simple explicit mode",
            args: []string{"plan", "vpc", "--stack", "prod", "--", "-var", "foo=bar"},
            opts: ParseOptions{EnableImplicitMode: true},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack": "prod",
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "foo=bar"},
            },
        },
        {
            name: "explicit mode with shorthand",
            args: []string{"plan", "vpc", "-s", "prod", "--", "-out=plan.tfplan"},
            opts: ParseOptions{EnableImplicitMode: true},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack": "prod",
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-out=plan.tfplan"},
            },
        },
        {
            name: "explicit mode with optional bool",
            args: []string{"plan", "vpc", "--upload-status", "--", "-var", "x=1"},
            opts: ParseOptions{EnableImplicitMode: true},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "upload-status": true,
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1"},
            },
        },
        {
            name: "explicit mode with optional bool false",
            args: []string{"plan", "vpc", "--upload-status=false", "--", "-var", "x=1"},
            opts: ParseOptions{EnableImplicitMode: true},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "upload-status": false,
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1"},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Parse(tt.args, registry, tt.opts)
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

func TestParse_ImplicitMode(t *testing.T) {
    registry := setupTestRegistry()

    tests := []struct {
        name    string
        args    []string
        want    *ParseResult
        wantErr bool
    }{
        {
            name: "interleaved Atmos and tool flags",
            args: []string{"plan", "vpc", "--stack", "prod", "-var", "foo=bar", "--dry-run", "-out=plan.tfplan"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack":   "prod",
                    "dry-run": true,
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "foo=bar", "-out=plan.tfplan"},
            },
        },
        {
            name: "shorthand mixed with tool flags",
            args: []string{"plan", "vpc", "-s", "prod", "-var", "x=1", "-i", "admin", "-out=plan"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack":    "prod",
                    "identity": "admin",
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1", "-out=plan"},
            },
        },
        {
            name: "all three types concurrent",
            args: []string{"plan", "vpc", "--stack", "prod", "-var", "env=prod", "--dry-run", "-out=plan.tfplan"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack":   "prod",
                    "dry-run": true,
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "env=prod", "-out=plan.tfplan"},
            },
        },
        {
            name: "optional bool in implicit mode",
            args: []string{"plan", "vpc", "-s", "prod", "--upload-status", "-var", "x=1"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack":         "prod",
                    "upload-status": true,
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1"},
            },
        },
        {
            name: "equals form flags",
            args: []string{"plan", "vpc", "--stack=prod", "-var-file=vars.tfvars", "--dry-run=true"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack":   "prod",
                    "dry-run": true,
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var-file=vars.tfvars"},
            },
        },
    }

    opts := ParseOptions{EnableImplicitMode: true}

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Parse(tt.args, registry, opts)
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

func TestParse_EdgeCases(t *testing.T) {
    registry := setupTestRegistry()
    opts := ParseOptions{EnableImplicitMode: true}

    tests := []struct {
        name    string
        args    []string
        want    *ParseResult
        wantErr bool
    }{
        {
            name: "empty args",
            args: []string{},
            want: &ParseResult{
                AtmosFlags:      map[string]interface{}{},
                PositionalArgs:  []string{},
                PassThroughArgs: []string{},
            },
        },
        {
            name: "only positional args",
            args: []string{"plan", "vpc"},
            want: &ParseResult{
                AtmosFlags:      map[string]interface{}{},
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{},
            },
        },
        {
            name: "only tool flags",
            args: []string{"-var", "foo=bar", "-out=plan.tfplan"},
            want: &ParseResult{
                AtmosFlags:      map[string]interface{}{},
                PositionalArgs:  []string{},
                PassThroughArgs: []string{"-var", "foo=bar", "-out=plan.tfplan"},
            },
        },
        {
            name: "only Atmos flags",
            args: []string{"--stack", "prod", "--dry-run"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack":   "prod",
                    "dry-run": true,
                },
                PositionalArgs:  []string{},
                PassThroughArgs: []string{},
            },
        },
        {
            name: "positional arg that looks like flag",
            args: []string{"--", "-s"},
            want: &ParseResult{
                AtmosFlags:      map[string]interface{}{},
                PositionalArgs:  []string{},
                PassThroughArgs: []string{"-s"},
            },
        },
        {
            name: "repeated tool flags",
            args: []string{"plan", "vpc", "-s", "prod", "-var", "x=1", "-var", "y=2"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack": "prod",
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "x=1", "-var", "y=2"},
            },
        },
        {
            name: "quoted values in tool flags",
            args: []string{"plan", "vpc", "-s", "prod", "-var", "'foo=bar baz'"},
            want: &ParseResult{
                AtmosFlags: map[string]interface{}{
                    "stack": "prod",
                },
                PositionalArgs:  []string{"plan", "vpc"},
                PassThroughArgs: []string{"-var", "'foo=bar baz'"},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Parse(tt.args, registry, opts)
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

func TestParseBool(t *testing.T) {
    tests := []struct {
        input   string
        want    bool
        wantErr bool
    }{
        {"true", true, false},
        {"false", false, false},
        {"1", true, false},
        {"0", false, false},
        {"yes", true, false},
        {"no", false, false},
        {"on", true, false},
        {"off", false, false},
        {"t", true, false},
        {"f", false, false},
        {"y", true, false},
        {"n", false, false},
        {"TRUE", true, false},
        {"FALSE", false, false},
        {"", false, false},
        {"invalid", false, true},
        {"maybe", false, true},
    }

    for _, tt := range tests {
        t.Run(tt.input, func(t *testing.T) {
            got, err := parseBool(tt.input)
            if tt.wantErr {
                require.Error(t, err)
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}

// Benchmark tests for performance
func BenchmarkParse_ExplicitMode(b *testing.B) {
    registry := setupTestRegistry()
    args := []string{"plan", "vpc", "--stack", "prod", "--dry-run", "--", "-var", "foo=bar", "-out=plan.tfplan"}
    opts := ParseOptions{EnableImplicitMode: true}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = Parse(args, registry, opts)
    }
}

func BenchmarkParse_ImplicitMode(b *testing.B) {
    registry := setupTestRegistry()
    args := []string{"plan", "vpc", "--stack", "prod", "-var", "foo=bar", "--dry-run", "-out=plan.tfplan"}
    opts := ParseOptions{EnableImplicitMode: true}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = Parse(args, registry, opts)
    }
}
```

### 1.3: Flag Registry (Day 4)

Complete implementation of flag registry:

```go
// pkg/flagparser/registry.go

package flagparser

import (
    "fmt"
    "sync"
)

// NewFlagRegistry creates a new flag registry.
func NewFlagRegistry() *FlagRegistry {
    return &FlagRegistry{
        flags:       []*FlagDefinition{},
        byName:      make(map[string]*FlagDefinition),
        byShorthand: make(map[string]*FlagDefinition),
    }
}

// Register adds a flag definition to the registry.
func (r *FlagRegistry) Register(def *FlagDefinition) error {
    if def.Name == "" {
        return fmt.Errorf("flag name cannot be empty")
    }

    if _, exists := r.byName[def.Name]; exists {
        return fmt.Errorf("flag --%s already registered", def.Name)
    }

    if def.Shorthand != "" {
        if _, exists := r.byShorthand[def.Shorthand]; exists {
            return fmt.Errorf("shorthand -%s already registered", def.Shorthand)
        }
    }

    r.flags = append(r.flags, def)
    r.byName[def.Name] = def

    if def.Shorthand != "" {
        r.byShorthand[def.Shorthand] = def
    }

    return nil
}

// Lookup finds a flag by name.
func (r *FlagRegistry) Lookup(name string) *FlagDefinition {
    return r.byName[name]
}

// LookupByShorthand finds a flag by shorthand.
func (r *FlagRegistry) LookupByShorthand(shorthand string) *FlagDefinition {
    return r.byShorthand[shorthand]
}

// All returns all registered flags.
func (r *FlagRegistry) All() []*FlagDefinition {
    return r.flags
}

// Clone creates a deep copy of the registry.
func (r *FlagRegistry) Clone() *FlagRegistry {
    clone := NewFlagRegistry()
    for _, def := range r.flags {
        defCopy := *def
        clone.Register(&defCopy)
    }
    return clone
}
```

**Tests**:
```go
// pkg/flagparser/registry_test.go

func TestFlagRegistry_Register(t *testing.T) { /* ... */ }
func TestFlagRegistry_DuplicateName(t *testing.T) { /* ... */ }
func TestFlagRegistry_DuplicateShorthand(t *testing.T) { /* ... */ }
func TestFlagRegistry_Lookup(t *testing.T) { /* ... */ }
func TestFlagRegistry_Clone(t *testing.T) { /* ... */ }
```

### 1.4: Atmos Flag Definitions (Day 5)

Create standard Atmos flag definitions:

```go
// pkg/flagparser/atmos_flags.go

package flagparser

// NewAtmosRegistry creates a registry with all standard Atmos flags.
func NewAtmosRegistry() *FlagRegistry {
    registry := NewFlagRegistry()

    // Stack flag
    registry.Register(&FlagDefinition{
        Name:      "stack",
        Shorthand: "s",
        Type:      FlagTypeString,
    })

    // Identity flag
    registry.Register(&FlagDefinition{
        Name:      "identity",
        Shorthand: "i",
        Type:      FlagTypeString,
    })

    // Dry run flag
    registry.Register(&FlagDefinition{
        Name:      "dry-run",
        Type:      FlagTypeBool,
        Default:   false,
    })

    // Skip init flag
    registry.Register(&FlagDefinition{
        Name:      "skip-init",
        Type:      FlagTypeBool,
        Default:   false,
    })

    // Upload status flag (optional bool)
    registry.Register(&FlagDefinition{
        Name:                  "upload-status",
        Type:                  FlagTypeBool,
        OptionalValue:         true,
        DefaultValueWhenAlone: true,
        Default:               false,
    })

    // From plan flag
    registry.Register(&FlagDefinition{
        Name:      "from-plan",
        Type:      FlagTypeBool,
        Default:   false,
    })

    // Add all other Atmos flags from commonFlags list...

    return registry
}
```

**Test coverage**: 90%+ for all parsing scenarios.

---

## Phase 2: Baseline Establishment (Week 2)

### Goals

- Capture current Terraform/Helmfile/Packer flag parsing behavior
- Create comprehensive test suite for existing behavior
- Establish regression detection

### 2.1: Extract Current Flag Parsing Behavior

Create baseline test cases from current implementation:

```go
// pkg/flagparser/baseline_test.go

package flagparser

import (
    "testing"
)

// BaselineTestCase represents a test case from current Atmos behavior.
type BaselineTestCase struct {
    Name            string
    Command         string   // e.g., "terraform", "helmfile"
    RawArgs         []string // Original command-line args
    ExpectedStack   string
    ExpectedFlags   map[string]interface{}
    ExpectedToolArgs []string
}

// LoadBaselineTestCases loads test cases from current Atmos behavior.
// These tests represent how Atmos CURRENTLY parses flags (baseline).
func LoadBaselineTestCases() []BaselineTestCase {
    return []BaselineTestCase{
        {
            Name:         "terraform plan with stack",
            Command:      "terraform",
            RawArgs:      []string{"plan", "vpc", "-s", "prod"},
            ExpectedStack: "prod",
            ExpectedFlags: map[string]interface{}{
                "stack": "prod",
            },
            ExpectedToolArgs: []string{},
        },
        {
            Name:         "terraform plan with stack and var",
            Command:      "terraform",
            RawArgs:      []string{"plan", "vpc", "-s", "prod", "-var", "foo=bar"},
            ExpectedStack: "prod",
            ExpectedFlags: map[string]interface{}{
                "stack": "prod",
            },
            ExpectedToolArgs: []string{"-var", "foo=bar"},
        },
        // Add 100+ baseline test cases covering all current usage patterns
    }
}

func TestBaseline_CurrentBehavior(t *testing.T) {
    cases := LoadBaselineTestCases()

    for _, tc := range cases {
        t.Run(tc.Name, func(t *testing.T) {
            // This test documents current behavior
            // Will be used to verify new parser produces same results

            // TODO: Extract actual current behavior from Atmos
            // For now, just document expected behavior
        })
    }
}
```

### 2.2: Integration Test Suite

Create integration tests that call actual Terraform/Helmfile commands:

```go
// tests/flagparser_integration_test.go

func TestTerraformFlagParsing_Baseline(t *testing.T) {
    // Run actual terraform commands and verify flag parsing
    // This establishes baseline for regression testing
}
```

---

## Phase 3: Parser Integration (Week 3)

### 3.1: Update Terraform Command

Replace custom parsing with new parser:

```go
// cmd/terraform.go

func terraformRun(cmd *cobra.Command, args []string) error {
    // Use new parser
    registry := flagparser.NewAtmosRegistry()
    opts := flagparser.ParseOptions{
        EnableImplicitMode: true,
    }

    result, err := flagparser.Parse(args, registry, opts)
    if err != nil {
        return err
    }

    // Extract parsed values
    info := schema.ConfigAndStacksInfo{
        SubCommand:          result.PositionalArgs[0],
        ComponentFromArg:    result.PositionalArgs[1],
        AdditionalArgsAndFlags: result.PassThroughArgs,
    }

    if stack, ok := result.AtmosFlags["stack"].(string); ok {
        info.Stack = stack
    }

    // Continue with existing logic...
}
```

### 3.2: Run Regression Tests

Compare new parser results against baseline:

```bash
# Run baseline tests
go test ./pkg/flagparser -run TestBaseline -v

# Run integration tests
go test ./tests -run TestTerraformFlagParsing -v

# Verify no regressions
diff baseline_results.txt new_results.txt
```

---

## Success Criteria

- [ ] **90% test coverage** for `pkg/flagparser/`
- [ ] **All three concurrent types** supported (Atmos flags, tool flags, positional)
- [ ] **Zero regressions** - all baseline tests pass
- [ ] **Pure functions** - no I/O, no side effects, deterministic
- [ ] **Comprehensive edge cases** - 100+ test scenarios
- [ ] **Performance** - <1ms for typical command parsing
- [ ] **Documentation** - GoDoc for all public APIs

---

## Testing Checklist

### Unit Tests (90%+ coverage)

- [ ] `splitAtDoubleDash()` - all separator positions
- [ ] `parseExplicitMode()` - with/without Atmos flags
- [ ] `parseImplicitMode()` - interleaved flags
- [ ] `tryParseAtmosFlag()` - all flag types
- [ ] `parseEqualsForm()` - `--flag=value` patterns
- [ ] `parseFlagName()` - long/short forms
- [ ] `parseValue()` - all types (string, bool, int, slice)
- [ ] `parseBool()` - all boolean representations
- [ ] `extractAtmosFlags()` - positional args extraction
- [ ] `FlagRegistry` - register, lookup, clone
- [ ] Edge cases - empty args, only positional, only flags

### Integration Tests

- [ ] Terraform plan with all patterns
- [ ] Terraform apply with auto-approve
- [ ] Terraform workspace commands
- [ ] Helmfile sync with flags
- [ ] Packer build with template
- [ ] Custom commands with dynamic flags

### Regression Tests

- [ ] All current Terraform usage patterns
- [ ] All current Helmfile usage patterns
- [ ] All current Packer usage patterns
- [ ] Verify no behavior changes

### Performance Tests

- [ ] Benchmark explicit mode parsing
- [ ] Benchmark implicit mode parsing
- [ ] Benchmark large arg lists (100+ args)
- [ ] Memory allocation profiling

---

## Documentation

### GoDoc Comments

All public APIs must have comprehensive GoDoc:

```go
// Parse parses command-line arguments into structured result.
//
// Parse supports three concurrent types of arguments:
//   1. Atmos-style flags (--stack, -s) - extracted into AtmosFlags
//   2. Tool-style flags (-var, -out) - passed through to PassThroughArgs
//   3. Positional arguments (plan, vpc) - extracted into PositionalArgs
//
// The parser operates in two modes:
//
// Explicit Mode (-- separator present):
//   Everything before -- is parsed for Atmos flags and positional args.
//   Everything after -- is passed through unchanged.
//
//   Example:
//     atmos terraform plan vpc --stack prod -- -var foo=bar
//     AtmosFlags: {"stack": "prod"}
//     PositionalArgs: ["plan", "vpc"]
//     PassThroughArgs: ["-var", "foo=bar"]
//
// Implicit Mode (no -- separator, opts.EnableImplicitMode = true):
//   Known Atmos flags are extracted based on registry.
//   Unknown flags are passed through.
//
//   Example:
//     atmos terraform plan vpc --stack prod -var foo=bar
//     AtmosFlags: {"stack": "prod"}
//     PositionalArgs: ["plan", "vpc"]
//     PassThroughArgs: ["-var", "foo=bar"]
//
// The function is pure - it has no side effects and returns deterministic
// results for the same inputs.
func Parse(args []string, registry *FlagRegistry, opts ParseOptions) (*ParseResult, error)
```

### Usage Examples

Create comprehensive examples:

```go
// Example_explicitMode demonstrates parsing with -- separator.
func Example_explicitMode() {
    registry := NewAtmosRegistry()
    args := []string{"plan", "vpc", "--stack", "prod", "--", "-var", "foo=bar"}
    opts := ParseOptions{EnableImplicitMode: true}

    result, _ := Parse(args, registry, opts)

    fmt.Println(result.AtmosFlags["stack"])
    fmt.Println(result.PositionalArgs)
    fmt.Println(result.PassThroughArgs)

    // Output:
    // prod
    // [plan vpc]
    // [-var foo=bar]
}
```

---

## Next Steps

1. **Review this plan** - Ensure approach is sound
2. **Create `pkg/flagparser/` skeleton** - Set up package structure
3. **Implement types.go** - Data structures first
4. **Write comprehensive tests** - Achieve 90% coverage
5. **Implement parser.go** - Pure parsing functions
6. **Create baseline tests** - Document current behavior
7. **Integrate with Terraform** - Replace custom parsing
8. **Run regression suite** - Verify no behavior changes

This implementation-first, test-heavy approach ensures the parser is rock-solid before touching any existing commands.
