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
  experimental: warn-daily  # silence | disable | warn | warn-daily (default) | error
```

### Behavior Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `silence` | No output, feature runs normally | Production systems that knowingly use experimental features |
| `disable` | Error when experimental command invoked | Strict environments that prohibit experimental features |
| `warn` | Show warning on each top-level invocation, then run normally | Explicit override for more frequent notices |
| `warn-daily` | Show each feature's warning once every 24 hours using the shared local cache, then run normally | Default for unpinned projects and editions on or after `2026-09-14` |
| `error` | Show warning, then exit with error | CI/CD that wants to catch experimental usage |

### Environment Variable

To explicitly select warnings on each top-level invocation:

```bash
ATMOS_EXPERIMENTAL=warn
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
│ silence    → continue silently          │
│ disable    → return error               │
│ warn       → warn at top level; continue│
│ warn-daily → warn if due; continue      │
│ error      → warn and return error      │
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
    // Values: "silence", "disable", "warn", "warn-daily" (default), "error".
    Experimental string `yaml:"experimental" json:"experimental" mapstructure:"experimental"`
}
```

### Configuration Default

In `pkg/config/load.go`:

```go
v.SetDefault("settings.experimental", "warn-daily")
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

When `warn` or `error` mode emits a warning, or a feature's warning is due in `warn-daily` mode, users see:

```
🧪 devcontainer is an experimental feature. Learn more atmos.tools/experimental
```

## Experimental Commands

The following commands are marked as experimental:

| Command | Description |
|---------|-------------|
| `atmos devcontainer` | Development container lifecycle management |
| `atmos toolchain` | Tool version management and installation |
| `atmos terraform backend` | Terraform state backend management |
| `atmos terraform workdir` | Component working directory management |
| `atmos list affected` | Identify changes for targeted CI/CD |
| `atmos kubernetes` | Manage Kubernetes-native components |

## Implementation Details

### Top-Level Command Experimental Status

Top-level commands declare experimental status via the `CommandProvider` interface:

```go
func (p *DevcontainerCommandProvider) IsExperimental() bool {
    return true
}
```

### Subcommand-Level Experimental Status

Subcommands use Cobra annotations to declare experimental status:

```go
func init() {
    // Mark this subcommand as experimental.
    workdirCmd.Annotations = map[string]string{"experimental": "true"}
}
```

The `findExperimentalParent()` function in `cmd/root.go` walks up the command tree to check both registry-based and annotation-based experimental status.

## Future Considerations

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
3. **warn mode** - Verify warning shown on each top-level invocation, command executes
4. **warn-daily mode** - Verify each feature warns independently once every 24 hours across invocations sharing the local cache, with another warning eligible when its interval expires
5. **error mode** - Verify warning shown, error returned
6. **Non-experimental commands** - Verify unaffected by settings

## Related Documents

- [Command Registry Pattern](command-registry-pattern.md) - How commands register with Atmos
- [Error Handling Strategy](error-handling-strategy.md) - How errors are formatted and returned

## Migration: Daily Warnings (2026-09-14)

The default for `settings.experimental` changes from `warn` to `warn-daily`, which
shows each feature's warning once every 24 hours using the shared local cache.
The edition journal preserves the `warn` default for pins before `2026-09-14`;
explicit configuration and environment overrides continue to take precedence.

The stored value `warn` also has a narrower behavior change: command and setting
notices previously repeated in child Atmos processes, but are now suppressed after
the parent handles startup. CI hooks retain their existing `warn` behavior. This
child-process suppression is not edition-gated because `KindBehavior` resolution
is not implemented; it is recorded in the editions PRD's behavior-gating roadmap.
The `error` and `disable` modes remain enforced in all invocations.
