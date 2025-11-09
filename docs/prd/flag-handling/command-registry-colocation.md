# Command Registry Co-location Strategy

**Status:** 📋 Planning (Phase 2)
**Depends on:** `strongly-typed-builder-pattern.md` (Phase 1 - ✅ Complete)

## Overview

As commands migrate to the Command Registry pattern, their parsers and options should be co-located within the command package rather than living in `pkg/flags`. This creates a clear ownership model: **the command registry owns commands, so commands own their options.**

## Problem Statement

Currently, command-specific types are scattered:

```
pkg/flags/
├── terraform_parser.go       # TerraformParser
├── terraform_options.go      # TerraformOptions
├── helmfile_parser.go
├── helmfile_options.go
└── ...

cmd/
├── terraform.go              # Uses types from pkg/flags
├── helmfile.go
└── ...
```

**Issues:**
1. **Split ownership**: Command in `cmd/`, but its types in `pkg/flags`
2. **Unclear organization**: Should new command-specific types go in `pkg/flags` or `cmd/`?
3. **Command registry disconnect**: Registry manages commands, but doesn't co-locate their configuration

## Solution: Co-locate Options with Commands

When a command uses the Command Registry pattern, move its options to the command package:

```
cmd/terraform/
├── provider.go               # TerraformCommandProvider
├── terraform.go              # Main command + subcommands
├── options.go                # TerraformOptions (moved from pkg/flags)
└── terraform_test.go

cmd/describe/
├── provider.go               # DescribeCommandProvider
├── component.go              # component subcommand
├── stacks.go                 # stacks subcommand
├── options.go                # DescribeOptions (local to package)
└── describe_test.go
```

## What Stays in `pkg/flags`

The **reusable infrastructure** remains in `pkg/flags`:

### Core Infrastructure
```go
pkg/flags/
├── types.go                  # Flag, StringFlag, BoolFlag, IntFlag
├── registry.go               # FlagRegistry
├── parser.go                 # FlagParser interface
├── standard.go               # StandardParser (for standard commands)
├── terraform/parser.go       # AtmosFlagParser (Terraform-specific with compatibility flags)
└── compatibility_flags.go  # CompatibilityFlagsTranslator (handles -s → --stack)
```

**Note**: `PassThroughFlagParser` was deleted on 2025-11-06. Terraform now uses `AtmosFlagParser` with `CompatibilityFlagsTranslator`.

### Shared Types (Used by Multiple Commands)
```go
pkg/flags/
├── global_flags.go           # GlobalFlags (embedded everywhere)
├── selectors.go              # IdentitySelector, PagerSelector
└── standard_builder.go       # StandardOptionsBuilder (reusable)
```

### Functional Options
```go
pkg/flags/
└── options.go                # WithStringFlag, WithBoolFlag, etc.
```

## What Moves to Command Packages

### Command-Specific Options
```go
// cmd/terraform/options.go
package terraform

import "github.com/cloudposse/atmos/pkg/flags"

// TerraformOptions contains parsed terraform command flags.
type TerraformOptions struct {
    flags.GlobalFlags          // Embed shared flags
    Stack        string
    DryRun       bool
    UploadStatus bool          // Terraform-specific
    SkipInit     bool          // Terraform-specific
    FromPlan     string        // Terraform-specific
}
```

### Optional: Command-Specific Parsers
```go
// cmd/terraform/parser.go (optional - can reuse flags.NewAtmosFlagParser)
package terraform

var terraformParser = flags.NewAtmosFlagParser(
    flags.WithTerraformFlags(),
)
```

## Migration Patterns

### Pattern 1: Simple Commands (Use StandardOptionsBuilder)

For commands with common flags (stack, format, dry-run):

```go
// cmd/describe/component.go
package describe

import "github.com/cloudposse/atmos/pkg/flags"

// ComponentOptions is co-located with command.
type ComponentOptions struct {
    flags.GlobalFlags
    Stack     string
    Component string
    Format    string
}

// Parser using shared builder.
var componentParser = flags.NewStandardOptionsBuilder().
    WithStack(true).
    WithFormat("yaml").
    WithQuery().
    Build()

var componentCmd = &cobra.Command{
    Use: "component",
    RunE: func(cmd *cobra.Command, args []string) error {
        opts, err := parseComponentOptions(cmd, args)
        if err != nil {
            return err
        }
        // Use opts...
    },
}

func parseComponentOptions(cmd *cobra.Command, args []string) (*ComponentOptions, error) {
    v := viper.New()
    componentParser.BindFlagsToViper(cmd, v)

    stdOpts, err := componentParser.Parse(context.Background(), args)
    if err != nil {
        return nil, err
    }

    return &ComponentOptions{
        GlobalFlags: stdOpts.GlobalFlags,
        Stack:       stdOpts.Stack,
        Component:   stdOpts.GetPositionalArgs()[0],
        Format:      stdOpts.Format,
    }, nil
}
```

### Pattern 2: Complex Commands (Terraform with Compatibility Aliases)

For commands that pass args to external tools (terraform, helmfile) and need compatibility flags:

```go
// cmd/terraform/options.go
package terraform

import "github.com/cloudposse/atmos/pkg/flags"

type TerraformOptions struct {
    flags.GlobalFlags
    Stack            string
    DryRun           bool
    UploadStatus     bool
    SkipInit         bool
    FromPlan         string
    positionalArgs   []string
    passThroughArgs  []string
}

// cmd/terraform/parser.go
// NOTE: Terraform uses AtmosFlagParser (NOT PassThroughFlagParser, which was deleted 2025-11-06)
var terraformParser = flags.NewAtmosFlagParser(
    flags.WithStack(),
    flags.WithDryRun(),
    flags.WithBoolFlag("upload-status", "", false, "Upload plan to Atmos Pro"),
    flags.WithBoolFlag("skip-init", "", false, "Skip terraform init"),
    flags.WithStringFlag("from-plan", "", "", "Apply from plan file"),
)
```

## Benefits

### 1. Clear Ownership
- **Command package owns everything**: provider, command, options, tests
- **No split brain**: All command-related code in one place

### 2. Better Discoverability
```
cmd/terraform/          # Everything terraform-related
├── provider.go         # How it registers
├── terraform.go        # What it does
├── options.go          # What it needs
└── terraform_test.go   # How it's tested
```

### 3. Easier Navigation
- Want to understand terraform command? → Look in `cmd/terraform/`
- Want to add terraform flag? → Update `cmd/terraform/options.go`
- Want to test terraform? → See `cmd/terraform/terraform_test.go`

### 4. Independent Evolution
- Commands can add custom options without cluttering `pkg/flags`
- `pkg/flags` stays focused on reusable infrastructure
- Command-specific logic stays local

## Migration Checklist

When migrating a command to registry + co-location:

- [ ] Create command package: `cmd/{command}/`
- [ ] Create `provider.go` implementing `CommandProvider`
- [ ] Move/create `options.go` with command-specific options
- [ ] Create parser using `flags.NewStandardOptionsBuilder()` or `flags.NewPassThroughParser()`
- [ ] Update command to parse options instead of using `cmd.Flags()` directly
- [ ] Add tests for options parsing
- [ ] Register with registry: `internal.Register(&CommandProvider{})`
- [ ] Remove old command file from `cmd/`

## Examples

### Already Migrated (Phase 1 Complete)

**cmd/about/**
```
cmd/about/
├── about.go            # AboutCommandProvider + command
└── about_test.go
```
- ✅ Uses command registry
- ✅ No flags needed (simple command)

**cmd/version/**
```
cmd/version/
├── version.go          # VersionCommandProvider + VersionOptions
└── version_test.go
```
- ✅ Uses command registry
- ✅ Uses `flags.NewStandardParser()` with functional options
- ✅ Options co-located (`VersionOptions` in same file)

### To Be Migrated (Phase 2)

**cmd/terraform.go → cmd/terraform/**
- Move `flags.TerraformParser` → `cmd/terraform/parser.go`
- Move `flags.TerraformOptions` → `cmd/terraform/options.go`
- Create `cmd/terraform/provider.go`
- Keep using `flags.NewPassThroughParser()` (reusable infrastructure)

**cmd/describe_component.go → cmd/describe/**
- Create `cmd/describe/options.go` with `ComponentOptions`
- Use `flags.NewStandardOptionsBuilder()`
- Create `cmd/describe/provider.go`

## Non-Goals

**Do NOT move to command packages:**
- ✅ `GlobalFlags` - shared by all commands
- ✅ `IdentitySelector`, `PagerSelector` - shared types
- ✅ `FlagRegistry`, `FlagParser` - core infrastructure
- ✅ `StandardOptionsBuilder` - reusable builder
- ✅ `StandardParser` - reusable parser for standard commands
- ✅ `CompatibilityFlagsTranslator` - reusable translator for terraform compatibility

## Success Criteria

1. Each command in registry has all code in its package
2. `pkg/flags` contains only reusable infrastructure
3. New developers can find all command code in one place
4. Adding command-specific flag doesn't require changing `pkg/flags`

## References

- `docs/prd/command-registry-pattern.md` - Command registry pattern
- `docs/prd/flag-handling/strongly-typed-builder-pattern.md` - Builder pattern (Phase 1)
- `cmd/about/about.go` - Example of registry pattern
- `cmd/version/version.go` - Example of registry + options pattern
