# Terraform Command Registry Migration PRD

## Problem Statement

The current Terraform command implementation has several architectural issues that make migration to the command registry pattern challenging:

### Current Architecture Issues

1. **Import Cycle**: terraform package → cmd package → terraform package
   - `cmd/terraform/utils.go` needs `cmd.GetConfigAndStacksInfo()` and `cmd.CreateAuthManager()`
   - `cmd/root.go` imports `cmd/terraform` for registration
   - This creates a circular dependency

2. **Tight Coupling**: Terraform commands are tightly coupled to cmd package infrastructure
   - `getConfigAndStacksInfo()` is in cmd package
   - `createAuthManager()` is in cmd package
   - `terraformRun()` depends on both

3. **DisableFlagParsing**: All terraform commands use `DisableFlagParsing = true`
   - Flags are parsed manually in `terraformRun()`
   - This makes it incompatible with the flag handler's StandardParser approach

4. **Dynamic Command Generation**: Commands are generated from array in `getTerraformCommands()`
   - Makes it hard to add per-command flags and help
   - No place to define compatibility flags per command

## Solution: Phased Migration Approach

Given the complexity, we should migrate in multiple phases:

### Phase 1: Extract Shared Infrastructure (This PR)

**Goal**: Break the import cycle by extracting shared utilities

**Changes**:
1. Create `pkg/terraform/` package for shared terraform logic
   - Move `terraformRun()` → `pkg/terraform/executor.go`
   - Move `checkTerraformFlags()` → `pkg/terraform/validator.go`
   - Move `handleInteractiveIdentitySelection()` → `pkg/terraform/identity.go`
   - These functions can depend on `cmd` package without creating cycles

2. Keep `cmd/terraform*.go` files in place temporarily
   - Still use dynamic command generation
   - Still use `DisableFlagParsing = true`
   - But now call `pkg/terraform` functions

**Benefits**:
- No import cycles
- Shared code is reusable
- Minimal behavior changes
- Safe, incremental step

**Files Changed**:
- Create `pkg/terraform/executor.go`
- Create `pkg/terraform/validator.go`
- Create `pkg/terraform/identity.go`
- Update `cmd/terraform_utils.go` to use `pkg/terraform`
- Export `cmd.GetConfigAndStacksInfo()` and `cmd.CreateAuthManager()`

### Phase 2: Command Registry Integration (Follow-up PR)

**Goal**: Migrate terraform commands to registry pattern

**Changes**:
1. Create `cmd/terraform/` package structure
2. Implement `TerraformCommandProvider`
3. Create explicit command files (plan.go, apply.go, etc.)
4. Each command defines its own:
   - Cobra command with proper help
   - Compatibility flags for pass-through
   - RunE function calling `pkg/terraform.Execute()`

**Compatibility Strategy**:
- Keep `DisableFlagParsing = true` initially
- Add compatibility flags for documentation only
- Manual arg processing in RunE

**Files Changed**:
- Create `cmd/terraform/terraform.go` (parent + provider)
- Create `cmd/terraform/plan.go` through `cmd/terraform/workspace.go` (24 files)
- Create `cmd/terraform/deploy.go`, `clean.go`, etc. (5 custom commands)
- Create `cmd/terraform/generate/` package (4 files)
- Delete `cmd/terraform_commands.go`
- Update `cmd/root.go` import

### Phase 3: Flag Handler Integration (Future PR)

**Goal**: Replace manual arg parsing with flag handler

**Changes**:
1. Add StandardParser to each command
2. Define native Cobra flags (--stack, --identity, etc.)
3. Define compatibility flags (all terraform native flags)
4. Use `CompatibilityFlagTranslator` for preprocessing
5. Remove `DisableFlagParsing` usage
6. Proper help generation with all flags

**This is the final state** from the original plan.

## Phase 1 Implementation Details

### 1. Create pkg/terraform/executor.go

```go
package terraform

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteTerraformCommand executes a terraform command with the given configuration.
// This is the main entry point for all terraform command execution.
func ExecuteTerraformCommand(parentCmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	info := cmd.GetConfigAndStacksInfo(cfg.TerraformComponentType, parentCmd, args)

	if info.NeedHelp {
		err := actualCmd.Usage()
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Extract flags
	flags := parentCmd.Flags()

	processTemplates, err := flags.GetBool("process-templates")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	processYamlFunctions, err := flags.GetBool("process-functions")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	skip, err := flags.GetStringSlice("skip")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	components, err := flags.GetStringSlice("components")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	dryRun, err := flags.GetBool("dry-run")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	info.ProcessTemplates = processTemplates
	info.ProcessFunctions = processYamlFunctions
	info.Skip = skip
	info.Components = components
	info.DryRun = dryRun

	// Handle --identity flag
	if info.Identity == cfg.IdentityFlagSelectValue {
		HandleInteractiveIdentitySelection(&info)
	}

	// Check flags
	err = ValidateFlags(&info)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	// Execute affected
	if info.Affected {
		parentCmd.PersistentFlags().String("file", "", "")
		parentCmd.PersistentFlags().String("format", "yaml", "")
		parentCmd.PersistentFlags().Bool("verbose", false, "")
		parentCmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "")
		parentCmd.PersistentFlags().Bool("include-settings", false, "")
		parentCmd.PersistentFlags().Bool("upload", false, "")

		a, err := e.ParseDescribeAffectedCliArgs(parentCmd, args)
		if err != nil {
			return err
		}

		a.IncludeSpaceliftAdminStacks = false
		a.IncludeSettings = false
		a.Upload = false
		a.OutputFile = ""

		err = e.ExecuteTerraformAffected(&a, &info)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Execute query/multi-component
	if info.All || len(info.Components) > 0 || info.Query != "" || (info.Stack != "" && info.ComponentFromArg == "") {
		err = e.ExecuteTerraformQuery(&info)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Execute single component
	err = e.ExecuteTerraform(info)
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			errUtils.CheckErrorAndPrint(err, "", "")
			return err
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	return nil
}
```

### 2. Create pkg/terraform/validator.go

```go
package terraform

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateFlags checks the usage of Single-Component and Multi-Component flags.
func ValidateFlags(info *schema.ConfigAndStacksInfo) error {
	// Check Multi-Component flags
	if info.ComponentFromArg != "" && (info.All || info.Affected || len(info.Components) > 0 || info.Query != "") {
		return fmt.Errorf("component `%s`: %w", info.ComponentFromArg, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	}

	if info.Affected && (info.All || len(info.Components) > 0 || info.Query != "") {
		return errUtils.ErrInvalidTerraformFlagsWithAffectedFlag
	}

	// Single-Component and Multi-Component flags are not allowed together
	singleComponentFlagPassed := info.PlanFile != "" || info.UseTerraformPlan
	multiComponentFlagPassed := info.Affected || info.All || len(info.Components) > 0 || info.Query != ""
	if singleComponentFlagPassed && multiComponentFlagPassed {
		return errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags
	}

	return nil
}
```

### 3. Create pkg/terraform/identity.go

```go
package terraform

import (
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// HandleInteractiveIdentitySelection handles the case where --identity was used without a value.
func HandleInteractiveIdentitySelection(info *schema.ConfigAndStacksInfo) {
	atmosConfig, err := cfg.InitCliConfig(*info, false)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err), "", "")
	}

	if len(atmosConfig.Auth.Providers) == 0 && len(atmosConfig.Auth.Identities) == 0 {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: no authentication configured", errUtils.ErrNoIdentitiesAvailable), "", "")
	}

	authManager, err := cmd.CreateAuthManager(&atmosConfig.Auth)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err), "", "")
	}

	selectedIdentity, err := authManager.GetDefaultIdentity(true)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			log.Debug("User aborted identity selection, exiting with SIGINT code")
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%w: %w", errUtils.ErrDefaultIdentity, err), "", "")
	}

	info.Identity = selectedIdentity
}
```

### 4. Update cmd/terraform_utils.go

```go
package cmd

import (
	"github.com/spf13/cobra"

	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/terraform"
)

func runHooks(event h.HookEvent, cmd *cobra.Command, args []string) error {
	// Existing hook code...
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	// Delegate to pkg/terraform
	return terraform.ExecuteTerraformCommand(cmd, actualCmd, args)
}
```

## Benefits of Phased Approach

✅ **Phase 1 (This PR)**:
- Breaks import cycle
- No behavior changes
- Safe, testable
- Foundation for future work
- ~300 lines of new code
- Can be done in 1 day

✅ **Phase 2 (Follow-up)**:
- Explicit commands with proper help
- Command registry integration
- Still works exactly the same
- ~2,500 lines of new code
- 3-4 days of work

✅ **Phase 3 (Future)**:
- Full flag handler integration
- Compatibility flags working
- Native and pass-through flags in help
- Cleanest architecture
- ~1,000 lines of changes
- 2-3 days of work

## Recommendation

**Proceed with Phase 1 only in this PR**. This provides immediate value (breaks import cycle, extracts reusable code) without the risk of a massive refactoring.

Phases 2 and 3 can be tackled in separate PRs after Phase 1 is proven stable.

## Timeline

- Phase 1: 1 day (this PR)
- Phase 2: 3-4 days (follow-up PR)
- Phase 3: 2-3 days (future PR)
- **Total: 6-8 days** split across 3 PRs instead of one risky 20-hour PR

## Success Criteria (Phase 1)

✅ Import cycle eliminated
✅ All existing tests pass
✅ No behavior changes
✅ Terraform commands still work identically
✅ New `pkg/terraform` package is well-tested
✅ Code coverage >80% for new package
