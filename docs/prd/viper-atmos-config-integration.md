# PRD: Viper and AtmosConfig Integration

## Problem Statement

Currently, we manually implement flag precedence in command implementations (like `validate editorconfig`) instead of leveraging Viper's built-in precedence system. This creates unnecessary complexity, code duplication, and potential for precedence bugs.

### Current Implementation

```go
// cmd/validate_editorconfig.go - Manual precedence implementation
cliConfig.IgnoreDefaults = opts.IgnoreDefaults
if !opts.IgnoreDefaultsProvided {
    cliConfig.IgnoreDefaults = atmosConfig.Validate.EditorConfig.IgnoreDefaults
}
cliConfig.DryRun = opts.DryRun
if !opts.DryRunProvided {
    cliConfig.DryRun = atmosConfig.Validate.EditorConfig.DryRun
}
// ... repeated for every boolean flag
```

This is **reimplementing what Viper already does** for us!

### Root Cause

The atmos.yaml configuration is loaded into `atmosConfig` struct separately from Viper's configuration system:

```go
// cmd/root.go
var atmosConfig schema.AtmosConfiguration  // Global

RootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
    // Load atmos.yaml → atmosConfig
    atmosConfig, err = cfg.InitCliConfig(...)

    // But Viper doesn't know about atmosConfig values!
    // So commands must manually check atmosConfig.Validate.EditorConfig.*
}
```

**The disconnect:**
- Viper knows about: CLI flags, env vars, flag defaults
- Viper does NOT know about: `atmosConfig.Validate.EditorConfig.*` from atmos.yaml
- Result: We manually implement precedence instead of using Viper's

## Architectural Issue

We have TWO separate configuration systems:

1. **Viper** - Handles CLI flags, env vars, precedence
2. **atmosConfig** - Loaded from atmos.yaml, but disconnected from Viper

Commands that need atmos.yaml values must manually implement precedence:

```go
// WRONG: Manual precedence checking
if !opts.DryRunProvided {
    cliConfig.DryRun = atmosConfig.Validate.EditorConfig.DryRun
}

// RIGHT: Let Viper handle it
// (But currently doesn't work because Viper doesn't know about atmosConfig)
```

## Proposed Solution

### Option A: Inject atmosConfig Into Viper (Preferred)

After loading atmosConfig in RootCmd.PersistentPreRun, inject its values into Viper as defaults:

```go
// cmd/root.go - After loading atmosConfig
func injectAtmosConfigIntoViper(atmosConfig schema.AtmosConfiguration, v *viper.Viper) {
    // EditorConfig settings
    if atmosConfig.Validate.EditorConfig.DryRun {
        v.SetDefault("dry-run", true)
    }
    if atmosConfig.Validate.EditorConfig.IgnoreDefaults {
        v.SetDefault("ignore-defaults", true)
    }
    if atmosConfig.Validate.EditorConfig.Format != "" {
        v.SetDefault("format", atmosConfig.Validate.EditorConfig.Format)
    }
    // ... for all editorconfig flags

    // Could extend to other commands' atmos.yaml settings
}

RootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
    atmosConfig, err = cfg.InitCliConfig(...)
    if err != nil {
        return err
    }

    // Inject atmosConfig values into Viper as defaults
    injectAtmosConfigIntoViper(atmosConfig, viper.GetViper())
}
```

**Then Viper's precedence works automatically:**
1. CLI flag (highest priority)
2. Environment variable
3. atmosConfig value (injected as Viper default)
4. Flag default value (lowest priority)

**Benefits:**
- Commands don't need manual precedence logic
- No `*Provided` boolean fields needed
- Viper handles everything automatically
- Less code, fewer bugs

### Option B: Accept Separate Configuration Systems

Document that atmos.yaml config is a separate layer and commands must handle it manually.

**Problems:**
- Violates DRY principle
- Error-prone (easy to get precedence wrong)
- Inconsistent (some flags use Viper, some use manual checking)
- More code to maintain

## Implementation Plan

### Phase 1: Create Viper Injection System

1. Add `injectAtmosConfigIntoViper()` function in cmd/root.go
2. Call it after atmosConfig is loaded in PersistentPreRun
3. Start with EditorConfig as proof-of-concept

### Phase 2: Simplify EditorConfig Command

1. Remove `*Provided` boolean fields from EditorConfigOptions
2. Remove manual precedence checking from initializeConfig()
3. Trust Viper's precedence system
4. Test that CLI > env > atmos.yaml > default works

### Phase 3: Extend to Other Commands

Apply the same pattern to any other commands that read from atmosConfig:
- Validate commands
- Terraform/Helmfile/Packer commands (if they use atmosConfig)
- Custom commands (if they use atmosConfig)

## Example: Before and After

### Before (Current - Manual Precedence)

```go
// EditorConfigOptions needs tracking fields
type EditorConfigOptions struct {
    DryRun         bool
    DryRunProvided bool  // Manually track if flag was provided
}

// Parser must check flag.Changed
func (p *EditorConfigParser) Parse(ctx context.Context, args []string) (*EditorConfigOptions, error) {
    opts := EditorConfigOptions{
        DryRun:         getBool(parsedConfig.Flags, "dry-run"),
        DryRunProvided: flagChanged("dry-run"),  // Manual tracking
    }
    return &opts, nil
}

// Command must manually implement precedence
func initializeConfig(opts *flags.EditorConfigOptions) {
    cliConfig.DryRun = opts.DryRun
    if !opts.DryRunProvided {
        cliConfig.DryRun = atmosConfig.Validate.EditorConfig.DryRun  // Manual check
    }
}
```

**Lines of code:** ~50 for manual precedence logic

### After (Proposed - Viper Precedence)

```go
// EditorConfigOptions just holds values
type EditorConfigOptions struct {
    DryRun bool  // No *Provided field needed
}

// Parser just reads from Viper
func (p *EditorConfigParser) Parse(ctx context.Context, args []string) (*EditorConfigOptions, error) {
    opts := EditorConfigOptions{
        DryRun: getBool(parsedConfig.Flags, "dry-run"),  // Viper handles precedence
    }
    return &opts, nil
}

// Command just uses the value - precedence already handled
func initializeConfig(opts *flags.EditorConfigOptions) {
    cliConfig.DryRun = opts.DryRun  // Already correct via Viper precedence
}

// Meanwhile in root.go:
func injectAtmosConfigIntoViper(atmosConfig schema.AtmosConfiguration, v *viper.Viper) {
    if atmosConfig.Validate.EditorConfig.DryRun {
        v.SetDefault("dry-run", true)
    }
}
```

**Lines of code:** ~5 lines in root.go for injection, remove ~45 lines of manual precedence logic

## Benefits

1. **Simpler code** - Remove all manual precedence checking
2. **Fewer bugs** - Viper's precedence is well-tested
3. **Consistent** - All flags work the same way
4. **DRY** - Single source of truth for precedence logic (Viper)
5. **Testable** - Can unit test by setting Viper defaults
6. **Maintainable** - Adding new flags doesn't require precedence boilerplate

## Risks and Mitigations

**Risk:** atmosConfig is loaded asynchronously (in PersistentPreRun)
**Mitigation:** Injection happens in PersistentPreRun before any command runs

**Risk:** Some commands might not want atmosConfig in Viper
**Mitigation:** Make injection opt-in per command or per config section

**Risk:** SetDefault might conflict with existing flag defaults
**Mitigation:** Only inject non-zero values, or document precedence clearly

## Acceptance Criteria

- [ ] `injectAtmosConfigIntoViper()` implemented and called in PersistentPreRun
- [ ] EditorConfig command simplified (no manual precedence)
- [ ] Tests verify precedence: CLI > env > atmos.yaml > default
- [ ] No `*Provided` fields needed for EditorConfig flags
- [ ] Documentation explains Viper injection pattern

## Future Considerations

This pattern should become standard for ALL commands that read configuration from atmos.yaml. The precedence order should always be:

1. CLI flag (highest)
2. Environment variable
3. atmos.yaml (injected into Viper)
4. Flag default (lowest)

No command should manually check `atmosConfig.*` values if they can be injected into Viper.

## References

- Current manual precedence: `cmd/validate_editorconfig.go` lines 104-161
- Code review fix that added manual precedence: commit 6c75eba0e
- Viper precedence docs: https://github.com/spf13/viper#why-viper
- PRD for this issue questioning manual precedence
