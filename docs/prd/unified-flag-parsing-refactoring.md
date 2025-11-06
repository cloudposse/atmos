# PRD: Unified Flag Parsing Refactoring

**Status**: Implementation In Progress
**Priority**: High
**Scope**: Complete flag parsing system overhaul
**Pull Request**: Unified flag parsing refactoring

## Executive Summary

This PRD consolidates all flag parsing improvements into a single comprehensive refactoring that addresses the root architectural issues in Atmos's flag handling system. The refactoring encompasses:

1. **Unified parser with compatibility aliases** - Replace custom parsing with Cobra+pflag
2. **Strongly-typed options pattern** - Replace map-based flags with type-safe structs
3. **Global flags via embedding** - Eliminate duplication across commands
4. **Safe argument parsing** - Fix shell quoting security issues
5. **Proper Cobra integration** - Enable validation, help text, and standard flag handling

**Related PRDs** (being consolidated into this document):
- `flag-handling/unified-flag-parsing.md` - Core architecture
- `flag-handling/global-flags-pattern.md` - Global flags design
- `unified-parser-with-compatibility-aliases.md` - Compatibility translation
- `safe-argument-parsing.md` - Shell quoting fixes
- `flag-handling/strongly-typed-builder-pattern.md` - Options structs
- `flag-handling/type-safe-positional-arguments.md` - Positional args builders

## Problem Statement

### Current Architecture Issues

Atmos's flag parsing has evolved into a fragmented system with multiple implementations:

1. **`DisableFlagParsing = true`** everywhere
   - Bypasses Cobra's validation and help generation
   - Requires custom `parseFlag()` that reinvents pflag
   - No type checking, no error messages for invalid flags

2. **Global flags duplicated in every parser**
   - Terraform, Packer, Helmfile each manually extract 17+ global flags
   - Inconsistent precedence handling (flags → env → config)
   - Testing nightmare (need to mock all flags in each test)

3. **Map-based flag access**
   - `parsedConfig.Flags["stack"].(string)` - runtime errors possible
   - No compile-time safety
   - Hard to refactor, easy to break

4. **Two separate parsers**
   - `StandardParser` for simple commands
   - `PassThroughFlagParser` for terraform/packer/helmfile
   - Duplicated logic, inconsistent behavior

5. **Terraform's legacy flags**
   - Single-dash multi-char flags (`-var`, `-var-file`) conflict with Cobra
   - Root cause of `DisableFlagParsing = true` workaround
   - Led to all other problems

### Root Cause Analysis

**The core issue**: Terraform uses `-var` (single-dash, multi-char) which conflicts with Cobra's POSIX parsing where `-var` would mean three separate flags: `-v -a -r`.

**Our response**: Disabled Cobra entirely (`DisableFlagParsing = true`) and implemented custom parsing.

**Consequences**: Lost all Cobra benefits (validation, help text, type checking) and created maintenance burden with custom parsing logic duplicated across codebase.

## Solution Architecture

### Core Strategy

**Don't fight Cobra - translate legacy syntax before Cobra sees it.**

```
User Input:  atmos terraform plan vpc -s dev -var x=1 -var-file prod.tfvars
              ↓
Compatibility Translator: -s → --stack, -var → --var, -var-file → separated args
              ↓
After Translation: atmos terraform plan vpc --stack dev --var x=1 -- -var-file prod.tfvars
              ↓
Cobra Parser: Validates --stack, --var normally
              ↓
Result: ParsedConfig {
  Flags: {stack: "dev", var: ["x=1"]},
  PositionalArgs: ["plan", "vpc"],
  PassThroughArgs: ["-var-file", "prod.tfvars"]
}
              ↓
Strongly-Typed: TerraformOptions {
  GlobalFlags: GlobalFlags{Stack: "dev"},
  Vars: []string{"x=1"},
  PassThroughArgs: ["-var-file", "prod.tfvars"]
}
```

### Architecture Components

#### 1. Compatibility Alias Translator

**Purpose**: Normalize legacy flag syntax to Cobra-compatible format.

**Two behaviors**:
- `MapToAtmosFlag`: Translate to Atmos flag (e.g., `-s` → `--stack`, `-var` → `--var`)
- `MoveToSeparated`: Move to pass-through args (e.g., `-var-file` → separated args)

**Implementation**: `pkg/flags/compatibility_translator.go`

```go
type CompatibilityAlias struct {
    Behavior CompatibilityBehavior
    Target   string  // Target flag name (for MapToAtmosFlag) or empty (for MoveToSeparated)
}

type CompatibilityAliasTranslator struct {
    aliasMap map[string]CompatibilityAlias
}

func (t *CompatibilityAliasTranslator) Translate(args []string) (atmosArgs []string, separatedArgs []string)
```

**Terraform compatibility aliases**:
- **Atmos shorthand**: `-s` → `--stack`, `-i` → `--identity`
- **Atmos-managed terraform flags**: `-var` → `--var`, `-out` → `--out`, `-auto-approve` → `--auto-approve`
- **Pass-through flags**: `-var-file`, `-target`, `-replace`, `-destroy`, etc. → separated args

#### 2. Unified Parser

**Purpose**: Single parser for all commands, replacing StandardParser and PassThroughFlagParser.

**Implementation**: `pkg/flags/unified_parser.go`

```go
type UnifiedParser struct {
    cmd        *cobra.Command
    viper      *viper.Viper
    translator *CompatibilityAliasTranslator
}

func (p *UnifiedParser) Parse(args []string) (*ParsedConfig, error) {
    // Step 1: Split at -- separator
    argsBeforeSep, argsAfterSep := splitAtSeparator(args)

    // Step 2: Translate compatibility aliases
    atmosArgs, translatedSeparated := p.translator.Translate(argsBeforeSep)

    // Step 3: Let Cobra parse (DisableFlagParsing = false!)
    if err := p.cmd.ParseFlags(atmosArgs); err != nil {
        return nil, err  // Cobra validation works!
    }

    // Step 4: Bind to Viper
    p.viper.BindPFlags(p.cmd.Flags())

    // Step 5: Build result
    return &ParsedConfig{
        Flags: buildFlagsMap(p.viper),
        PositionalArgs: p.cmd.Flags().Args(),
        PassThroughArgs: append(translatedSeparated, argsAfterSep...),
    }, nil
}
```

**Key features**:
- Enables Cobra validation (no more `DisableFlagParsing`)
- Handles `--` separator
- Supports NoOptDefVal for interactive flags (`--identity=` → `__SELECT__`)
- Single implementation for all command types

#### 3. Strongly-Typed Options Structs

**Purpose**: Replace `map[string]interface{}` with compile-time type safety.

**Pattern**:
```go
// Old way (runtime errors possible)
stack := parsedConfig.Flags["stack"].(string)  // Panic if not string!

// New way (compile-time safety)
opts := parsedConfig.ToTerraformOptions()
stack := opts.Stack  // Type checked by compiler
```

**Global flags via embedding**:
```go
type GlobalFlags struct {
    Stack       string
    Identity    string
    LogsLevel   string
    NoColor     bool
    DryRun      bool
    // ... 13+ more global flags
}

type TerraformOptions struct {
    GlobalFlags  // Embedded - all global flags available

    // Terraform-specific flags
    Vars            []string
    Out             string
    AutoApprove     bool
    UploadStatus    bool
    SkipInit        bool
    FromPlan        string

    // Pass-through args for subprocess
    PassThroughArgs []string
}
```

**Benefits**:
- **DRY**: Global flags defined once, embedded everywhere
- **Type-safe**: Compiler catches typos and type errors
- **Refactorable**: IDE can rename fields across codebase
- **Testable**: Mock structs easier than mock maps
- **Self-documenting**: Field names + types = clear API

#### 4. Positional Args Builders

**Purpose**: Type-safe extraction of positional arguments with clear requirements.

**Pattern**:
```go
// Terraform commands: expect [subcommand, component]
builder := NewTerraformPositionalArgsBuilder().
    WithSubcommand(Required).
    WithComponent(Required)

positionalArgs, err := builder.Extract(args)
// positionalArgs.Subcommand → "plan"
// positionalArgs.Component → "vpc"

// Workflow commands: expect [workflowName]
builder := NewWorkflowPositionalArgsBuilder().
    WithWorkflowName(Required)

positionalArgs, err := builder.Extract(args)
// positionalArgs.WorkflowName → "deploy-prod"
```

**Benefits**:
- Clear requirements (required vs optional)
- Better error messages
- Type-safe field access
- No magic array indices

#### 5. Safe Argument Parsing

**Purpose**: Fix shell quoting security/correctness issue when passing args to subprocesses.

**Problem**:
```go
// WRONG: Current implementation
trailingArgs := strings.Join(args, " ")  // "hello  world" → loses quotes
ExecuteShell(trailingArgs)  // Shell re-parses, splits on double space

// RIGHT: Proper implementation
ExecuteCommand(args)  // Pass array directly, no shell parsing
```

**Solution**: Use `exec.Command(cmd, args...)` instead of shell string execution.

## Terraform Flag Categorization

Critical design decision: Which terraform flags does Atmos process vs pass-through?

### Atmos-Managed Flags

Flags that Atmos processes and uses in its logic:

| Flag | Atmos Usage | Implementation |
|------|-------------|----------------|
| `--var` | Parsed by `getCliVars()` | `internal/exec/cli_utils.go:704-727` |
| `--out` | Checked for planfile validation | `internal/exec/terraform.go:409-412` |
| `--auto-approve` | Checked for auto-approval config | `internal/exec/terraform.go:364-373` |

These flags are:
- Registered with Cobra (validation, help text)
- Mapped via compatibility aliases (`-var` → `--var`)
- Accessible via `TerraformOptions` struct
- Processed by Atmos before calling terraform

### Pass-Through Flags

Flags that Atmos just passes to terraform subprocess:

`-var-file`, `-target`, `-replace`, `-destroy`, `-refresh-only`, `-lock`, `-lock-timeout`, `-parallelism`, `-state`, `-state-out`, `-backup`, `-json`, `-compact-warnings`, etc.

These flags are:
- NOT registered with Cobra (unknown to Atmos)
- Moved to separated args via compatibility aliases
- Passed directly to terraform subprocess
- No Atmos processing

### Unknown/Future Flags

For terraform flags not in either category (e.g., from plugins):
- **Use `--` separator**: `atmos terraform plan vpc -- -weird-plugin-flag value`
- Guarantees pass-through to terraform
- Future-proof: works with any terraform flag

## NoOptDefVal Pattern

**Critical requirement**: Support Cobra's NoOptDefVal for interactive flags.

### The Pattern

```go
// Register flag with NoOptDefVal
cmd.Flags().StringP("identity", "i", "", "Identity selector")
// Note: We do NOT set NoOptDefVal on Cobra flag anymore (Cobra design limitation)

// Instead, we detect empty values AFTER parsing
if flag.Changed && flag.Value.String() == "" {
    flag.Value.Set("__SELECT__")  // Trigger interactive selection
}
```

### Usage Modes

```bash
# Mode 1: Explicit value (equals syntax)
atmos terraform plan vpc --identity=prod

# Mode 2: Empty value (interactive selection)
atmos terraform plan vpc --identity=

# Mode 3: No flag (use default from config/env)
atmos terraform plan vpc
```

**Important**: Due to Cobra's design, `--identity prod` (space-separated) does NOT work with NoOptDefVal. The next arg ("prod") would be treated as positional. Users must use `--identity=prod` (equals syntax).

### Precedence

1. **Flag set with value** → use value
2. **Flag set without value** (`--identity=`) → use NoOptDefVal (`__SELECT__`)
3. **Flag not set** → use Viper (env/config)

### Viper Binding Caveat

```go
// Only bind environment variables, NOT the flag itself
viper.BindEnv("identity", "ATMOS_IDENTITY", "IDENTITY")

// Do NOT do this for NoOptDefVal flags:
// viper.BindPFlag("identity", cmd.Flags().Lookup("identity"))
// ^ This would interfere with Changed detection
```

## Implementation Status

### ✅ Phase 1: Core Infrastructure (COMPLETE)

**Completed**:
- ✅ CompatibilityAliasTranslator (51 tests passing)
- ✅ UnifiedParser (25 tests passing)
- ✅ TerraformOptions struct (complete with all flags)
- ✅ NoOptDefVal handling for interactive selection
- ✅ Comprehensive test coverage (100% for new code)

**Files**:
- `pkg/flags/compatibility_translator.go` + `_test.go`
- `pkg/flags/unified_parser.go` + `_test.go`
- `pkg/flags/terraform_options.go` + `_test.go`

### 🚧 Phase 1.5: Per-Subcommand Flag Registration (CURRENT - BLOCKING)

**Problem**: Current implementation has monolithic flag sets (`TerraformFlags()`, `PackerFlags()`, `HelmfileFlags()`) that register ALL flags for ALL subcommands. This causes:
- Incorrect help text (e.g., `terraform plan --help` shows `--auto-approve` which is apply-only)
- No validation (e.g., `terraform plan --auto-approve` doesn't error)
- Type safety impossible (can't have `TerraformPlanOptions` vs `TerraformApplyOptions`)

**Solution**: Per-subcommand flag builders and type-safe options structs.

**See**: [`per-subcommand-flag-registration.md`](per-subcommand-flag-registration.md) for complete specification.

**Tasks**:

**Terraform**:
- [ ] Create `TerraformPlanFlags()` + `TerraformPlanCompatibilityAliases()` + `TerraformPlanOptions`
- [ ] Create `TerraformApplyFlags()` + `TerraformApplyCompatibilityAliases()` + `TerraformApplyOptions`
- [ ] Create `TerraformDestroyFlags()` + `TerraformDestroyCompatibilityAliases()` + `TerraformDestroyOptions`
- [ ] Repeat for: init, output, workspace, import, state, etc.

**Packer**:
- [ ] Create `PackerBuildFlags()` + `PackerBuildCompatibilityAliases()` + `PackerBuildOptions`
- [ ] Repeat for: validate, inspect, etc.

**Helmfile**:
- [ ] Create `HelmfileSyncFlags()` + `HelmfileSyncCompatibilityAliases()` + `HelmfileSyncOptions`
- [ ] Repeat for: apply, destroy, diff, etc.

**Deprecation**:
- [ ] Mark `TerraformFlags()` as deprecated
- [ ] Mark `PackerFlags()` as deprecated
- [ ] Mark `HelmfileFlags()` as deprecated

**Critical**: This phase MUST be completed before Phase 2 (integration with actual commands).

### 📋 Phase 2: Terraform Integration (NEXT AFTER 1.5)

**Tasks**:
- [ ] Update terraform plan command to use `TerraformPlanFlags()` and `UnifiedParser`
- [ ] Update terraform apply command to use `TerraformApplyFlags()` and `UnifiedParser`
- [ ] Update terraform destroy command to use `TerraformDestroyFlags()` and `UnifiedParser`
- [ ] Update all other terraform subcommands
- [ ] Remove `DisableFlagParsing = true` from all terraform commands
- [ ] Update `internal/exec/terraform.go` to use per-subcommand options structs
- [ ] Run existing terraform integration tests
- [ ] Add new tests for compatibility alias scenarios

### 📋 Phase 3: Packer & Helmfile

**Tasks**:
- [ ] Apply same pattern to Packer
- [ ] Apply same pattern to Helmfile

### 🧹 Phase 4: Cleanup

**Tasks**:
- [ ] Delete PassThroughFlagParser (replaced by UnifiedParser)
- [ ] Delete custom `parseFlag()` logic
- [ ] Remove global flag duplication
- [ ] Delete deprecated `TerraformFlags()`, `PackerFlags()`, `HelmfileFlags()`
- [ ] Update documentation

## Future: Command-Specific Package Migration

### Completed: Flag Package Organization (Phase 0)

**Status**: ✅ COMPLETE - All flag packages organized into subdirectories

As part of the unified flag parsing refactoring, we've reorganized the flags package to support per-tool and per-command organization:

**Current structure**:
```
pkg/flags/
  ├── terraform/          # Terraform-specific flags and parsers
  │   ├── options.go      # TerraformOptions (renamed from Options)
  │   ├── parser.go       # Parser (tool-level parser)
  │   ├── plan.go         # PlanFlags(), PlanCompatibilityAliases(), PlanPositionalArgs()
  │   ├── apply.go        # ApplyFlags(), ApplyCompatibilityAliases(), ApplyPositionalArgs()
  │   ├── init.go         # InitFlags(), InitCompatibilityAliases(), InitPositionalArgs()
  │   ├── output.go       # OutputFlags(), OutputCompatibilityAliases(), OutputPositionalArgs()
  │   ├── destroy.go      # DestroyFlags(), DestroyCompatibilityAliases(), DestroyPositionalArgs()
  │   ├── validate.go     # ValidateFlags(), ValidateCompatibilityAliases(), ValidatePositionalArgs()
  │   ├── positional_args_builder.go
  │   ├── compatibility.go  # Common compatibility helper functions
  │   ├── common.go        # Common flag sets shared across terraform commands
  │   └── remaining_commands.go  # Other terraform subcommands
  │
  ├── helmfile/          # Helmfile-specific flags and parsers
  │   ├── options.go     # Options (renamed from HelmfileOptions)
  │   ├── parser.go      # Parser (renamed from HelmfileParser)
  │   ├── sync.go        # SyncFlags(), SyncPositionalArgs()
  │   ├── apply.go       # ApplyFlags(), ApplyPositionalArgs()
  │   ├── diff.go        # DiffFlags(), DiffPositionalArgs()
  │   ├── destroy.go     # DestroyFlags(), DestroyPositionalArgs()
  │   └── positional_args_builder.go
  │
  ├── packer/            # Packer-specific flags and parsers
  │   ├── options.go     # Options (renamed from PackerOptions)
  │   ├── parser.go      # Parser (renamed from PackerParser)
  │   ├── build.go       # BuildFlags(), BuildPositionalArgs()
  │   ├── validate.go    # ValidateFlags(), ValidatePositionalArgs()
  │   ├── init.go        # InitFlags(), InitPositionalArgs()
  │   └── positional_args_builder.go
  │
  ├── workflow/          # Workflow-specific flags and parsers
  │   ├── options.go
  │   ├── parser.go
  │   ├── builder.go
  │   └── positional_args_builder.go
  │
  ├── auth/              # Auth-specific flags and parsers
  │   ├── options.go     # AuthOptions (NOT renamed - kept as AuthOptions)
  │   ├── parser.go
  │   └── builder.go
  │
  ├── authexec/          # Auth exec flags
  │   ├── options.go
  │   └── parser.go
  │
  ├── authshell/         # Auth shell flags
  │   ├── options.go
  │   └── parser.go
  │
  ├── editorconfig/      # Editor config flags
  │   ├── options.go
  │   ├── parser.go
  │   └── builder.go
  │
  └── (root level - shared infrastructure)
      ├── global_flags.go           # Shared GlobalFlags struct
      ├── compatibility_translator.go
      ├── flag_parser.go
      ├── registry.go
      ├── standard.go
      └── ... (other shared types)
```

**Key principles**:
1. **Tool-specific packages**: Each tool (terraform, helmfile, packer) gets its own subdirectory
2. **Command-specific files**: Each subcommand gets its own file (e.g., `plan.go`, `apply.go`)
3. **Type renaming**: Types within packages drop tool prefix (e.g., `HelmfileParser` → `Parser`)
4. **Shared infrastructure at root**: Common types like `GlobalFlags` remain in `pkg/flags/`
5. **Import parent for shared types**: Subpackages import `github.com/cloudposse/atmos/pkg/flags` for shared types

### Future: Command Package Migration (Phase 5)

**Status**: 📋 PLANNED - After unified flag parsing is fully integrated

Once the unified flag parsing refactoring is complete (Phases 1-4), we should migrate the command structure to match the flag organization:

**Proposed future structure**:
```
cmd/
  ├── terraform/              # All terraform commands in subdirectory
  │   ├── terraform.go        # Root terraform command (replaces cmd/terraform.go)
  │   ├── plan.go             # terraform plan command
  │   ├── apply.go            # terraform apply command
  │   ├── init.go             # terraform init command
  │   ├── output.go           # terraform output command
  │   ├── destroy.go          # terraform destroy command
  │   ├── validate.go         # terraform validate command
  │   ├── workspace.go        # terraform workspace command
  │   └── ... (other terraform subcommands)
  │
  ├── helmfile/              # All helmfile commands in subdirectory
  │   ├── helmfile.go        # Root helmfile command (replaces cmd/helmfile.go)
  │   ├── sync.go            # helmfile sync command
  │   ├── apply.go           # helmfile apply command
  │   ├── diff.go            # helmfile diff command
  │   └── destroy.go         # helmfile destroy command
  │
  ├── packer/                # All packer commands in subdirectory
  │   ├── packer.go          # Root packer command (replaces cmd/packer.go)
  │   ├── build.go           # packer build command
  │   ├── validate.go        # packer validate command
  │   └── init.go            # packer init command
  │
  ├── workflow/              # Workflow commands
  │   └── workflow.go        # (if workflow gets subcommands in future)
  │
  ├── vendor/                # Vendor commands
  │   ├── vendor.go          # Root vendor command
  │   ├── pull.go            # vendor pull command
  │   └── ...
  │
  ├── describe/              # Describe commands
  │   ├── describe.go        # Root describe command
  │   ├── stacks.go          # describe stacks command
  │   ├── component.go       # describe component command
  │   └── ...
  │
  ├── auth/                  # Auth commands
  │   ├── auth.go            # Root auth command
  │   ├── exec.go            # auth exec command
  │   ├── shell.go           # auth shell command
  │   └── user/              # auth user subcommands
  │       ├── user.go
  │       ├── configure.go
  │       └── ...
  │
  └── (root level - top-level commands)
      ├── root.go            # Root command
      ├── version/           # version command
      └── ... (other root-level commands)
```

**Migration benefits**:
1. **Consistent organization**: Commands organized same way as their flags
2. **Better discoverability**: Related commands grouped together
3. **Easier navigation**: Find terraform plan code in `cmd/terraform/plan.go` and flags in `pkg/flags/terraform/plan.go`
4. **Cleaner imports**: `cmd/terraform/plan.go` imports `pkg/flags/terraform` naturally
5. **Per-command files**: Each subcommand in its own file, easier to maintain

**Migration challenges**:
1. **Command registry updates**: Need to update how commands register themselves
2. **Import path changes**: Update imports across codebase
3. **Cobra parent-child relationships**: Ensure subcommand hierarchy works correctly
4. **Testing updates**: Update command tests to work with new structure

**Recommended approach**:
1. Start with terraform (largest, most complex)
2. Create `cmd/terraform/` directory
3. Move `cmd/terraform.go` → `cmd/terraform/terraform.go`
4. Split terraform subcommands into separate files
5. Update command registry to support subdirectories
6. Repeat for helmfile, packer, vendor, describe, auth
7. Update all imports and tests

**Alignment with flags package**:
- `pkg/flags/terraform/plan.go` provides flags for `cmd/terraform/plan.go`
- `pkg/flags/helmfile/sync.go` provides flags for `cmd/helmfile/sync.go`
- `pkg/flags/packer/build.go` provides flags for `cmd/packer/build.go`

This creates a **mirror structure** between flags and commands, making the codebase more intuitive.

## Breaking Changes & Migration

### Potential Breaking Changes

1. **NoOptDefVal Pattern Change** (Low Impact)
   - **Old**: `--identity prod` (inconsistent behavior)
   - **New**: `--identity=prod` required, or `--identity=` for interactive
   - **Impact**: This pattern was already inconsistent

2. **Unknown Terraform Flags** (Medium Impact)
   - **Old**: Any flag like `-weird-flag` passed through silently
   - **New**: Unknown single-dash flags trigger Cobra error unless after `--`
   - **Solution**: Use `--` separator for unknown/plugin flags
   - **Impact**: Users with custom terraform plugins affected

3. **Flag Validation** (Low Impact - Beneficial)
   - **Old**: No validation (DisableFlagParsing)
   - **New**: Cobra validates flag types, required values
   - **Impact**: Catches user errors earlier (positive)

### Recommended Pattern (Future-Proof)

**Always use `--` to separate Atmos flags from tool flags**:

```bash
# Clear and explicit
atmos terraform plan vpc \
  --stack=prod \
  --identity=admin \
  -- \
  -var-file=common.tfvars \
  -var-file=prod.tfvars \
  -target=aws_instance.app
```

**Why this is best practice**:
- Works with ANY terraform/packer/helmfile flag (current or future)
- Clear visual separation
- Never requires compatibility aliases
- Eliminates ambiguity
- Future-proof

### Legacy Pattern (Still Works)

Common flags work without `--` via compatibility aliases:

```bash
# Compatibility mode (still works)
atmos terraform plan vpc -s prod -var region=us-east-1 -var-file prod.tfvars
```

### Migration Checklist

**Before upgrading**:
1. Review CI/CD scripts for terraform/packer/helmfile commands
2. Look for patterns like `--identity prod` → change to `--identity=prod`
3. If using custom terraform plugins with unknown flags, add `--` separator

**Testing**:
```bash
# Test in dry-run mode first
atmos terraform plan vpc --stack=dev --dry-run -- -var-file=prod.tfvars

# Verify help shows expected flags
atmos terraform plan --help
```

**If you hit issues**:
1. Add `--` separator before tool flags
2. Use `=` syntax: `--stack=dev` not `--stack dev`
3. Report issue on GitHub with example command

### Risk Assessment

| Pattern | Works? | Notes |
|---------|--------|-------|
| `atmos terraform plan vpc -s dev` | ✅ Yes | Compatibility alias |
| `atmos terraform plan vpc --stack=dev` | ✅ Yes | Modern syntax |
| `atmos terraform plan vpc -var x=1` | ✅ Yes | Atmos-managed flag |
| `atmos terraform plan vpc -var-file prod.tfvars` | ✅ Yes | Pass-through compatibility |
| `atmos terraform plan vpc -- -weird-plugin-flag` | ✅ Yes | Explicit separator |
| `atmos terraform plan vpc -weird-plugin-flag` | ⚠️ Maybe | Only if compatibility alias exists |
| `--identity prod` | ❌ No | Use `--identity=prod` (equals) |

## Blog Post Guidance

**Title**: "Unified Flag Parsing: Better Validation, Clearer Syntax"

**Key Messages**:

1. **Prefer `--` separator** (future-proof, works with everything)
2. **Compatibility aliases** maintain backwards compatibility for common patterns
3. **Breaking changes minimal** but possible - test first
4. **Benefits outweigh risks** - better validation, help text, consistency

**Tone**: Positive but honest about tradeoffs. Emphasize `--` separator as the "escape hatch" that guarantees compatibility.

**Include**:
- Migration checklist
- Recommended vs legacy patterns
- Risk assessment table
- Testing guidance
- Support channel (GitHub issues)

## Success Criteria

- ✅ All compatibility alias translation tests pass
- ✅ Unified parser tests pass with Cobra validation enabled
- [ ] Existing terraform/packer/helmfile tests pass
- [ ] No `DisableFlagParsing = true` in codebase
- [ ] Global flags registered once, inherited everywhere
- [ ] `--help` shows terraform flags properly
- [ ] Unknown flags trigger Cobra validation errors
- [ ] Legacy `-var` syntax works
- [ ] Modern `--var` syntax works
- [ ] `--` separator works
- [ ] NoOptDefVal pattern works (`--identity=`)
- [ ] >80% test coverage maintained

## Benefits

### Immediate

1. **Proper validation** - Cobra catches typos and invalid flags
2. **Better help text** - `--help` shows all supported flags with types
3. **Type safety** - Compiler catches errors instead of runtime panics
4. **Security** - Proper shell quoting (from safe-argument-parsing PRD)

### Long-term

1. **Maintainability** - Less custom code, more standard Cobra/pflag
2. **Consistency** - Same patterns across all commands
3. **Testability** - Easier to mock and test
4. **Extensibility** - Easy to add new flags/commands
5. **Future-proof** - `--` separator handles any terraform evolution

## References

### Related PRDs (Consolidated Here)

- `flag-handling/unified-flag-parsing.md` - Original architecture (now integrated)
- `flag-handling/global-flags-pattern.md` - Global flags embedding (now integrated)
- `flag-handling/global-flags-examples.md` - Examples (now integrated)
- `flag-handling/default-values-pattern.md` - Default values (now integrated)
- `unified-parser-with-compatibility-aliases.md` - Compatibility translation (now integrated)
- `safe-argument-parsing.md` - Shell quoting fixes (now integrated)
- `flag-handling/strongly-typed-builder-pattern.md` - Options structs (now integrated)
- `flag-handling/type-safe-positional-arguments.md` - Positional args (now integrated)

All these PRDs are now superseded by this unified document.

### Implementation Files

**Core infrastructure**:
- `pkg/flags/compatibility_translator.go` - Compatibility alias translation
- `pkg/flags/unified_parser.go` - Unified parser implementation
- `pkg/flags/terraform_options.go` - Strongly-typed options struct
- `pkg/flags/standard_builder.go` - Builder pattern for flag registration

**Tests**:
- `pkg/flags/compatibility_translator_test.go` - 51 test cases
- `pkg/flags/unified_parser_test.go` - 25 test cases
- `pkg/flags/terraform_options_test.go` - Options struct tests

### Documentation

- This PRD (source of truth)
- `flag-handling/README.md` - High-level overview (to be updated)
- Blog post (to be written)
