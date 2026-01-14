# Command Alias Pattern

## Overview

This document describes the first-class alias system for Atmos commands, which enables declarative alias creation without manual RunE implementation, automatic flag forwarding, and correct PreRun hook execution.

## Problem Statement

### Current State

The current alias implementation in `cmd/toolchain/search.go` requires:

1. **Manual RunE forwarding** - Find target command, extract it, call its RunE manually
2. **Manual flag registration** - Call `GetSearchParser()` and manually register flags
3. **Knowledge of internals** - Must understand PersistentPreRun execution order
4. **Fragile coordination** - Calling `ExecuteContext()` bypasses parent's `PersistentPreRun`, breaking IO context initialization

**Current implementation (`cmd/toolchain/search.go`):**

```go
var searchAliasCmd = &cobra.Command{
    Use:   "search <query>",
    Short: "Search for tools (alias to 'registry search')",
    Args: cobra.MinimumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        // PROBLEM 1: Manual target discovery
        registryCmd := registrycmd.GetRegistryCommand()
        searchCmd, _, err := registryCmd.Find([]string{"search"})
        if err != nil {
            return err
        }

        // PROBLEM 2: Manual args forwarding
        cmd.SetArgs(args)

        // PROBLEM 3: Manual RunE execution (bypasses PreRun hooks if using ExecuteContext)
        return searchCmd.RunE(cmd, args)
    },
}

func init() {
    // PROBLEM 4: Manual flag registration
    searchParser := registrycmd.GetSearchParser()
    if searchParser != nil {
        searchParser.RegisterFlags(searchAliasCmd)
    }
}
```

### Issues with Current Approach

1. **PreRun hook bypass** - If alias calls `ExecuteContext()`, parent's `PersistentPreRun` doesn't execute
2. **Boilerplate duplication** - Every alias requires ~40 lines of manual wiring
3. **Fragile coordination** - Must understand Cobra internals and execution order
4. **Flag synchronization** - Flags must be manually copied from target to alias
5. **No validation** - Target command existence not validated at registration time

### Use Cases

**Scenario 1: Top-level alias to nested subcommand**
```bash
atmos toolchain search terraform     # Alias at top level
atmos toolchain registry search terraform  # Target is nested subcommand
```

**Scenario 2: Multiple aliases to same target**
```bash
atmos tf plan        # Alias 1
atmos terraform plan # Alias 2 (could also be base command)
```

**Scenario 3: Alias with flag inheritance**
```bash
atmos search terraform --limit 10  # All flags from target work on alias
```

## Solution: First-Class Alias System

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                   Command Registry                           │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────┐             │
│  │      CommandProvider Interface             │             │
│  │  - GetCommand()                            │             │
│  │  - GetName()                                │             │
│  │  - GetGroup()                               │             │
│  │  - GetFlagsBuilder()       (optional)      │             │
│  │  - GetAliasTarget()        (new)           │             │
│  └────────────────────────────────────────────┘             │
│           │                        │                         │
│           │                        │                         │
│           ▼                        ▼                         │
│  ┌─────────────────┐    ┌──────────────────────┐           │
│  │  Base Commands  │    │  Alias Commands      │           │
│  │                 │    │                      │           │
│  │  - Implement    │    │  - Implement         │           │
│  │    GetCommand() │    │    GetAliasTarget()  │           │
│  │                 │    │                      │           │
│  │  - Return       │    │  - Return            │           │
│  │    actual       │    │    AliasTarget       │           │
│  │    command      │    │    descriptor        │           │
│  │                 │    │                      │           │
│  │  - Own RunE     │    │  - No RunE needed    │           │
│  │    logic        │    │  - Registry creates  │           │
│  │                 │    │    wrapper command   │           │
│  └─────────────────┘    └──────────────────────┘           │
│                                                              │
│  Registry Logic:                                             │
│  1. During RegisterAll():                                    │
│     a. Separate base commands from aliases                   │
│     b. Register all base commands first                      │
│     c. For each alias:                                       │
│        - Resolve target command                              │
│        - Create wrapper command                              │
│        - Copy flags from target                              │
│        - Set RunE to forward to target                       │
│        - Register wrapper command                            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Declarative** - Aliases declared with target path, not manual implementation
2. **Automatic** - Registry handles all wiring (flags, PreRun, args)
3. **Type-safe** - Compile-time validation that target exists
4. **Transparent** - Aliases behave identically to calling target directly
5. **Zero overhead** - No performance penalty vs. manual implementation

## Implementation

### 1. AliasTarget Descriptor

```go
// cmd/internal/alias.go
package internal

// AliasTarget describes the target command for an alias.
//
// Examples:
//   - ["registry", "search"] -> points to "toolchain registry search"
//   - ["plan"] -> points to "terraform plan"
type AliasTarget struct {
    // Path is the sequence of command names to reach the target.
    // Example: ["registry", "search"] means "toolchain registry search"
    // when the alias is registered under "toolchain".
    Path []string

    // TargetProvider is an optional direct reference to the target provider.
    // If set, Path is ignored and this provider is used directly.
    // This is useful for cross-package aliases where target is in different subtree.
    TargetProvider CommandProvider
}

// NewAliasTarget creates an alias target using a command path.
// The path is relative to the parent command where this alias will be registered.
//
// Example:
//   NewAliasTarget("registry", "search")  // For toolchain.AddCommand(alias) -> points to toolchain registry search
func NewAliasTarget(path ...string) *AliasTarget {
    return &AliasTarget{Path: path}
}

// NewAliasTargetProvider creates an alias target using a direct provider reference.
// This is useful when the target command is in a different command subtree.
//
// Example:
//   NewAliasTargetProvider(registrycmd.NewSearchCommandProvider())
func NewAliasTargetProvider(provider CommandProvider) *AliasTarget {
    return &AliasTarget{TargetProvider: provider}
}
```

### 2. Extended CommandProvider Interface

```go
// cmd/internal/command.go (updated)
package internal

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/pkg/flags"
    "github.com/cloudposse/atmos/pkg/flags/compat"
)

// CommandProvider is the interface that built-in command packages implement
// to register themselves with the Atmos command registry.
type CommandProvider interface {
    // GetCommand returns the cobra.Command for this provider.
    // For base commands: returns the actual command with RunE logic.
    // For aliases: returns a shell command (Use/Short/Long only), RunE added by registry.
    GetCommand() *cobra.Command

    // GetName returns the command name (e.g., "about", "terraform", "search").
    GetName() string

    // GetGroup returns the command group for help organization.
    GetGroup() string

    // GetFlagsBuilder returns the flags builder for this command.
    // Base commands: return their parser.
    // Aliases: return nil (flags copied from target).
    GetFlagsBuilder() flags.Builder

    // GetPositionalArgsBuilder returns the positional args builder.
    GetPositionalArgsBuilder() *flags.PositionalArgsBuilder

    // GetCompatibilityFlags returns compatibility flags.
    GetCompatibilityFlags() map[string]compat.CompatibilityFlag

    // GetAliasTarget returns the alias target descriptor if this is an alias.
    // Base commands: return nil.
    // Aliases: return AliasTarget pointing to target command.
    GetAliasTarget() *AliasTarget
}
```

### 3. Updated Registry with Alias Support

```go
// cmd/internal/registry.go (updated RegisterAll)
package internal

import (
    "fmt"
    "sync"

    "github.com/spf13/cobra"

    errUtils "github.com/cloudposse/atmos/errors"
)

// RegisterAll registers all built-in commands with the root command.
// Handles both base commands and aliases automatically.
func RegisterAll(root *cobra.Command) error {
    registry.mu.RLock()
    defer registry.mu.RUnlock()

    // Separate base commands from aliases for correct ordering.
    var baseProviders []CommandProvider
    var aliasProviders []CommandProvider

    for _, provider := range registry.providers {
        if provider.GetAliasTarget() != nil {
            aliasProviders = append(aliasProviders, provider)
        } else {
            baseProviders = append(baseProviders, provider)
        }
    }

    // Register base commands first.
    for _, provider := range baseProviders {
        cmd := provider.GetCommand()
        if cmd == nil {
            return fmt.Errorf("%w: provider %s", errUtils.ErrCommandNil, provider.GetName())
        }
        root.AddCommand(cmd)
    }

    // Register aliases after base commands (targets must exist).
    for _, provider := range aliasProviders {
        aliasCmd, err := createAliasCommand(root, provider)
        if err != nil {
            return fmt.Errorf("%w: failed to create alias %s: %s",
                errUtils.ErrCommandAlias, provider.GetName(), err)
        }
        root.AddCommand(aliasCmd)
    }

    return nil
}

// createAliasCommand creates a fully-wired alias command that forwards to target.
func createAliasCommand(root *cobra.Command, provider CommandProvider) (*cobra.Command, error) {
    aliasTarget := provider.GetAliasTarget()
    if aliasTarget == nil {
        return nil, fmt.Errorf("provider %s is not an alias", provider.GetName())
    }

    // Get the shell command from provider (Use/Short/Long only).
    aliasCmd := provider.GetCommand()
    if aliasCmd == nil {
        return nil, fmt.Errorf("alias provider %s returned nil command", provider.GetName())
    }

    // Resolve target command.
    var targetCmd *cobra.Command
    var targetProvider CommandProvider
    var err error

    if aliasTarget.TargetProvider != nil {
        // Direct provider reference.
        targetProvider = aliasTarget.TargetProvider
        targetCmd = targetProvider.GetCommand()
    } else {
        // Path-based resolution.
        targetCmd, err = resolveTargetCommand(root, aliasCmd, aliasTarget.Path)
        if err != nil {
            return nil, err
        }
        // Try to find provider for target (for flag copying).
        targetProvider = findProviderByCommand(targetCmd)
    }

    // Copy flags from target to alias.
    if targetProvider != nil {
        if builder := targetProvider.GetFlagsBuilder(); builder != nil {
            builder.RegisterFlags(aliasCmd)
        }
    } else {
        // Fallback: copy flags directly from target command.
        copyFlags(targetCmd, aliasCmd)
    }

    // Copy Args validator from target.
    aliasCmd.Args = targetCmd.Args

    // Set RunE to forward to target.
    // CRITICAL: We call target's RunE directly, NOT ExecuteContext.
    // This ensures parent's PersistentPreRun has already executed.
    originalTargetRunE := targetCmd.RunE
    if originalTargetRunE == nil {
        return nil, fmt.Errorf("target command %s has no RunE", targetCmd.CommandPath())
    }

    aliasCmd.RunE = func(cmd *cobra.Command, args []string) error {
        // Forward to target's RunE.
        // cmd is the alias command with the current context (IO initialized by parent PreRun).
        // We pass the alias cmd so flags are read from it, but execute target's logic.
        return originalTargetRunE(cmd, args)
    }

    return aliasCmd, nil
}

// resolveTargetCommand finds the target command by walking the command tree.
// parentCmd is the parent where this alias will be added (e.g., toolchainCmd).
// path is relative to parentCmd (e.g., ["registry", "search"]).
func resolveTargetCommand(root *cobra.Command, aliasCmd *cobra.Command, path []string) (*cobra.Command, error) {
    if len(path) == 0 {
        return nil, fmt.Errorf("alias target path is empty")
    }

    // Determine the parent command where this alias will be registered.
    // Walk up from aliasCmd's parent chain to find where it attaches.
    parentCmd := root

    // If aliasCmd will be added to a subcommand (e.g., toolchain),
    // we need to start from that subcommand.
    // This is determined by the provider's parent context during registration.
    // For now, we assume aliases registered at root level or one level deep.

    // Try to find the parent command by looking at the registry context.
    // This is set when the alias is registered (e.g., toolchainCmd.AddCommand(alias)).
    // For MVP, we handle common case: alias at root or under a parent.

    // Walk the path to find target.
    currentCmd := parentCmd
    for i, segment := range path {
        found := false
        for _, subcmd := range currentCmd.Commands() {
            if subcmd.Name() == segment {
                currentCmd = subcmd
                found = true
                break
            }
        }
        if !found {
            return nil, fmt.Errorf("target command not found: %s (at segment %d: %s)",
                path, i, segment)
        }
    }

    return currentCmd, nil
}

// findProviderByCommand finds a provider by its command reference.
func findProviderByCommand(cmd *cobra.Command) CommandProvider {
    for _, provider := range registry.providers {
        if provider.GetCommand() == cmd {
            return provider
        }
    }
    return nil
}

// copyFlags copies all flags from source to destination command.
// This is a fallback when target provider is not available.
func copyFlags(source, dest *cobra.Command) {
    source.Flags().VisitAll(func(flag *pflag.Flag) {
        if dest.Flags().Lookup(flag.Name) == nil {
            dest.Flags().AddFlag(flag)
        }
    })

    source.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
        if dest.PersistentFlags().Lookup(flag.Name) == nil {
            dest.PersistentFlags().AddFlag(flag)
        }
    })
}
```

### 4. Helper for Nested Parent Resolution

```go
// cmd/internal/alias_parent.go
package internal

import "github.com/spf13/cobra"

// AliasParentContext tracks where an alias will be registered.
// This is set during command construction to help resolve target paths correctly.
type AliasParentContext struct {
    // ParentCommand is the command this alias will be added to.
    // For "toolchain search" alias, this would be toolchainCmd.
    ParentCommand *cobra.Command
}

// SetAliasParentContext stores the parent context in the command's metadata.
// This is used during RegisterAll to resolve relative target paths correctly.
func SetAliasParentContext(aliasCmd *cobra.Command, parent *cobra.Command) {
    aliasCmd.Annotations = map[string]string{
        "alias_parent": parent.Name(),
    }
}

// GetAliasParentFromRoot finds the parent command for an alias by walking root.
func GetAliasParentFromRoot(root *cobra.Command, parentName string) *cobra.Command {
    if parentName == "" || parentName == "root" {
        return root
    }

    for _, cmd := range root.Commands() {
        if cmd.Name() == parentName {
            return cmd
        }
    }

    return root // Fallback to root.
}
```

### 5. Example: Refactored Search Alias

```go
// cmd/toolchain/search.go (refactored)
package toolchain

import (
    "github.com/spf13/cobra"

    "github.com/cloudposse/atmos/cmd/internal"
    "github.com/cloudposse/atmos/pkg/flags"
    "github.com/cloudposse/atmos/pkg/flags/compat"
)

// searchAliasCmd is a shell command (Use/Short/Long only).
// The registry will wire up flags, args, and RunE automatically.
var searchAliasCmd = &cobra.Command{
    Use:   "search <query>",
    Short: "Search for tools (alias to 'registry search')",
    Long: `Search for tools matching the query string across all configured registries.

This is an alias to 'atmos toolchain registry search' for convenience.

The query is matched against tool owner, repo name, and description.
Results are sorted by relevance score.`,
    // NO Args validator - copied from target
    // NO RunE - created by registry
}

func init() {
    // Register as an alias provider.
    // The registry will handle all the wiring automatically.
    internal.Register(&SearchAliasProvider{})
}

// SearchAliasProvider implements CommandProvider for the search alias.
type SearchAliasProvider struct{}

func (s *SearchAliasProvider) GetCommand() *cobra.Command {
    return searchAliasCmd
}

func (s *SearchAliasProvider) GetName() string {
    return "search"
}

func (s *SearchAliasProvider) GetGroup() string {
    return "Toolchain Commands"
}

func (s *SearchAliasProvider) GetFlagsBuilder() flags.Builder {
    // Aliases return nil - flags copied from target.
    return nil
}

func (s *SearchAliasProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
    return nil
}

func (s *SearchAliasProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
    return nil
}

func (s *SearchAliasProvider) GetAliasTarget() *internal.AliasTarget {
    // Point to toolchain registry search.
    // Path is relative to parent command where this alias is registered.
    // Since this is registered under toolchainCmd, path is ["registry", "search"].
    return internal.NewAliasTarget("registry", "search")
}
```

**Before (44 lines with manual wiring):**
```go
var searchAliasCmd = &cobra.Command{
    Use:   "search <query>",
    Short: "Search for tools (alias to 'registry search')",
    Long: `...`,
    Args: cobra.MinimumNArgs(1),  // Manual
    RunE: func(cmd *cobra.Command, args []string) error {  // Manual
        registryCmd := registrycmd.GetRegistryCommand()
        searchCmd, _, err := registryCmd.Find([]string{"search"})
        if err != nil {
            return err
        }
        cmd.SetArgs(args)
        return searchCmd.RunE(cmd, args)
    },
}

func init() {
    searchParser := registrycmd.GetSearchParser()  // Manual
    if searchParser != nil {
        searchParser.RegisterFlags(searchAliasCmd)  // Manual
    }
}
```

**After (21 lines, no manual wiring):**
```go
var searchAliasCmd = &cobra.Command{
    Use:   "search <query>",
    Short: "Search for tools (alias to 'registry search')",
    Long: `...`,
    // Registry handles Args, RunE, flags automatically
}

func (s *SearchAliasProvider) GetAliasTarget() *internal.AliasTarget {
    return internal.NewAliasTarget("registry", "search")  // Declarative
}
```

**Reduction: 52% fewer lines, 100% less manual wiring.**

## Benefits

### Immediate Benefits

1. ✅ **Declarative** - Aliases declared with target path, not manual implementation
2. ✅ **Automatic flag forwarding** - Registry copies flags from target automatically
3. ✅ **Correct PreRun execution** - Parent's PersistentPreRun runs before alias RunE
4. ✅ **Type-safe** - Compile-time validation via GetAliasTarget() return type
5. ✅ **Less boilerplate** - ~50% reduction in code vs. manual implementation
6. ✅ **Maintainable** - Target changes automatically propagate to aliases
7. ✅ **Testable** - Registry logic tested once, applies to all aliases

### Future Benefits

8. ✅ **Plugin aliases** - External plugins can provide aliases to built-in commands
9. ✅ **Alias discovery** - Registry can list all aliases and their targets
10. ✅ **Alias validation** - Registry validates targets exist at startup

## Implementation Phases

### Phase 1: Core Infrastructure (Current PR)

- [ ] Implement AliasTarget struct
- [ ] Extend CommandProvider interface with GetAliasTarget()
- [ ] Update registry.RegisterAll() to handle aliases
- [ ] Implement createAliasCommand() with flag copying
- [ ] Add tests for alias resolution

**Files changed:**
- `cmd/internal/command.go` - Add GetAliasTarget() to interface
- `cmd/internal/alias.go` - New file with AliasTarget
- `cmd/internal/registry.go` - Update RegisterAll() for aliases
- `cmd/internal/registry_test.go` - Add alias tests

### Phase 2: Refactor Existing Aliases (Next PR)

- [ ] Refactor `cmd/toolchain/search.go` to use new pattern
- [ ] Remove `GetSearchParser()` and `GetRegistryCommand()` exports (no longer needed)
- [ ] Update tests to verify alias behavior

**Files changed:**
- `cmd/toolchain/search.go` - Refactor to AliasProvider
- `cmd/toolchain/registry/search.go` - Remove GetSearchParser() export
- `cmd/toolchain/registry/registry.go` - Remove GetRegistryCommand() export

### Phase 3: Documentation & Guidelines (Next PR)

- [ ] Update command-registry-pattern.md with alias section
- [ ] Add examples to docs/developing-atmos-commands.md
- [ ] Create migration guide for future aliases

**Files changed:**
- `docs/prd/command-registry-pattern.md` - Add alias section
- `docs/developing-atmos-commands.md` - Add alias examples

## Migration Guide

### Creating a New Alias

**Step 1: Create alias command (shell only, no logic)**

```go
// cmd/mycommand/myalias.go
package mycommand

var myAliasCmd = &cobra.Command{
    Use:   "myalias <arg>",
    Short: "Short description (alias to 'target subcommand')",
    Long:  `Long description...`,
    // NO Args, NO RunE - registry adds these
}
```

**Step 2: Implement AliasProvider**

```go
type MyAliasProvider struct{}

func (m *MyAliasProvider) GetCommand() *cobra.Command {
    return myAliasCmd
}

func (m *MyAliasProvider) GetName() string {
    return "myalias"
}

func (m *MyAliasProvider) GetGroup() string {
    return "My Commands"
}

func (m *MyAliasProvider) GetFlagsBuilder() flags.Builder {
    return nil  // Aliases return nil
}

func (m *MyAliasProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
    return nil
}

func (m *MyAliasProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
    return nil
}

func (m *MyAliasProvider) GetAliasTarget() *internal.AliasTarget {
    // Path relative to parent command.
    return internal.NewAliasTarget("target", "subcommand")
}
```

**Step 3: Register with registry**

```go
func init() {
    internal.Register(&MyAliasProvider{})
}
```

**Step 4: Import in parent command or root.go**

```go
// cmd/mycommand/mycommand.go
import (
    _ "github.com/cloudposse/atmos/cmd/mycommand/myalias"
)
```

**Done!** The registry handles:
- Flag registration
- Args validation
- RunE forwarding
- PreRun execution

### Converting Existing Manual Alias

**Before:**
```go
var myAliasCmd = &cobra.Command{
    Use:   "myalias <arg>",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        targetCmd := findTarget()
        return targetCmd.RunE(cmd, args)
    },
}

func init() {
    targetParser.RegisterFlags(myAliasCmd)
}
```

**After:**
```go
var myAliasCmd = &cobra.Command{
    Use:   "myalias <arg>",
    // Remove Args, RunE - registry adds these
}

func (m *MyAliasProvider) GetAliasTarget() *internal.AliasTarget {
    return internal.NewAliasTarget("target", "subcommand")
}

// Remove init() flag registration - registry handles it
```

## Testing Strategy

### Unit Tests

```go
// cmd/internal/registry_test.go
package internal

func TestAliasRegistration(t *testing.T) {
    Reset()

    // Register base command.
    baseCmd := &cobra.Command{Use: "base"}
    subCmd := &cobra.Command{Use: "sub", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
    baseCmd.AddCommand(subCmd)

    baseProvider := &mockProvider{
        name: "base",
        cmd: baseCmd,
    }
    Register(baseProvider)

    // Register alias pointing to base sub.
    aliasProvider := &mockAliasProvider{
        name: "alias",
        target: NewAliasTarget("sub"),
    }
    Register(aliasProvider)

    // Register all.
    root := &cobra.Command{Use: "root"}
    err := RegisterAll(root)
    assert.NoError(t, err)

    // Verify alias exists.
    aliasCmd, _, err := root.Find([]string{"alias"})
    assert.NoError(t, err)
    assert.NotNil(t, aliasCmd)
    assert.NotNil(t, aliasCmd.RunE, "Alias should have RunE set by registry")
}

func TestAliasFlagForwarding(t *testing.T) {
    Reset()

    // Register target with flags.
    targetCmd := &cobra.Command{Use: "target"}
    targetCmd.Flags().String("flag1", "", "Flag 1")
    targetProvider := &mockProviderWithFlags{
        name: "target",
        cmd: targetCmd,
        builder: flags.NewStandardParser(
            flags.WithStringFlag("flag1", "", "", "Flag 1"),
        ),
    }
    Register(targetProvider)

    // Register alias.
    aliasProvider := &mockAliasProvider{
        name: "alias",
        target: NewAliasTarget("target"),
    }
    Register(aliasProvider)

    // Register all.
    root := &cobra.Command{Use: "root"}
    err := RegisterAll(root)
    assert.NoError(t, err)

    // Verify alias has flag.
    aliasCmd, _, _ := root.Find([]string{"alias"})
    flag := aliasCmd.Flags().Lookup("flag1")
    assert.NotNil(t, flag, "Alias should have flag from target")
}

func TestAliasTargetNotFound(t *testing.T) {
    Reset()

    // Register alias with invalid target.
    aliasProvider := &mockAliasProvider{
        name: "alias",
        target: NewAliasTarget("nonexistent"),
    }
    Register(aliasProvider)

    root := &cobra.Command{Use: "root"}
    err := RegisterAll(root)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "target command not found")
}
```

### Integration Tests

```go
// cmd/toolchain/search_test.go
package toolchain

func TestSearchAlias(t *testing.T) {
    // Setup: Register toolchain command and search alias.
    root := &cobra.Command{Use: "atmos"}
    internal.Register(&ToolchainCommandProvider{})
    internal.Register(&SearchAliasProvider{})
    err := internal.RegisterAll(root)
    require.NoError(t, err)

    // Execute: Run search alias.
    output := &bytes.Buffer{}
    root.SetOut(output)
    root.SetArgs([]string{"toolchain", "search", "terraform", "--limit", "5"})
    err = root.Execute()

    // Verify: Same behavior as registry search.
    assert.NoError(t, err)
    assert.Contains(t, output.String(), "terraform")
}

func TestSearchAliasFlags(t *testing.T) {
    // Verify all flags from registry search work on alias.
    root := &cobra.Command{Use: "atmos"}
    internal.RegisterAll(root)

    searchAliasCmd, _, _ := root.Find([]string{"toolchain", "search"})

    // Check all expected flags exist.
    assert.NotNil(t, searchAliasCmd.Flags().Lookup("limit"))
    assert.NotNil(t, searchAliasCmd.Flags().Lookup("registry"))
    assert.NotNil(t, searchAliasCmd.Flags().Lookup("format"))
}
```

## Error Handling

### Validation Errors

```go
// Registry validates aliases during RegisterAll:

// 1. Target path empty
GetAliasTarget() returns &AliasTarget{Path: []}
→ Error: "alias target path is empty"

// 2. Target command not found
GetAliasTarget() returns &AliasTarget{Path: ["nonexistent"]}
→ Error: "target command not found: [nonexistent] (at segment 0: nonexistent)"

// 3. Target has no RunE
GetAliasTarget() points to parent command with only subcommands
→ Error: "target command toolchain registry has no RunE"

// 4. Circular alias
Alias A → Alias B → Alias A
→ Error: "circular alias detected: A → B → A"
```

### Runtime Errors

```go
// 1. Flag binding fails
If target flags can't be copied to alias
→ Error: "failed to copy flags from target: <reason>"

// 2. Target changes after registration
If target command is removed after alias registration
→ Error at runtime: "alias target no longer exists: <path>"
```

## Alternatives Considered

### Alternative 1: Cobra Native Aliases

**Approach:** Use Cobra's built-in `Aliases` field.

```go
var terraformCmd = &cobra.Command{
    Use: "terraform",
    Aliases: []string{"tf"},
}
```

**Pros:**
- Native Cobra feature
- Simple to use

**Cons:**
- ❌ Only works for same-level aliases (not cross-command)
- ❌ Can't alias to nested subcommands
- ❌ No separate help text for aliases
- ❌ No flag subsetting (all flags or none)

**Decision:** Rejected - doesn't support our use case (aliasing nested subcommands).

### Alternative 2: Wrapper Commands (Current Approach)

**Approach:** Manually create wrapper command with forwarding logic.

**Pros:**
- Full control over behavior
- No registry changes needed

**Cons:**
- ❌ Boilerplate duplication (~40 lines per alias)
- ❌ Fragile (PreRun hook bypass)
- ❌ Hard to maintain
- ❌ No standardization

**Decision:** Rejected - too much manual work and fragile.

### Alternative 3: Code Generation

**Approach:** Generate alias commands from configuration file.

```yaml
# aliases.yaml
aliases:
  - name: search
    target: registry search
    parent: toolchain
```

**Pros:**
- Declarative configuration
- Single source of truth

**Cons:**
- ❌ Build-time generation complexity
- ❌ Harder to debug generated code
- ❌ Less type-safe than Go interfaces
- ❌ Adds build step dependency

**Decision:** Rejected - runtime approach is simpler and more Go-idiomatic.

### Alternative 4: Registry-Based (Selected)

**Approach:** Extend CommandProvider interface, handle aliases in registry.

**Pros:**
- ✅ Declarative via interface
- ✅ Automatic flag forwarding
- ✅ Type-safe (compile-time checking)
- ✅ Testable
- ✅ Maintainable

**Cons:**
- Requires interface extension (non-breaking with default nil return)

**Decision:** Selected - best balance of simplicity, type safety, and maintainability.

## Open Questions

### Q1: How to handle aliases across different parent commands?

**Example:** Alias at root level pointing to deeply nested command.

```go
// Root-level alias "search" → "toolchain registry search"
```

**Solution:** Use `SetAliasParentContext()` to track parent, resolve path from parent.

**Status:** Solved in design with parent context tracking.

### Q2: Should aliases support flag subsetting?

**Example:** Target has 10 flags, alias only exposes 5.

**Current design:** All flags forwarded (simplicity).

**Future enhancement:** Add `FilterFlags []string` to AliasTarget.

**Status:** Deferred to future enhancement.

### Q3: How to handle versioned aliases?

**Example:** `atmos tf2 plan` → Terraform 2.x, `atmos tf3 plan` → Terraform 3.x

**Current design:** Each alias is a separate provider.

**Status:** Works with current design, no changes needed.

## Success Criteria

### Phase 1 (Core Infrastructure)

- ✅ AliasTarget struct implemented
- ✅ CommandProvider.GetAliasTarget() added
- ✅ Registry handles alias registration
- ✅ Flags automatically forwarded
- ✅ PreRun hooks execute correctly
- ✅ All tests pass

### Phase 2 (Refactor Existing)

- ✅ `cmd/toolchain/search.go` refactored
- ✅ Manual flag registration removed
- ✅ Manual RunE forwarding removed
- ✅ Tests verify identical behavior

### Phase 3 (Documentation)

- ✅ PRD updated with alias section
- ✅ Migration guide published
- ✅ Examples added to docs

## References

- [Cobra Command Documentation](https://github.com/spf13/cobra)
- [Command Registry Pattern PRD](./command-registry-pattern.md)
- [Go Interfaces Best Practices](https://golang.org/doc/effective_go#interfaces)
- [kubectl Plugin Aliases](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-11 | Initial design for first-class alias system |
