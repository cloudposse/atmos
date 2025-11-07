# FlagParser Integration Plan - Full Integration (Option B)

**⚠️ STATUS: OBSOLETE (2025-11-06)**

This document describes a historical design that has been superseded. The actual implementation:
- Uses `StandardParser` (not `StandardFlagParser`) for standard commands
- Uses `AtmosFlagParser` (not `PassThroughFlagParser`) for terraform with compatibility aliases
- `PassThroughFlagParser` was completely deleted on 2025-11-06
- See `../unified-flag-parsing-refactoring.md` for current architecture

## Historical Objective
Replace all manual flag parsing in Atmos with the unified `pkg/flagparser` system. Remove `DisableFlagParsing = true` and let Cobra + our parsers handle everything properly.

## Success Criteria (Historical)
✅ Remove `DisableFlagParsing = true` from all commands
✅ Remove manual arg parsing code (`ExtractSeparatedArgs`, `processArgsAndFlags`, etc.)
✅ All commands use FlagParser (**ACTUAL: StandardParser or AtmosFlagParser, NOT PassThroughFlagParser**)
✅ Flag precedence works correctly: flags > env vars > config files
✅ Identity flag NoOptDefVal works correctly
✅ Double dash separator (`--`) works correctly
✅ All existing tests pass
✅ No behavioral changes from user perspective

**Migration Complete**: All these criteria have been met with the modern architecture (AtmosFlagParser + StandardParser).

## Phase 2A: Terraform Command Integration

### Current State Analysis

**Files to modify:**
- `cmd/terraform.go` - Base terraform command
- `cmd/terraform_commands.go` - Subcommands (plan, apply, etc.)
- `cmd/terraform_utils.go` - terraformRun function
- `cmd/cmd_utils.go` - getConfigAndStacksInfo function
- `internal/exec/terraform.go` - ProcessCommandLineArgs function

**Current flow:**
1. `cmd/terraform.go:init()` sets `DisableFlagParsing = true`
2. `cmd/terraform_commands.go:attachTerraformCommands()` defines flags but Cobra doesn't parse them
3. `cmd/terraform_utils.go:terraformRun()` uses `os.Args[2:]` to get raw args
4. `cmd/cmd_utils.go:getConfigAndStacksInfo()` manually splits at `--`
5. `internal/exec/terraform.go:ProcessCommandLineArgs()` does the actual parsing

### Proposed New Flow

1. **`cmd/terraform.go:init()`**:
   - **REMOVE**: `DisableFlagParsing = true`
   - **ADD**: Create `PassThroughFlagParser` instance
   - **ADD**: Call `parser.RegisterFlags(terraformCmd)`

2. **`cmd/terraform_commands.go:attachTerraformCommands()`**:
   - **REMOVE**: Manual `PersistentFlags()` calls
   - **REMOVE**: `DisableFlagParsing = true` for subcommands
   - **ADD**: Use `WithTerraformFlags()` option when creating parser
   - **MODIFY**: `RunE` function to use parser

3. **`cmd/terraform_utils.go:terraformRun()`**:
   - **REMOVE**: `flags.GetBool()` calls (flags now come from ParsedConfig)
   - **REMOVE**: Special identity handling (parser handles it)
   - **ADD**: Get flags from `ParsedConfig.AtmosFlags`

4. **`cmd/cmd_utils.go:getConfigAndStacksInfo()`**:
   - **REMOVE**: Manual `--` splitting
   - **ADD**: Use parser.Parse() to get ParsedConfig
   - **MODIFY**: Pass ParsedConfig to ProcessCommandLineArgs

5. **`internal/exec/terraform.go:ProcessCommandLineArgs()`**:
   - **MODIFY**: Accept ParsedConfig instead of raw args
   - **REMOVE**: Manual flag extraction logic
   - **SIMPLIFY**: Extract component/stack from positional args only

### Implementation Steps

#### Step 1: Create terraform flag parser in cmd/terraform.go

```go
package cmd

import (
    "github.com/cloudposse/atmos/pkg/flagparser"
    "github.com/spf13/cobra"
)

var (
    terraformParser *flagparser.PassThroughFlagParser
)

func init() {
    // Create parser with Terraform flags
    terraformParser = flagparser.NewPassThroughFlagParser(
        flagparser.WithTerraformFlags(),
    )

    // Register flags with Cobra (this replaces DisableFlagParsing)
    terraformParser.RegisterFlags(terraformCmd)
    terraformParser.BindToViper(viper.GetViper())

    // NOTE: We do NOT set DisableFlagParsing = true anymore

    AddStackCompletion(terraformCmd)
    attachTerraformCommands(terraformCmd)
    RootCmd.AddCommand(terraformCmd)
}
```

#### Step 2: Update attachTerraformCommands

```go
func attachTerraformCommands(parentCmd *cobra.Command) {
    // REMOVE all the PersistentFlags() calls - they're now in the parser

    commands := getTerraformCommands()

    for _, cmd := range commands {
        // REMOVE: cmd.DisableFlagParsing = true
        // REMOVE: cmd.FParseErrWhitelist.UnknownFlags = true

        if setFlags, ok := commandMaps[cmd.Use]; ok {
            setFlags(cmd)
        }

        cmd.ValidArgsFunction = ComponentsArgCompletion
        cmd.RunE = func(cmd_ *cobra.Command, args []string) error {
            handleHelpRequest(cmd, args)
            enableHeatmapIfRequested()

            // NEW: Parse args with flagparser
            ctx := context.Background()
            parsedConfig, err := terraformParser.Parse(ctx, args)
            if err != nil {
                return err
            }

            // Pass ParsedConfig to terraformRun
            return terraformRun(parentCmd, cmd_, parsedConfig)
        }
        parentCmd.AddCommand(cmd)
    }
}
```

#### Step 3: Update terraformRun signature

```go
// OLD signature:
func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error

// NEW signature:
func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, parsedConfig *flagparser.ParsedConfig) error {
    info := getConfigAndStacksInfo(cfg.TerraformComponentType, cmd, parsedConfig)

    // Get flags from ParsedConfig instead of cmd.Flags()
    processTemplates := parsedConfig.AtmosFlags["process-templates"].(bool)
    processYamlFunctions := parsedConfig.AtmosFlags["process-functions"].(bool)
    skip := parsedConfig.AtmosFlags["skip"].([]string)
    components := parsedConfig.AtmosFlags["components"].([]string)
    dryRun := parsedConfig.AtmosFlags["dry-run"].(bool)

    info.ProcessTemplates = processTemplates
    info.ProcessFunctions = processYamlFunctions
    info.Skip = skip
    info.Components = components
    info.DryRun = dryRun

    // Identity already handled by parser
    if info.Identity == cfg.IdentityFlagSelectValue {
        handleInteractiveIdentitySelection(&info)
    }

    // ... rest of function unchanged
}
```

#### Step 4: Update getConfigAndStacksInfo

```go
// OLD signature:
func getConfigAndStacksInfo(commandName string, cmd *cobra.Command, args []string) schema.ConfigAndStacksInfo

// NEW signature:
func getConfigAndStacksInfo(commandName string, cmd *cobra.Command, parsedConfig *flagparser.ParsedConfig) schema.ConfigAndStacksInfo {
    checkAtmosConfig()

    // Parser already split args at --
    // No need for manual splitting

    info, err := e.ProcessCommandLineArgs(commandName, cmd, parsedConfig)
    errUtils.CheckErrorPrintAndExit(err, "", "")
    return info
}
```

#### Step 5: Update ProcessCommandLineArgs

```go
// OLD signature:
func ProcessCommandLineArgs(commandName string, cmd *cobra.Command, args []string, argsAfterDoubleDash []string) (schema.ConfigAndStacksInfo, error)

// NEW signature:
func ProcessCommandLineArgs(commandName string, cmd *cobra.Command, parsedConfig *flagparser.ParsedConfig) (schema.ConfigAndStacksInfo, error) {
    info := schema.ConfigAndStacksInfo{}

    // Extract from ParsedConfig
    info.SubCommand = parsedConfig.SubCommand
    info.ComponentFromArg = parsedConfig.ComponentName
    info.Stack = parsedConfig.Stack
    info.Identity = parsedConfig.Identity
    info.DryRun = parsedConfig.DryRun

    // Tool-specific args already separated
    info.AdditionalArgsAndFlags = parsedConfig.PassThroughArgs

    // ... rest of processing
}
```

### Files to Delete/Refactor

After integration, these become obsolete:
- `cmd/args_separator.go` - Replaced by PassThroughFlagParser.SplitAtDoubleDash()
- `cmd/flag_utils.go` - Replaced by flagparser methods
- Parts of `internal/exec/terraform.go:processArgsAndFlags()` - Simplified significantly

### Testing Strategy

1. **Unit tests**: Test parser integration in isolation
2. **Integration tests**: Update existing terraform command tests
3. **Manual testing**: Run real terraform commands
   - `atmos terraform plan vpc -s dev`
   - `atmos terraform plan vpc -s dev -- -var foo=bar`
   - `atmos terraform plan vpc --stack=dev --identity admin`
   - `atmos terraform plan vpc --identity` (interactive)
4. **Edge cases**: Test problematic scenarios from PRD

### Rollout Plan

1. Implement changes in feature branch
2. Run full test suite
3. Manual testing with real stacks
4. Code review
5. Merge to main

## Phase 2B: Other Pass-Through Commands (Historical - NOW COMPLETE)

**ACTUAL IMPLEMENTATION (2025-11-06)**:
- ✅ Helmfile commands use `StandardParser` (NOT PassThroughFlagParser)
- ✅ Packer commands use `StandardParser` (NOT PassThroughFlagParser)
- ✅ Workflow commands use `StandardParser`
- ✅ PassThroughFlagParser was deleted entirely

## Phase 2C: Standard Commands (Historical - NOW COMPLETE)

**ACTUAL IMPLEMENTATION (2025-11-06)**:
- ✅ All standard commands use `StandardParser`
- ✅ `version`, `describe`, `list`, etc. all migrated
- ✅ Consistent pattern across all commands

## Risk Mitigation

**Risk**: Breaking existing behavior
**Mitigation**: Comprehensive test coverage, feature flag to enable/disable new parser

**Risk**: Performance regression
**Mitigation**: Benchmark before/after, parser is lightweight

**Risk**: Edge cases not covered
**Mitigation**: Extensive testing with real-world scenarios from PRD

## Success Metrics

- ✅ Zero `DisableFlagParsing = true` in codebase
- ✅ All terraform tests pass
- ✅ All helmfile tests pass
- ✅ All packer tests pass
- ✅ Coverage >90% for flagparser package
- ✅ No user-facing behavioral changes
- ✅ Code is simpler and more maintainable

## Timeline

- Phase 2A (Terraform): 2-3 hours
- Phase 2B (Helmfile/Packer): 1-2 hours
- Phase 2C (Cleanup): 1 hour
- Testing & refinement: 2-3 hours

**Total estimate**: 6-9 hours of focused work
