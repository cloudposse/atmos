# PRD: Per-Subcommand Flag Registration

**Status**: Ready for Implementation (Phase 1.5 - Required before Phase 2)
**Priority**: High - Blocking terraform/packer/helmfile integration
**Parent PRD**: `unified-flag-parsing-refactoring.md`

## Problem Statement

Current implementation has **monolithic flag sets** that register ALL flags for ALL subcommands:

```go
// WRONG: Current approach
TerraformFlags()  // Returns flags for plan + apply + destroy + ... everything
PackerFlags()     // Returns flags for build + validate + ... everything
HelmfileFlags()   // Returns flags for sync + apply + ... everything
```

**Issues**:
1. **Incorrect help text** - `terraform plan --help` shows `-auto-approve` (which is apply-only)
2. **No validation** - `terraform plan --auto-approve` doesn't error (should be invalid)
3. **Confusing UX** - Users see irrelevant flags
4. **Type safety impossible** - Can't have `TerraformPlanOptions` vs `TerraformApplyOptions`

## Solution: Per-Subcommand Flag Builders

### Architecture

Each subcommand gets:
1. **Flag registry builder** - Returns only flags for that subcommand
2. **Compatibility aliases** - Only terraform/packer/helmfile legacy flags for that subcommand
3. **Type-safe options struct** - Strongly-typed fields specific to that subcommand

### Terraform Example

#### Flag Registry Builders

```go
// pkg/flags/terraform_flags.go

// TerraformCommonFlags returns flags common to ALL terraform subcommands.
// These are Atmos-processed flags shared across plan/apply/destroy/etc.
func TerraformCommonFlags() *FlagRegistry {
    registry := CommonFlags()  // Includes: stack (-s), dry-run

    // Terraform variable flag (common to plan/apply/destroy)
    registry.Register(&StringSliceFlag{
        Name:        "var",
        Shorthand:   "",  // No shorthand - this is NOT the same as terraform's -var
        Description: "Set terraform variables (Atmos-managed)",
        EnvVars:     []string{"ATMOS_TERRAFORM_VAR"},
    })

    return registry
}

// TerraformPlanFlags returns flags specific to `terraform plan` subcommand.
func TerraformPlanFlags() *FlagRegistry {
    registry := TerraformCommonFlags()

    // Plan-specific Atmos flags
    registry.Register(&StringFlag{
        Name:        "out",
        Description: "Path to save plan file (Atmos checks this)",
        EnvVars:     []string{"ATMOS_TERRAFORM_PLAN_OUT"},
    })

    registry.Register(&BoolFlag{
        Name:        "upload-status",
        Description: "Upload plan status to Atmos Pro",
        Default:     false,
        EnvVars:     []string{"ATMOS_UPLOAD_STATUS"},
    })

    return registry
}

// TerraformApplyFlags returns flags specific to `terraform apply` subcommand.
func TerraformApplyFlags() *FlagRegistry {
    registry := TerraformCommonFlags()

    // Apply-specific Atmos flags
    registry.Register(&BoolFlag{
        Name:        "auto-approve",
        Description: "Auto-approve terraform apply (Atmos checks this)",
        Default:     false,
        EnvVars:     []string{"ATMOS_TERRAFORM_AUTO_APPROVE"},
    })

    registry.Register(&StringFlag{
        Name:        "from-plan",
        Description: "Apply from previously generated plan file",
        Default:     "",
        EnvVars:     []string{"ATMOS_TERRAFORM_FROM_PLAN"},
    })

    return registry
}

// TerraformDestroyFlags returns flags specific to `terraform destroy` subcommand.
func TerraformDestroyFlags() *FlagRegistry {
    registry := TerraformCommonFlags()

    // Destroy-specific Atmos flags
    registry.Register(&BoolFlag{
        Name:        "auto-approve",
        Description: "Auto-approve terraform destroy",
        Default:     false,
        EnvVars:     []string{"ATMOS_TERRAFORM_AUTO_APPROVE"},
    })

    return registry
}

// ... TerraformInitFlags, TerraformOutputFlags, etc.
```

#### Compatibility Aliases (Per-Subcommand)

**CRITICAL DISTINCTION**:
- **Native Cobra shorthands** (NOT compatibility aliases): `-s` (stack), `-i` (identity)
  - Single-dash, **single character**
  - Registered via `StringP("stack", "s", ...)` - the `P` suffix means "with shorthand"
  - Handled natively by Cobra, no translation needed
- **Compatibility aliases** (terraform's legacy syntax): `-var`, `-var-file`, `-auto-approve`
  - Single-dash, **multi-character**
  - Conflict with Cobra's POSIX parsing (would be interpreted as `-v -a -r`)
  - Require translation before Cobra sees them

```go
// TerraformPlanCompatibilityAliases returns compatibility aliases for `terraform plan`.
// These translate terraform's legacy single-dash multi-char flags.
//
// IMPORTANT: -s and -i are NOT included here - they're native Cobra shorthands,
// registered with StringP("stack", "s", ...) and StringP("identity", "i", ...).
// Only terraform's multi-character single-dash flags need compatibility translation.
func TerraformPlanCompatibilityAliases() map[string]CompatibilityAlias {
    return map[string]CompatibilityAlias{
        // Atmos-managed terraform flags (terraform's legacy syntax)
        "-var": {Behavior: MapToAtmosFlag, Target: "--var"},
        "-out": {Behavior: MapToAtmosFlag, Target: "--out"},

        // Pass-through flags (plan-specific, terraform's legacy syntax)
        "-destroy":         {Behavior: MoveToSeparated, Target: ""},
        "-refresh-only":    {Behavior: MoveToSeparated, Target: ""},
        "-target":          {Behavior: MoveToSeparated, Target: ""},
        "-replace":         {Behavior: MoveToSeparated, Target: ""},
        "-var-file":        {Behavior: MoveToSeparated, Target: ""},
        "-compact-warnings": {Behavior: MoveToSeparated, Target: ""},
        "-detailed-exitcode": {Behavior: MoveToSeparated, Target: ""},
        "-input":           {Behavior: MoveToSeparated, Target: ""},
        "-lock":            {Behavior: MoveToSeparated, Target: ""},
        "-lock-timeout":    {Behavior: MoveToSeparated, Target: ""},
        "-no-color":        {Behavior: MoveToSeparated, Target: ""},
        "-parallelism":     {Behavior: MoveToSeparated, Target: ""},
        "-refresh":         {Behavior: MoveToSeparated, Target: ""},
        "-state":           {Behavior: MoveToSeparated, Target: ""},
    }
}

// TerraformApplyCompatibilityAliases returns compatibility aliases for `terraform apply`.
func TerraformApplyCompatibilityAliases() map[string]CompatibilityAlias {
    return map[string]CompatibilityAlias{
        // Atmos-managed terraform flags
        "-var":          {Behavior: MapToAtmosFlag, Target: "--var"},
        "-auto-approve": {Behavior: MapToAtmosFlag, Target: "--auto-approve"},

        // Pass-through flags (apply-specific)
        "-backup":          {Behavior: MoveToSeparated, Target: ""},
        "-compact-warnings": {Behavior: MoveToSeparated, Target: ""},
        "-input":           {Behavior: MoveToSeparated, Target: ""},
        "-lock":            {Behavior: MoveToSeparated, Target: ""},
        "-lock-timeout":    {Behavior: MoveToSeparated, Target: ""},
        "-no-color":        {Behavior: MoveToSeparated, Target: ""},
        "-parallelism":     {Behavior: MoveToSeparated, Target: ""},
        "-state":           {Behavior: MoveToSeparated, Target: ""},
        "-state-out":       {Behavior: MoveToSeparated, Target: ""},
        "-var-file":        {Behavior: MoveToSeparated, Target: ""},
    }
}

// TerraformDestroyCompatibilityAliases returns compatibility aliases for `terraform destroy`.
func TerraformDestroyCompatibilityAliases() map[string]CompatibilityAlias {
    return map[string]CompatibilityAlias{
        // Atmos-managed terraform flags
        "-var":          {Behavior: MapToAtmosFlag, Target: "--var"},
        "-auto-approve": {Behavior: MapToAtmosFlag, Target: "--auto-approve"},

        // Pass-through flags (destroy-specific)
        "-backup":       {Behavior: MoveToSeparated, Target: ""},
        "-lock":         {Behavior: MoveToSeparated, Target: ""},
        "-lock-timeout": {Behavior: MoveToSeparated, Target: ""},
        "-no-color":     {Behavior: MoveToSeparated, Target: ""},
        "-parallelism":  {Behavior: MoveToSeparated, Target: ""},
        "-refresh":      {Behavior: MoveToSeparated, Target: ""},
        "-state":        {Behavior: MoveToSeparated, Target: ""},
        "-state-out":    {Behavior: MoveToSeparated, Target: ""},
        "-target":       {Behavior: MoveToSeparated, Target: ""},
        "-var-file":     {Behavior: MoveToSeparated, Target: ""},
    }
}
```

#### Type-Safe Options Structs

```go
// pkg/flags/terraform_plan_options.go

// TerraformPlanOptions provides strongly-typed access to `terraform plan` flags.
type TerraformPlanOptions struct {
    GlobalFlags // Embedded: chdir, logs-level, identity, pager, profiler, etc.

    // Common terraform flags
    Stack  string   // --stack/-s
    DryRun bool     // --dry-run
    Var    []string // --var (Atmos-managed)

    // Plan-specific flags
    Out          string // --out (Atmos checks for planfile)
    UploadStatus bool   // --upload-status (upload to Atmos Pro)

    // Positional args
    Component string // Component name from positional arg

    // Pass-through args
    PassThroughArgs []string // Args after -- or from compatibility aliases
}

// ParseTerraformPlanFlags parses flags for `terraform plan` subcommand.
func ParseTerraformPlanFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) TerraformPlanOptions {
    // Extract component from positional args
    component := ""
    if len(positionalArgs) >= 2 {
        component = positionalArgs[1] // ["plan", "vpc"] → "vpc"
    }

    return TerraformPlanOptions{
        GlobalFlags:     ParseGlobalFlags(cmd, v),
        Stack:           v.GetString("stack"),
        DryRun:          v.GetBool("dry-run"),
        Var:             v.GetStringSlice("var"),
        Out:             v.GetString("out"),
        UploadStatus:    v.GetBool("upload-status"),
        Component:       component,
        PassThroughArgs: passThroughArgs,
    }
}

// pkg/flags/terraform_apply_options.go

// TerraformApplyOptions provides strongly-typed access to `terraform apply` flags.
type TerraformApplyOptions struct {
    GlobalFlags

    // Common terraform flags
    Stack  string
    DryRun bool
    Var    []string

    // Apply-specific flags
    AutoApprove bool   // --auto-approve
    FromPlan    string // --from-plan

    // Positional args
    Component string

    // Pass-through args
    PassThroughArgs []string
}

// ParseTerraformApplyFlags parses flags for `terraform apply` subcommand.
func ParseTerraformApplyFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) TerraformApplyOptions {
    component := ""
    if len(positionalArgs) >= 2 {
        component = positionalArgs[1]
    }

    return TerraformApplyOptions{
        GlobalFlags:     ParseGlobalFlags(cmd, v),
        Stack:           v.GetString("stack"),
        DryRun:          v.GetBool("dry-run"),
        Var:             v.GetStringSlice("var"),
        AutoApprove:     v.GetBool("auto-approve"),
        FromPlan:        v.GetString("from-plan"),
        Component:       component,
        PassThroughArgs: passThroughArgs,
    }
}

// pkg/flags/terraform_destroy_options.go

// TerraformDestroyOptions provides strongly-typed access to `terraform destroy` flags.
type TerraformDestroyOptions struct {
    GlobalFlags

    // Common terraform flags
    Stack  string
    DryRun bool
    Var    []string

    // Destroy-specific flags
    AutoApprove bool // --auto-approve

    // Positional args
    Component string

    // Pass-through args
    PassThroughArgs []string
}

// ParseTerraformDestroyFlags parses flags for `terraform destroy` subcommand.
func ParseTerraformDestroyFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) TerraformDestroyOptions {
    component := ""
    if len(positionalArgs) >= 2 {
        component = positionalArgs[1]
    }

    return TerraformDestroyOptions{
        GlobalFlags:     ParseGlobalFlags(cmd, v),
        Stack:           v.GetString("stack"),
        DryRun:          v.GetBool("dry-run"),
        Var:             v.GetStringSlice("var"),
        AutoApprove:     v.GetBool("auto-approve"),
        Component:       component,
        PassThroughArgs: passThroughArgs,
    }
}
```

### Packer Example

```go
// pkg/flags/packer_flags.go

// PackerCommonFlags returns flags common to ALL packer subcommands.
func PackerCommonFlags() *FlagRegistry {
    registry := CommonFlags()  // stack, dry-run
    return registry
}

// PackerBuildFlags returns flags specific to `packer build`.
func PackerBuildFlags() *FlagRegistry {
    registry := PackerCommonFlags()

    registry.Register(&BoolFlag{
        Name:        "force",
        Description: "Force build even if artifacts exist",
        Default:     false,
    })

    return registry
}

// PackerBuildCompatibilityAliases - packer's legacy syntax
func PackerBuildCompatibilityAliases() map[string]CompatibilityAlias {
    return map[string]CompatibilityAlias{
        // Pass-through flags (packer's legacy syntax)
        "-var":      {Behavior: MoveToSeparated, Target: ""},
        "-var-file": {Behavior: MoveToSeparated, Target: ""},
        "-only":     {Behavior: MoveToSeparated, Target: ""},
        "-except":   {Behavior: MoveToSeparated, Target: ""},
        "-force":    {Behavior: MoveToSeparated, Target: ""},
        "-on-error": {Behavior: MoveToSeparated, Target: ""},
    }
}

// pkg/flags/packer_build_options.go

type PackerBuildOptions struct {
    GlobalFlags

    Stack  string
    DryRun bool
    Force  bool

    Component       string
    PassThroughArgs []string
}

func ParsePackerBuildFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) PackerBuildOptions {
    component := ""
    if len(positionalArgs) >= 2 {
        component = positionalArgs[1]
    }

    return PackerBuildOptions{
        GlobalFlags:     ParseGlobalFlags(cmd, v),
        Stack:           v.GetString("stack"),
        DryRun:          v.GetBool("dry-run"),
        Force:           v.GetBool("force"),
        Component:       component,
        PassThroughArgs: passThroughArgs,
    }
}
```

### Helmfile Example

```go
// pkg/flags/helmfile_flags.go

// HelmfileCommonFlags returns flags common to ALL helmfile subcommands.
func HelmfileCommonFlags() *FlagRegistry {
    registry := CommonFlags()  // stack, dry-run
    return registry
}

// HelmfileSyncFlags returns flags specific to `helmfile sync`.
func HelmfileSyncFlags() *FlagRegistry {
    registry := HelmfileCommonFlags()

    registry.Register(&BoolFlag{
        Name:        "skip-deps",
        Description: "Skip dependency update",
        Default:     false,
    })

    return registry
}

// HelmfileSyncCompatibilityAliases - helmfile's legacy syntax
func HelmfileSyncCompatibilityAliases() map[string]CompatibilityAlias {
    return map[string]CompatibilityAlias{
        // Pass-through flags (helmfile's legacy syntax)
        "-f":               {Behavior: MoveToSeparated, Target: ""},
        "-e":               {Behavior: MoveToSeparated, Target: ""},
        "-l":               {Behavior: MoveToSeparated, Target: ""},
        "-args":            {Behavior: MoveToSeparated, Target: ""},
        "-values":          {Behavior: MoveToSeparated, Target: ""},
        "-set":             {Behavior: MoveToSeparated, Target: ""},
        "-skip-deps":       {Behavior: MoveToSeparated, Target: ""},
        "-include-crds":    {Behavior: MoveToSeparated, Target: ""},
        "-suppress-secrets": {Behavior: MoveToSeparated, Target: ""},
    }
}

// pkg/flags/helmfile_sync_options.go

type HelmfileSyncOptions struct {
    GlobalFlags

    Stack    string
    DryRun   bool
    SkipDeps bool

    Component       string
    PassThroughArgs []string
}

func ParseHelmfileSyncFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) HelmfileSyncOptions {
    component := ""
    if len(positionalArgs) >= 2 {
        component = positionalArgs[1]
    }

    return HelmfileSyncOptions{
        GlobalFlags:     ParseGlobalFlags(cmd, v),
        Stack:           v.GetString("stack"),
        DryRun:          v.GetBool("dry-run"),
        SkipDeps:        v.GetBool("skip-deps"),
        Component:       component,
        PassThroughArgs: passThroughArgs,
    }
}
```

## Usage in Commands

### Terraform Plan Command

```go
// cmd/terraform/plan.go

var planCmd = &cobra.Command{
    Use:   "plan <component>",
    Short: "Execute terraform plan",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Parse using plan-specific parser
        result, err := parser.Parse(args)
        if err != nil {
            return err
        }

        // Get strongly-typed options
        opts := flags.ParseTerraformPlanFlags(
            cmd,
            viper.GetViper(),
            result.PositionalArgs,
            result.PassThroughArgs,
        )

        // Type-safe access
        if opts.Stack == "" {
            return errors.New("stack is required")
        }

        if opts.UploadStatus {
            // Upload plan to Atmos Pro
        }

        if opts.Out != "" {
            // Atmos will check for planfile
        }

        return execTerraformPlan(opts)
    },
}

var parser *flags.UnifiedParser

func init() {
    // Register only plan-specific flags
    flagRegistry := flags.TerraformPlanFlags()
    flagRegistry.RegisterAll(planCmd)

    // Create parser with plan-specific compatibility aliases
    translator := flags.NewCompatibilityAliasTranslator(
        flags.TerraformPlanCompatibilityAliases(),
    )

    parser = flags.NewUnifiedParser(planCmd, viper.GetViper(), translator)
}
```

### Terraform Apply Command

```go
// cmd/terraform/apply.go

var applyCmd = &cobra.Command{
    Use:   "apply <component>",
    Short: "Execute terraform apply",
    RunE: func(cmd *cobra.Command, args []string) error {
        result, err := parser.Parse(args)
        if err != nil {
            return err
        }

        // Get strongly-typed options (different struct than plan!)
        opts := flags.ParseTerraformApplyFlags(
            cmd,
            viper.GetViper(),
            result.PositionalArgs,
            result.PassThroughArgs,
        )

        // Type-safe access to apply-specific flags
        if opts.AutoApprove {
            // Skip confirmation
        }

        if opts.FromPlan != "" {
            // Apply from saved plan
        }

        return execTerraformApply(opts)
    },
}

var parser *flags.UnifiedParser

func init() {
    // Register only apply-specific flags
    flagRegistry := flags.TerraformApplyFlags()
    flagRegistry.RegisterAll(applyCmd)

    // Create parser with apply-specific compatibility aliases
    translator := flags.NewCompatibilityAliasTranslator(
        flags.TerraformApplyCompatibilityAliases(),
    )

    parser = flags.NewUnifiedParser(applyCmd, viper.GetViper(), translator)
}
```

## Benefits

### 1. Correct Help Text

```bash
$ atmos terraform plan --help
Usage:
  atmos terraform plan <component> [flags]

Flags:
  -s, --stack string         Stack name
      --dry-run              Perform dry run
      --var stringSlice      Set terraform variables
      --out string           Path to save plan file
      --upload-status        Upload plan status to Atmos Pro

# Notice: No --auto-approve (that's apply-only)
```

```bash
$ atmos terraform apply --help
Usage:
  atmos terraform apply <component> [flags]

Flags:
  -s, --stack string         Stack name
      --dry-run              Perform dry run
      --var stringSlice      Set terraform variables
      --auto-approve         Auto-approve terraform apply
      --from-plan string     Apply from saved plan file

# Notice: No --out or --upload-status (those are plan-only)
```

### 2. Proper Validation

```bash
$ atmos terraform plan vpc --auto-approve
Error: unknown flag: --auto-approve

# Correct! auto-approve is apply-only
```

```bash
$ atmos terraform apply vpc --out=plan.tfplan
Error: unknown flag: --out

# Correct! out is plan-only
```

### 3. Type Safety

```go
// Compiler catches errors
func execTerraformPlan(opts flags.TerraformPlanOptions) error {
    if opts.UploadStatus {  // ✅ Valid - plan has this field
        // ...
    }

    if opts.AutoApprove {  // ❌ Compile error - plan doesn't have AutoApprove
        // ...
    }
}

func execTerraformApply(opts flags.TerraformApplyOptions) error {
    if opts.AutoApprove {  // ✅ Valid - apply has this field
        // ...
    }

    if opts.UploadStatus {  // ❌ Compile error - apply doesn't have UploadStatus
        // ...
    }
}
```

### 4. Clear Intent

Code is self-documenting:
- `TerraformPlanFlags()` - explicitly states which flags plan accepts
- `TerraformPlanCompatibilityAliases()` - explicitly states which legacy flags plan handles
- `TerraformPlanOptions` - explicitly types what data plan needs

### 5. Maintainability

Adding new subcommand is straightforward:
1. Create `TerraformXxxFlags()` function
2. Create `TerraformXxxCompatibilityAliases()` function
3. Create `TerraformXxxOptions` struct
4. Create `ParseTerraformXxxFlags()` function
5. Use in command `init()` and `RunE`

## Implementation Plan

### Phase 1.5a: Terraform Subcommands

- [ ] Create `terraform_plan_flags.go` with `TerraformPlanFlags()` and compatibility aliases
- [ ] Create `terraform_plan_options.go` with `TerraformPlanOptions` struct
- [ ] Create `terraform_apply_flags.go` with `TerraformApplyFlags()` and compatibility aliases
- [ ] Create `terraform_apply_options.go` with `TerraformApplyOptions` struct
- [ ] Create `terraform_destroy_flags.go` with `TerraformDestroyFlags()` and compatibility aliases
- [ ] Create `terraform_destroy_options.go` with `TerraformDestroyOptions` struct
- [ ] Repeat for: init, output, workspace, import, state, etc.

### Phase 1.5b: Packer Subcommands

- [ ] Create `packer_build_flags.go` with `PackerBuildFlags()` and compatibility aliases
- [ ] Create `packer_build_options.go` with `PackerBuildOptions` struct
- [ ] Repeat for: validate, inspect, etc.

### Phase 1.5c: Helmfile Subcommands

- [ ] Create `helmfile_sync_flags.go` with `HelmfileSyncFlags()` and compatibility aliases
- [ ] Create `helmfile_sync_options.go` with `HelmfileSyncOptions` struct
- [ ] Repeat for: apply, destroy, diff, etc.

### Phase 1.5d: Deprecate Monolithic Flag Sets

- [ ] Mark `TerraformFlags()` as deprecated
- [ ] Mark `PackerFlags()` as deprecated
- [ ] Mark `HelmfileFlags()` as deprecated
- [ ] Update all commands to use per-subcommand builders

## Testing Strategy

Each subcommand gets its own test suite:

```go
// pkg/flags/terraform_plan_flags_test.go

func TestTerraformPlanFlags(t *testing.T) {
    registry := TerraformPlanFlags()

    // Verify plan-specific flags are registered
    assert.NotNil(t, registry.Get("out"))
    assert.NotNil(t, registry.Get("upload-status"))

    // Verify apply-specific flags are NOT registered
    assert.Nil(t, registry.Get("auto-approve"))
    assert.Nil(t, registry.Get("from-plan"))
}

func TestTerraformPlanCompatibilityAliases(t *testing.T) {
    aliases := TerraformPlanCompatibilityAliases()

    // Verify plan-specific compatibility aliases
    assert.Contains(t, aliases, "-destroy")
    assert.Contains(t, aliases, "-target")

    // Verify behavior
    assert.Equal(t, MoveToSeparated, aliases["-destroy"].Behavior)
    assert.Equal(t, MapToAtmosFlag, aliases["-var"].Behavior)
}

func TestParseTerraformPlanFlags(t *testing.T) {
    // Test type-safe parsing
    cmd := &cobra.Command{}
    v := viper.New()
    v.Set("stack", "dev")
    v.Set("upload-status", true)

    opts := ParseTerraformPlanFlags(cmd, v, []string{"plan", "vpc"}, []string{})

    assert.Equal(t, "dev", opts.Stack)
    assert.True(t, opts.UploadStatus)
    assert.Equal(t, "vpc", opts.Component)
}
```

## Success Criteria

- [ ] `terraform plan --help` shows only plan flags
- [ ] `terraform apply --help` shows only apply flags
- [ ] `terraform plan --auto-approve` errors (unknown flag)
- [ ] `terraform apply --out=plan` errors (unknown flag)
- [ ] Type-safe options structs compile without errors
- [ ] All compatibility aliases work per-subcommand
- [ ] All tests pass
- [ ] Same pattern works for packer and helmfile
- [ ] Code is self-documenting and maintainable

## Related PRDs

- Parent: `unified-flag-parsing-refactoring.md`
- Blocks: Phase 2 (Terraform Integration)
- Blocks: Phase 3 (Packer & Helmfile Integration)
