# Experimental Features System

## Overview

This document describes the experimental features system for Atmos, which provides first-class support for marking commands as experimental and configuring how experimental features are handled at runtime.

## Problem Statement

### Current State

Atmos has several features that are still being refined and may change or be removed in future versions. Currently, there is no standardized way to:

1. Mark commands or features as experimental
2. Warn users when they use experimental functionality
3. Allow users to control their tolerance for experimental features
4. Automatically communicate experimental status without manual intervention

### Challenges

1. **No visibility** - Users may not realize they're using experimental features
2. **No control** - Users cannot opt-in or opt-out of experimental features
3. **Inconsistent UX** - No standard way to communicate experimental status
4. **Manual overhead** - Each command must manually implement experimental warnings
5. **CI/CD concerns** - Experimental features may break pipelines unexpectedly

## Solution: Experimental Features System

### Design Principles

1. **Self-registration** - Commands declare their experimental status via interface
2. **Centralized handling** - Warning/error logic in one place (root command)
3. **User control** - Configurable behavior levels
4. **Automatic** - No manual calls needed in command implementations

### Configuration

Users configure experimental feature behavior via `settings.experimental` in `atmos.yaml`:

```yaml
settings:
  experimental: warn  # silence | disable | warn | error
```

### Behavior Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `silence` | No output, feature runs normally | Production systems that knowingly use experimental features |
| `disable` | Error when experimental command invoked | Strict environments that prohibit experimental features |
| `warn` | Show warning, then run normally | Development environments (default) |
| `error` | Show warning, then exit with error | CI/CD that wants to catch experimental usage |

### Environment Variable

```bash
ATMOS_SETTINGS_EXPERIMENTAL=warn
```

## Architecture

### CommandProvider Interface Extension

```go
type CommandProvider interface {
    // Existing methods...
    GetCommand() *cobra.Command
    GetName() string
    GetGroup() string
    GetFlagsBuilder() flags.Builder
    GetPositionalArgsBuilder() *flags.PositionalArgsBuilder
    GetCompatibilityFlags() map[string]compat.CompatibilityFlag
    GetAliases() []CommandAlias

    // New method
    IsExperimental() bool
}
```

### Execution Flow

```
User invokes command
        │
        ▼
┌─────────────────────┐
│  PersistentPreRun   │
│  (cmd/root.go)      │
└─────────────────────┘
        │
        ▼
┌─────────────────────┐
│ Check if command is │
│   experimental      │
│ (IsCommandExperimental)
└─────────────────────┘
        │
        ▼
┌─────────────────────┐
│ Read settings.      │
│ experimental mode   │
└─────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│              Switch on mode             │
├─────────────────────────────────────────┤
│ silence → continue silently             │
│ disable → return error                  │
│ warn    → ui.Experimental() + continue  │
│ error   → ui.Experimental() + return err│
└─────────────────────────────────────────┘
```

### Registry Helper

```go
// IsCommandExperimental returns true if the named command is experimental.
func IsCommandExperimental(name string) bool {
    provider, ok := GetProvider(name)
    if !ok {
        return false
    }
    return provider.IsExperimental()
}
```

## Implementation

### Schema Changes

Add to `Settings` struct in `pkg/schema/schema.go`:

```go
type Settings struct {
    // ... existing fields ...

    // Experimental controls how experimental features are handled.
    // Values: "silence" (no output), "disable" (disabled), "warn" (default), "error" (exit).
    Experimental string `yaml:"experimental" json:"experimental" mapstructure:"experimental"`
}
```

### Configuration Default

In `pkg/config/load.go`:

```go
v.SetDefault("settings.experimental", "warn")
```

### Command Implementation

Commands implement `IsExperimental()`:

```go
// Non-experimental command (most commands)
func (p *TerraformCommandProvider) IsExperimental() bool {
    return false
}

// Experimental command
func (p *DevcontainerCommandProvider) IsExperimental() bool {
    return true
}
```

### Experimental Warning Output

When `warn` or `error` mode is active, users see:

```
🧪 devcontainer is an experimental feature. Learn more atmos.tools/experimental
```

## Experimental Commands

The following commands are marked as experimental:

- `atmos devcontainer` - Devcontainer management
- Additional commands as identified

## Future Considerations

### Subcommand-Level Experimental Flags

Currently, the experimental flag is at the command level. Future versions may support subcommand-level flags:

```go
type SubcommandProvider interface {
    IsExperimental() bool
}
```

### Feature-Level Experimental Support

Non-command features (YAML functions, config options) may need experimental flags:

```yaml
settings:
  experimental_features:
    yaml_functions:
      some_new_function: true
```

### Graduation Path

Experimental features should have a clear path to stability:

1. **Experimental** - Initial release, may change
2. **Beta** - API stabilizing, feedback welcome
3. **Stable** - Production ready, backward compatible

## Testing

1. **silence mode** - Verify no output, command executes
2. **disable mode** - Verify error returned, command does not execute
3. **warn mode** - Verify warning shown, command executes
4. **error mode** - Verify warning shown, error returned
5. **Non-experimental commands** - Verify unaffected by settings

## Related Documents

- [Command Registry Pattern](command-registry-pattern.md) - How commands register with Atmos
- [Error Handling Strategy](error-handling-strategy.md) - How errors are formatted and returned
