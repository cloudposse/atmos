# PRD: Unified Flag Parsing System for Atmos

## Problem Statement

Atmos currently has inconsistent flag parsing implementations across different commands:
- **Terraform, Helmfile, Packer**: Custom implementations with `DisableFlagParsing`
- **Auth**: Custom identity flag handling with manual Viper binding
- **Custom commands**: Separate flag processing logic
- **Global flags**: Inconsistent propagation and precedence

**Pain Points**:
1. **Precedence order not enforced consistently**: Flags → ENV vars → config files → defaults must be manually implemented in each command
2. **Duplicated logic**: Flag extraction code repeated across `cmd/`, `internal/exec/`, and `pkg/config/`
3. **Testing difficulty**: Tightly coupled flag parsing and business logic
4. **Double dash handling**: Custom implementations in different places
5. **Global flags**: Don't always work correctly with pass-through commands

### Critical Terraform Flag Parsing Challenges

Terraform (and OpenTofu) use a complex flag syntax that creates significant parsing challenges:

#### 1. **Mixed Single-Dash and Double-Dash Flags**

Terraform uses **both** single-dash and double-dash flags with different semantics:

**Single-dash flags** (POSIX-style):
```bash
terraform plan -var 'foo=bar' -out=plan.tfplan -detailed-exitcode
```

**Double-dash flags** (GNU-style):
```bash
terraform plan --help --version
```

**Problem**: Standard flag parsers often assume one style or the other. Mixing both requires careful handling.

#### 2. **Flags with Optional Values - Core Pattern (NOT an Exception)**

**This is a CONVENTION that must be supported, not an exceptional case.**

Atmos uses Cobra's `NoOptDefVal` pattern for flags that can be used with or without values. This is a first-class feature that the unified parser MUST support.

##### Pattern 1: Boolean Flags with Optional Values

Some flags behave differently based on whether a value is provided:

```bash
# Flag alone = defaults to true
atmos terraform plan --upload-status ...

# Explicit true value
atmos terraform plan --upload-status=true ...

# Explicit false value
atmos terraform plan --upload-status=false ...
```

**Implementation** (from `parseUploadStatusFlag()`):
```go
// Check for --flag (without value, defaults to true)
if u.SliceContainsString(args, "--"+flagName) {
    return true
}

// Check for --flag=value forms
for _, arg := range args {
    if strings.HasPrefix(arg, flagPrefix) {
        value := strings.TrimPrefix(arg, flagPrefix)
        // Parse boolean value, default to true if not a valid boolean
        return value != "false"
    }
}
```

##### Pattern 2: String Flags with Special Default (Identity Pattern)

**The `--identity` flag is the canonical example of this pattern.**

This pattern uses Cobra's `NoOptDefVal` to provide a special default value when the flag is used without an argument:

**From `cmd/auth.go`:**
```go
const (
    IdentityFlagName        = "identity"
    IdentityFlagSelectValue = "__SELECT__" // Special value for interactive selection
)

// Register the flag
authCmd.PersistentFlags().StringP(IdentityFlagName, "i", "",
    "Specify the target identity to assume. Use without value to interactively select.")

// Set NoOptDefVal to enable optional flag value
identityFlag := authCmd.PersistentFlags().Lookup(IdentityFlagName)
if identityFlag != nil {
    identityFlag.NoOptDefVal = IdentityFlagSelectValue
}

// Bind environment variables (but NOT the flag itself - see note below)
viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
```

**Three usage modes:**

1. **Explicit value with equals** - Use the specified identity:
   ```bash
   atmos auth console --identity=prod-admin
   ```

2. **Explicit value with space** - Use the specified identity:
   ```bash
   atmos auth console --identity prod-admin
   ```

3. **Flag alone** - Trigger interactive selection (NoOptDefVal):
   ```bash
   atmos auth console --identity
   # Prompts user to select from available identities
   ```

4. **No flag** - Use default from config/env:
   ```bash
   atmos auth console
   # Uses ATMOS_IDENTITY env var or identity from config
   ```

**Critical**: Both `--identity=value` (equals form) and `--identity value` (space form) MUST be supported. The parser must distinguish:
- `--identity` alone → NoOptDefVal (`__SELECT__`)
- `--identity=foo` → explicit value `foo`
- `--identity foo` → explicit value `foo`
- (no flag) → fallback to env/config

**Precedence handling** (from `cmd/internal/flag/identity.go`):
```go
func GetIdentity(cmd *cobra.Command, viperInstance *viper.Viper) (string, error) {
    // 1. Check if flag was explicitly set (highest priority)
    if cmd.Flags().Changed(IdentityFlagName) {
        flagValue, _ := cmd.Flags().GetString(IdentityFlagName)

        // If flag value is the special select value, trigger interactive selection
        if flagValue == IdentityFlagSelectValue {
            return selectIdentityInteractive()
        }

        return flagValue, nil
    }

    // 2. Fall back to Viper (env var or config)
    return viperInstance.GetString(IdentityFlagName), nil
}
```

**Why this pattern matters:**
- Provides interactive UX when flag is used alone
- Supports explicit values for scripting/CI
- Falls back to config/env for default behavior
- Uses standard Cobra feature (NoOptDefVal)
- Must work with precedence system
- **Must integrate with `viper.BindEnv`** for environment variable fallback

**Critical: Viper Integration**

Note that the code explicitly **does NOT** bind the flag itself to Viper:
```go
// Only bind environment variables, NOT the flag
viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")

// Do NOT do this for flags with NoOptDefVal:
// viper.BindPFlag(IdentityFlagName, cmd.Flags().Lookup(IdentityFlagName))
```

**Why?** Because `viper.BindPFlag` would interfere with NoOptDefVal detection. The precedence checking must be done manually:
1. First check `cmd.Flags().Changed()` to see if flag was explicitly set
2. If flag is set and value equals NoOptDefVal, trigger interactive selection
3. If flag not set, fall back to `viper.Get()` which respects env vars and config

This pattern ensures environment variables work correctly while preserving interactive selection behavior.

##### Pattern 3: Pager Flag (Tri-State Logic)

Similar pattern for pager control (from `cmd/root.go`):

```bash
atmos describe component vpc --pager         # Enable pager
atmos describe component vpc --pager=false   # Disable pager
atmos describe component vpc                 # Use config default
```

##### Convention Requirements

**The unified flag parser MUST support:**

1. **Cobra's `NoOptDefVal` pattern** - Flags with special default values when used alone
2. **Precedence with NoOptDefVal** - Flag present (even with special value) beats env/config
3. **Interactive triggers** - Special values like `__SELECT__` that trigger interactive flows
4. **Boolean optional values** - `--flag` vs `--flag=true` vs `--flag=false`
5. **String optional values** - `--flag` (special default) vs `--flag=value` vs `--flag value` vs (no flag)
6. **Both equals and space forms** - `--flag=value` and `--flag value` must both work correctly
7. **Disambiguation** - Parser must distinguish `--flag` alone from `--flag value` from `--flag=value`
8. **`viper.BindEnv` integration** - Environment variable binding must work correctly with all flag patterns
9. **Selective `BindPFlag`** - Only bind flags to Viper when NOT using NoOptDefVal (to preserve detection)

**Problem**: Standard flag parsers may not handle the interaction between:
- Cobra's `NoOptDefVal` feature
- Viper's precedence system
- Manual flag extraction (in pass-through mode)
- Equals vs space separation with optional values

**Specific challenge with space form + NoOptDefVal:**
```bash
# How does parser know if "foo" is the flag value or next positional arg?
atmos auth console --identity foo --stack prod
                    ^^^^^^^^^ ^^^
                    flag      value? or positional arg?

# With equals form, it's unambiguous:
atmos auth console --identity=foo --stack prod
                    ^^^^^^^^^^^^^
                    clearly flag with value

# Flag alone is unambiguous:
atmos auth console --identity --stack prod
                    ^^^^^^^^^
                    clearly flag without value (NoOptDefVal)
```

The unified parser must handle this ambiguity correctly by checking if the next arg starts with `-` or is a known positional arg position.

#### 3. **Positional Arguments vs. Optional Flag Values**

Some Terraform commands accept positional arguments AND flags with optional values, creating ambiguity:

```bash
# Positional argument (resource address)
terraform import aws_instance.example i-1234567890abcdef0 -var-file=vars.tfvars

# vs. flag with optional value?
terraform plan -out planfile  # Is "planfile" the value or a positional arg?
```

**Problem**: When parsing `plan -out planfile`, how do you know if `planfile` is:
- The value for `-out` flag?
- A positional argument that follows `-out` flag?

#### 4. **Flags That Accept Multiple Values**

Some flags can be repeated:

```bash
terraform plan -var 'foo=bar' -var 'baz=qux' -target=module.vpc -target=module.eks
```

**Problem**: Must handle flag repetition and accumulate values.

#### 5. **Flags with Equals vs. Space Separation**

Terraform accepts both forms:

```bash
terraform plan -var-file=prod.tfvars    # equals form
terraform plan -var-file prod.tfvars    # space form
terraform plan -var 'foo=bar'           # quotes when value contains =
```

**Problem**: Parser must handle both `-flag=value` and `-flag value` forms, AND handle values that themselves contain `=`.

#### 6. **Three Concurrent Flag Types in Same Command**

**The fundamental challenge**: Atmos must support **THREE different types of flags/args in a single command invocation**:

1. **Atmos-style flags** (double-dash, GNU-style): `--stack`, `--dry-run`, `--identity`
2. **Terraform-style flags** (single-dash, POSIX-style): `-var`, `-out`, `-var-file`
3. **Positional arguments**: component name, subcommand, resource addresses

All three can appear **concurrently** in the same command, in various orders:

```bash
# All three types mixed together:
atmos terraform plan vpc --stack prod -var 'env=prod' --dry-run -out=plan.tfplan
                    ^^^  ^^^^^^^^^^^^ ^^^^^^^^^^^^^^^^ ^^^^^^^^^ ^^^^^^^^^^^^^^^^^
                    pos  Atmos        Terraform         Atmos     Terraform

# With double-dash separator (explicit):
atmos terraform plan vpc --stack prod --dry-run -- -var 'env=prod' -out=plan.tfplan
                    ^^^  ^^^^^^^^^^^^ ^^^^^^^^^    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                    pos  Atmos        Atmos        Terraform (after --)

# Without double-dash (implicit - backward compat):
atmos terraform plan vpc -s prod --dry-run -var 'env=prod' -out=plan.tfplan
                    ^^^  ^^^^^^^^ ^^^^^^^^^  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                    pos  Atmos    Atmos      Terraform (must extract Atmos, pass rest)

# Complex real-world example:
atmos terraform plan vpc \
    --stack prod \              # Atmos flag
    --identity admin \          # Atmos flag
    --dry-run \                 # Atmos flag
    -var 'region=us-east-1' \   # Terraform flag
    -var-file=common.tfvars \   # Terraform flag
    -- \                        # Separator
    -var 'override=true' \      # Terraform flag (after separator)
    -out=plan.tfplan            # Terraform flag (after separator)
```

**Current implementation**:
- Uses `DisableFlagParsing = true` on Terraform command
- Manually extracts args with `extractTrailingArgs()` and `processArgsAndFlags()`
- Removes known Atmos flags from the list (via `commonFlags` list)
- Passes remaining args to Terraform

**Problem**:
- Manual parsing is error-prone
- Hard to test
- Doesn't leverage Cobra/pflag benefits
- Difficult to add new Atmos flags without breaking Terraform flags

#### 7. **Positional Arguments Before and After Flags**

Terraform commands have varying argument structures:

```bash
# Subcommand only
terraform plan

# Subcommand + flags
terraform plan -out=planfile

# Subcommand + sub-subcommand + args
terraform workspace select prod

# Import: command + address + id + flags
terraform import aws_instance.example i-1234567890abcdef0 -var-file=vars.tfvars
```

**Problem**: Parser needs to:
- Identify subcommand (`plan`, `apply`, `import`, etc.)
- Identify sub-subcommand (`workspace select`, `workspace new`, etc.)
- Extract positional args (resource address, workspace name, etc.)
- Extract flags
- Preserve order for Terraform execution

#### 8. **Ambiguity When Positional Args Look Like Flags**

Edge case that can break parsers:

```bash
# Component named "-s" or starting with dash?
atmos terraform plan -s -s prod  # First -s is flag, second is value, third is component?

# Workspace named "--help"?
terraform workspace select --help  # Show help or select workspace named "--help"?
```

**Current mitigation**: Use `--` separator to avoid ambiguity:
```bash
atmos terraform plan -- <ambiguous-component-name> -s prod
```

#### 9. **Atmos-Specific Flags That Don't Exist in Terraform**

Atmos adds flags that Terraform doesn't understand:

```bash
--upload-status       # Atmos-only: upload plan status to Atmos Pro
--skip-init           # Atmos-only: don't run terraform init
--dry-run             # Atmos-only: show what would be executed
--from-plan           # Atmos-only: apply from previously generated plan
--identity            # Atmos-only: authentication identity
```

**Problem**: These must be:
- Parsed by Atmos
- Removed from args before passing to Terraform
- Available in all Terraform subcommands
- Work correctly with or without `--` separator

### Summary of Parsing Challenges

| Challenge | Current Solution | Problems |
|-----------|------------------|----------|
| Mixed single/double dash | `DisableFlagParsing=true` + manual parsing | Error-prone, not using Cobra |
| Optional flag values | Custom `parseUploadStatusFlag()` | Duplicated logic |
| Positional args ambiguity | Manual arg list processing | Hard to extend |
| Multiple flag values | Manual accumulation | Not leveraging pflag |
| Equals vs. space | String manipulation | Fragile |
| Double dash separator | `extractTrailingArgs()` + manual split | Custom implementation |
| Mixed Atmos/Terraform flags | `commonFlags` list + removal | Must maintain list manually |
| Positional args ordering | Manual slice manipulation | Complex logic |
| Ambiguous args | `--` separator | User must know to use it |
| Atmos-specific flags | `commonFlags` list | Must update in multiple places |

**Key Insight**: The fundamental challenge is that **Atmos acts as a wrapper CLI that needs to parse its own flags while preserving Terraform's complex flag syntax**. This is why `DisableFlagParsing=true` is currently used - but it forces manual parsing of everything.

### The Three-Way Parsing Requirement

**Critical requirement**: The unified flag parsing system MUST support **all three types concurrently** in a single command:

#### 1. Atmos-Style Flags (Double-Dash, GNU-style)

```bash
--stack prod
--dry-run
--identity admin
--skip-init
--upload-status=false
-s prod                # shorthand for --stack
-i admin               # shorthand for --identity
```

**Characteristics**:
- Double-dash prefix (`--flag`)
- Optional shorthand single-letter form (`-s` for `--stack`)
- Values separated by space or equals (`--stack prod` or `--stack=prod`)
- Boolean flags may have optional values (`--upload-status` or `--upload-status=false`)

#### 2. Terraform-Style Flags (Single-Dash, POSIX-style)

```bash
-var 'foo=bar'
-var-file=prod.tfvars
-out=plan.tfplan
-target=module.vpc
-auto-approve
-detailed-exitcode
```

**Characteristics**:
- Single-dash prefix (`-flag`)
- Values separated by space or equals (`-out plan` or `-out=plan`)
- Can be repeated (`-var 'x=1' -var 'y=2'`)
- Values may contain special characters requiring quotes (`-var 'key=val'`)

#### 3. Positional Arguments

```bash
plan                   # subcommand
vpc                    # component name
workspace select prod  # subcommand + argument
import aws_instance.example i-abc123  # command + multiple positional args
```

**Characteristics**:
- No prefix
- Position-dependent meaning
- May appear before, after, or between flags
- Can be ambiguous (is `-s` a flag or an argument?)

#### Concurrent Usage Patterns

**Pattern 1: Interleaved (most complex)**
```bash
atmos terraform plan vpc --stack prod -var 'x=1' --dry-run -out=plan.tfplan
# Parser must:
# 1. Recognize 'plan' as subcommand
# 2. Recognize 'vpc' as component (positional)
# 3. Extract --stack and --dry-run as Atmos flags
# 4. Preserve -var and -out as Terraform flags
# 5. Maintain order for Terraform execution
```

**Pattern 2: Explicit separation with --**
```bash
atmos terraform plan vpc --stack prod --dry-run -- -var 'x=1' -out=plan.tfplan
# Everything before -- is parsed (Atmos + positional)
# Everything after -- goes to Terraform unchanged
```

**Pattern 3: Atmos flags first (cleanest)**
```bash
atmos terraform --stack prod --dry-run plan vpc -var 'x=1' -out=plan.tfplan
# Atmos global flags before subcommand
# Terraform flags after component
```

**Pattern 4: Mixed with shorthand**
```bash
atmos terraform plan vpc -s prod -var 'x=1' --identity admin -out=plan.tfplan
# -s is Atmos shorthand (--stack)
# -var is Terraform flag
# --identity is Atmos flag
# -out is Terraform flag
```

#### Why This Is Hard

Standard flag parsers (including Cobra/pflag) **cannot** handle this because:

1. **Ambiguous prefixes**: Both Atmos and Terraform use `-` and `--` prefixes
2. **Unknown flags**: Parser doesn't know which flags belong to which system
3. **Order preservation**: Must preserve Terraform flag order exactly
4. **Value extraction**: Must distinguish `-s` (Atmos shorthand) from `-s` (Terraform flag) from `s` (positional arg)
5. **Quoting**: Must preserve quotes in Terraform flags (`-var 'foo=bar'`)

**Example of ambiguity**:
```bash
atmos terraform plan -s prod -var 'stack=prod'
                     ^^^^^^^ ^^^^^^^^^^^^^^^^^
                     Atmos   Terraform

# How does parser know?
# -s could be: Atmos --stack shorthand, OR Terraform flag, OR component name starting with -s
# Solution: Maintain a registry of known Atmos flags
```

### Requirements for Unified Parser

1. **Must parse Atmos flags** (double-dash style) and extract their values
2. **Must recognize Atmos shorthand flags** (`-s` → `--stack`, `-i` → `--identity`)
3. **Must NOT parse Terraform flags** (preserve them exactly as-is)
4. **Must extract positional arguments** (component, subcommand)
5. **Must support `--` separator** for explicit separation
6. **Must support implicit mode** (no separator, backward compatible)
7. **Must preserve order** of Terraform flags for execution
8. **Must preserve quoting** of Terraform flag values
9. **Must handle optional boolean values** (`--upload-status` vs `--upload-status=false`)
10. **Must support all three types concurrently** in any order (with implicit mode)

## Goals

1. **Single flag parsing pass**: All flags processed through one unified system
2. **Consistent precedence**: Flags > ENV vars > config files > defaults enforced automatically
3. **Interface-driven**: Enable mocking and unit testing (target 80-90% coverage)
4. **Preserve Cobra/Viper**: Augment, don't replace - maintain backward compatibility
5. **Double dash support**: Clean separation of Atmos flags from tool-specific flags
6. **Global flags**: Work consistently across all commands including pass-through

## Non-Goals

- Replacing Cobra or Viper with custom implementations
- Breaking existing CLI interfaces or config files
- Changing user-facing command syntax

## Critical Integration Requirements

### 1. Log Level Initialization Fix

**Current Issue** (from `pkg/config/config.go:64-79`):
```go
func setLogConfig(atmosConfig *schema.AtmosConfiguration) {
    // TODO: This is a quick patch to mitigate the issue
    // Issue: https://linear.app/cloudposse/issue/DEV-3093/create-a-cli-command-core-library
    if os.Getenv("ATMOS_LOGS_LEVEL") != "" {
        atmosConfig.Logs.Level = os.Getenv("ATMOS_LOGS_LEVEL")
    }
    flagKeyValue := parseFlags()  // Manual flag parsing!
    if v, ok := flagKeyValue["logs-level"]; ok {
        atmosConfig.Logs.Level = v
    }
    // ...
}
```

**Problems**:
1. Logger is initialized **before** config is loaded
2. Log level can be set from: flag, env var, or config file
3. Currently uses **manual flag parsing** (`parseFlags()`) to get log level early
4. Precedence order must be: `--logs-level` flag > `ATMOS_LOGS_LEVEL` > config file > default
5. Similar issues with `--logs-file`, `--no-color`, `--pager`

**Requirement**: Unified flag parser MUST support **early extraction** of log configuration flags before full config loading, so logger can be initialized with correct level.

### 2. TestKit Integration for Isolated Testing

**Current TestKit** (`cmd/testkit_test.go`):
```go
// TestKit wraps testing.TB and provides automatic RootCmd state cleanup
type TestKit struct {
    testing.TB
}

func NewTestKit(tb testing.TB) *TestKit {
    // Snapshots RootCmd state and registers cleanup
    snapshot := snapshotRootCmdState()
    tb.Cleanup(func() {
        restoreRootCmdState(snapshot)
    })
    return &TestKit{TB: tb}
}
```

**What it does**:
- Automatically snapshots and restores `RootCmd` state
- Prevents test pollution from global state
- Works with subtests and table-driven tests
- Restores `os.Args`

**Requirement**: Flag parser tests MUST use TestKit to ensure:
1. No pollution between tests
2. Clean sandbox environments for argument parsing
3. Proper cleanup of global flag state
4. Isolation when testing with mock component

### 3. Command Registry Integration

**Current Command Registry Pattern** (`cmd/internal/registry.go`):
- Commands implement `CommandProvider` interface
- Self-register via `internal.Register()` in `init()`
- Provides `GetCommand()` that returns fully configured `*cobra.Command`
- Commands organized in subdirectories (e.g., `cmd/about/`, `cmd/auth/`)

**Example CommandProvider (current pattern - version command):**
```go
package version

var (
    checkFlag     bool
    versionFormat string
)

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Display the version of Atmos",
    RunE: func(c *cobra.Command, args []string) error {
        return exec.NewVersionExec(atmosConfigPtr).Execute(checkFlag, versionFormat)
    },
}

func init() {
    // Register flags using Cobra's built-in methods
    versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks")
    versionCmd.Flags().StringVar(&versionFormat, "format", "", "Specify output format")

    // Register with command registry
    internal.Register(&VersionCommandProvider{})
}

type VersionCommandProvider struct{}

func (v *VersionCommandProvider) GetCommand() *cobra.Command {
    return versionCmd
}

func (v *VersionCommandProvider) GetName() string {
    return "version"
}

func (v *VersionCommandProvider) GetGroup() string {
    return "Other Commands"
}
```

**Current limitations:**
- Flags stored in package-level variables (hard to test)
- No automatic precedence handling (flags don't check env vars or config)
- No Viper integration
- Manual flag registration with `Flags().BoolVarP()`, `Flags().StringVar()`, etc.
- No support for NoOptDefVal pattern

**Requirement**: Commands using the command registry MUST be able to easily integrate with the unified flag parser.

**How flag parser integrates with command registry:**

```go
package terraform

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
    "github.com/cloudposse/atmos/pkg/flagparser"
)

// TerraformCommandProvider implements CommandProvider.
type TerraformCommandProvider struct {
    parser flagparser.PassThroughFlagParser
    loader config.ConfigLoader
}

func init() {
    // Create provider with dependencies
    provider := &TerraformCommandProvider{
        parser: flagparser.NewPassThroughFlagParser(
            flagparser.WithAtmosFlags("stack", "dry-run", "identity", "upload-status"),
            flagparser.WithOptionalBoolFlags("upload-status"),
        ),
        loader: config.NewViperLoader(),
    }

    // Register with command registry
    internal.Register(provider)
}

func (t *TerraformCommandProvider) GetCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "terraform [command] [component] -- [terraform flags]",
        Short: "Execute Terraform commands",
    }

    // Register flags using unified parser
    t.parser.RegisterFlags(cmd)

    // Bind to Viper
    t.parser.BindToViper(t.loader.Viper())

    // Middleware chain
    cmd.PersistentPreRunE = middleware.ComposeMiddleware(
        middleware.ConfigMiddleware(t.loader),
        middleware.AuthMiddleware(t.authManager),
    )

    // Business logic
    cmd.RunE = func(cmd *cobra.Command, args []string) error {
        // Parser extracts Atmos flags, passes rest to Terraform
        cfg, err := t.parser.Parse(cmd.Context(), args)
        if err != nil {
            return err
        }

        return t.executor.Execute(cmd.Context(), cfg)
    }

    return cmd
}

func (t *TerraformCommandProvider) GetName() string {
    return "terraform"
}

func (t *TerraformCommandProvider) GetGroup() string {
    return "Core Stack Commands"
}
```

**Key benefits for command registry users:**

1. **Clean dependency injection** - Parser and loader injected in provider constructor
2. **Self-contained** - Each command owns its flag configuration
3. **Testable** - Provider can be instantiated with mock parser/loader
4. **No global state** - Each provider has its own parser instance
5. **Follows existing pattern** - Minimal changes to CommandProvider interface
6. **Easy to use** - Three steps: create parser, register flags, bind to Viper

**Pattern for standard commands:**
```go
func NewDescribeCommandProvider(
    parser flagparser.FlagParser,
    loader config.ConfigLoader,
) *DescribeCommandProvider {
    return &DescribeCommandProvider{
        parser: parser,
        loader: loader,
    }
}

func (d *DescribeCommandProvider) GetCommand() *cobra.Command {
    cmd := &cobra.Command{...}

    // 1. Register flags
    d.parser.RegisterFlags(cmd)

    // 2. Bind to Viper
    d.parser.BindToViper(d.loader.Viper())

    // 3. Use middleware
    cmd.PersistentPreRunE = middleware.ConfigMiddleware(d.loader)

    return cmd
}
```

**No changes needed to CommandProvider interface** - the integration happens inside `GetCommand()`.

#### Before/After: Version Command Migration

**BEFORE (current):**
```go
package version

var (
    checkFlag     bool
    versionFormat string
)

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Display the version of Atmos",
    RunE: func(c *cobra.Command, args []string) error {
        return exec.NewVersionExec(atmosConfigPtr).Execute(checkFlag, versionFormat)
    },
}

func init() {
    // Manual flag registration
    versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks")
    versionCmd.Flags().StringVar(&versionFormat, "format", "", "Specify output format")

    internal.Register(&VersionCommandProvider{})
}
```

**Issues:**
- Package-level variables make testing difficult
- No env var support (can't set `ATMOS_VERSION_CHECK=true`)
- No config file support
- No precedence enforcement

**AFTER (with unified parser):**
```go
package version

import (
    "github.com/cloudposse/atmos/pkg/flagparser"
    "github.com/cloudposse/atmos/pkg/config"
)

type VersionCommandProvider struct {
    parser flagparser.FlagParser
    loader config.ConfigLoader
}

func init() {
    // Create parser with version-specific flags
    parser := flagparser.NewStandardFlagParser(
        flagparser.WithBoolFlag("check", "c", false, "Run additional checks"),
        flagparser.WithStringFlag("format", "", "", "Specify output format"),
    )

    provider := &VersionCommandProvider{
        parser: parser,
        loader: config.NewViperLoader(),
    }

    internal.Register(provider)
}

func (v *VersionCommandProvider) GetCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "version",
        Short: "Display the version of Atmos",
        RunE: func(c *cobra.Command, args []string) error {
            // Get values from Viper (respects precedence)
            checkFlag := v.loader.Viper().GetBool("check")
            format := v.loader.Viper().GetString("format")

            return exec.NewVersionExec(atmosConfigPtr).Execute(checkFlag, format)
        },
    }

    // Register flags
    v.parser.RegisterFlags(cmd)

    // Bind to Viper with env var support
    v.parser.BindToViper(v.loader.Viper())
    v.loader.Viper().BindEnv("check", "ATMOS_VERSION_CHECK")
    v.loader.Viper().BindEnv("format", "ATMOS_VERSION_FORMAT")

    return cmd
}
```

**Benefits:**
- ✅ Testable (inject mock parser/loader)
- ✅ Environment variables work: `ATMOS_VERSION_CHECK=true atmos version`
- ✅ Config file support: Set defaults in `atmos.yaml`
- ✅ Precedence enforced: flag > env > config > default
- ✅ No package-level variables
- ✅ Standard pattern across all commands

### 4. Custom Command Integration

**Current Custom Commands** (`cmd/custom/`):
- Dynamically loaded from atmos.yaml configuration
- Support arbitrary flags defined in config
- Can specify flag types (bool, string, int), defaults, required status
- Must work with `--identity` flag for authentication
- Must support both equals and space forms for all flag types

**Example custom command config:**
```yaml
commands:
  - name: deploy-app
    description: Deploy application
    flags:
      - name: environment
        shorthand: e
        type: string
        required: true
        description: Target environment
      - name: dry-run
        type: bool
        default: false
        description: Perform dry run
      - name: identity
        type: string
        usage: Specify identity (use alone to select interactively)
        # Must support NoOptDefVal pattern for interactive selection
```

**Requirement**: Custom commands MUST support the same flag patterns as built-in commands:

1. **NoOptDefVal pattern** - Custom string flags can specify a special default value for flag-alone usage
2. **Equals and space forms** - `--environment=prod` and `--environment prod` both work
3. **Identity flag integration** - `--identity` works with custom commands using same pattern
4. **Shorthand flags** - `-e prod` and `-e=prod` both work
5. **Boolean optional values** - `--dry-run`, `--dry-run=true`, `--dry-run=false`
6. **Precedence order** - Custom command flags follow same precedence (flag > env > config > default)
7. **Dynamic flag registration** - Parser must handle flags defined in config, not hardcoded

**Challenge**: Custom commands are defined in YAML config, not Go code. The parser must:
- Dynamically create flags based on config
- Support NoOptDefVal for string flags when specified in config
- Integrate with identity flag pattern
- Validate flag types and required flags
- Work with component registry pattern

### 4. Mock Component for Comprehensive Edge Case Testing

**Mock Component** (`pkg/component/mock/`):
- Implements `component.Provider` interface
- Self-registers with component registry
- Used for testing without external dependencies (no Terraform/Helmfile/etc.)
- Supports validation, execution, artifact generation

**Requirement**: Flag parser MUST be tested with mock component to ensure it works correctly with component registry pattern. Tests should include:

1. **Every possible exception scenario**:
   - Missing flags
   - Invalid flag values
   - Conflicting flags
   - Flags in wrong order
   - Empty values
   - Special characters
   - Unicode in values
   - Very long argument lists
   - Nested quotes
   - Escape sequences

2. **Component registry integration**:
   - Component type detection
   - Component-specific flag handling
   - Mock component execution with flags
   - Validation with various flag combinations

3. **Real pain-in-the-butt edge cases**:
   - Component name that looks like a flag (`-s`, `--help`)
   - Stack name with special chars (`prod/us-east-1`, `staging:v2`)
   - Flags that appear multiple times
   - Values that contain `=`, `-`, `--`
   - Mixed single/double quotes
   - Trailing/leading whitespace
   - Flag values that are file paths with spaces
   - Flag values that are JSON/YAML strings

4. **Custom command integration tests**:
   - Custom command with identity flag using NoOptDefVal
   - Custom command with 10+ dynamic flags
   - Custom command with required flags (validation)
   - Custom command with boolean optional values
   - Custom command mixing equals and space forms
   - Custom command with shorthand flags
   - Custom command with flags defined in config but overridden by env/CLI
   - Custom command with precedence order (flag > env > config > default from YAML)

## Success Criteria

- [ ] All commands use unified flag parsing system
- [ ] 80-90% test coverage for flag parsing logic
- [ ] No duplicated flag extraction code
- [ ] Precedence order enforced consistently
- [ ] Global flags work in all commands
- [ ] Double dash separator works uniformly
- [ ] Backward compatible with existing usage

## Design Principles

### 1. Single Source of Truth (Viper)

All configuration reads go through Viper, which maintains precedence automatically:
```
CLI flags > Environment variables > Config files > Defaults
```

### 2. Two-Phase Parsing for Wrapper Commands

For Terraform/Helmfile/Packer, use a **two-phase approach**:

**Phase 1: Extract Atmos Flags** (before `--` or from known Atmos flags)
- Use custom parser that understands Atmos-specific flags
- Extract and validate Atmos flags (`--stack`, `--dry-run`, `--identity`, etc.)
- Leave everything else untouched for the underlying tool

**Phase 2: Pass-Through to Tool**
- Pass remaining args directly to Terraform/Helmfile/Packer
- No parsing or modification
- Preserve all flag syntax, ordering, quoting

**Example**:
```bash
# Input
atmos terraform plan vpc -s prod --dry-run --upload-status=false -- -var-file=prod.tfvars -out=plan.tfplan

# Phase 1: Extract Atmos flags
atmosFlags: {
    component: "vpc",
    stack: "prod",
    dryRun: true,
    uploadStatus: false
}

# Phase 2: Pass to Terraform
terraformArgs: ["plan", "-var-file=prod.tfvars", "-out=plan.tfplan"]
```

### 3. Interface-Driven Architecture

```go
// FlagParser handles flag parsing for a command
type FlagParser interface {
    // Parse processes args and returns parsed config
    Parse(ctx context.Context, args []string) (*ParsedConfig, error)

    // RegisterFlags adds flags to the command
    RegisterFlags(cmd *cobra.Command)

    // BindToViper binds flags to Viper keys
    BindToViper(v *viper.Viper) error
}

// ConfigLoader loads configuration with proper precedence
type ConfigLoader interface {
    // Load reads config from all sources with precedence
    Load(ctx context.Context, opts ...LoadOption) (*Config, error)

    // Reload refreshes configuration
    Reload(ctx context.Context) error
}

// PassThroughHandler separates Atmos flags from tool flags
type PassThroughHandler interface {
    // SplitAtDoubleDash separates args at -- separator
    SplitAtDoubleDash(args []string) (beforeDash, afterDash []string)

    // ExtractAtmosFlags pulls out known Atmos flags from args
    // Returns: atmosFlags, remainingArgs, error
    ExtractAtmosFlags(args []string) (map[string]interface{}, []string, error)

    // ExtractPositionalArgs identifies positional arguments
    // (component name, subcommand, etc.) from arg list
    ExtractPositionalArgs(args []string, expectedCount int) ([]string, []string, error)
}

// OptionalBoolFlag handles flags that can be:
// --flag (defaults to true), --flag=true, --flag=false
type OptionalBoolFlag interface {
    // Parse returns the boolean value and whether flag was present
    Parse(args []string, flagName string) (value bool, present bool, error)

    // Remove removes all instances of the flag from args
    Remove(args []string, flagName string) []string
}
```

### 4. Middleware Pattern

Use Cobra hooks for configuration pipeline:
```go
PersistentPreRunE → Load Config → Bind Flags → Validate → Execute
```

### 5. Dependency Injection

Commands receive dependencies via constructors:
```go
func NewTerraformCmd(
    configLoader ConfigLoader,
    flagParser PassThroughFlagParser,
    executor TerraformExecutor,
) *cobra.Command
```

### 6. Explicit vs. Implicit Pass-Through

Support both explicit (`--` separator) and implicit (flag recognition) modes:

**Explicit mode** (recommended):
```bash
atmos terraform plan vpc -s prod -- -var-file=prod.tfvars
# Everything after -- goes to Terraform unchanged
```

**Implicit mode** (legacy support):
```bash
atmos terraform plan vpc -s prod -var-file=prod.tfvars
# Parser extracts known Atmos flags, passes rest to Terraform
```

**Design decision**: Encourage `--` separator in documentation, but maintain backward compatibility with implicit mode.

## Strongly-Typed Command Interpreters

### Design Philosophy

Instead of weakly-typed string-based flag access (like Cobra's `GetString("flag")`), we use **strongly-typed interpreters** (like Viper's schema-first approach). Each command type has its own interpreter struct with properly typed fields.

**See also**:
- `docs/prd/flag-handling/global-flags-pattern.md` - Handling global/persistent flags via struct embedding
- `docs/prd/flag-handling/global-flags-examples.md` - Real examples with `--logs-level` and `--identity`
- `docs/prd/flag-handling/default-values-pattern.md` - Default value handling across all layers

### Why Strongly Typed?

**Problem with weak typing (Cobra-style):**
```go
stack := interpreter.GetString("stack")        // ❌ Runtime error if typo
dryRun := interpreter.GetBool("dry-run")       // ❌ No autocomplete
identity := interpreter.Flags["identity"].(string) // ❌ Type assertion
```

**Solution with strong typing (Viper-style):**
```go
stack := interpreter.Stack                     // ✅ Compile-time safety
dryRun := interpreter.DryRun                   // ✅ IDE autocomplete
identity := interpreter.Identity.Value()       // ✅ No assertions
```

### Interface Design

```go
// CommandInterpreter is the base interface all interpreters implement.
type CommandInterpreter interface {
    GetGlobalFlags() *GlobalFlags
    GetPositionalArgs() []string
    GetPassThroughArgs() []string
}

// GlobalFlags contains all persistent flags available to every command.
// These are inherited from RootCmd.PersistentFlags() and embedded in all interpreters.
// See docs/prd/global-flags-pattern.md for detailed design.
type GlobalFlags struct {
    Chdir      string
    BasePath   string
    Config     []string
    ConfigPath []string
    LogsLevel  string
    LogsFile   string
    NoColor    bool
    Pager      PagerSelector
    // ... 13+ global flags
}
```

### Command-Specific Interpreters

Each command defines its own strongly-typed interpreter:

#### Terraform Interpreter
```go
type TerraformInterpreter struct {
    GlobalFlags  // Embedded: provides all 13+ global flags (logs-level, no-color, pager, etc.)

    // All values have precedence resolved: CLI > ENV > config > defaults
    Stack        string
    Identity     IdentitySelector  // Special type for identity semantics
    DryRun       bool
    SkipInit     bool
    FromPlan     string
    UploadStatus bool

    // Parsed structure
    Subcommand string
    Component  string

    positionalArgs  []string
    passThroughArgs []string
}

// Usage examples:
// interpreter.LogsLevel    // ✅ From GlobalFlags (embedded)
// interpreter.NoColor      // ✅ From GlobalFlags (embedded)
// interpreter.Stack        // ✅ From TerraformInterpreter
// interpreter.DryRun       // ✅ From TerraformInterpreter
```

#### Helmfile Interpreter
```go
type HelmfileInterpreter struct {
    GlobalFlags  // Embedded: same global flags as Terraform

    Stack     string
    Identity  IdentitySelector
    DryRun    bool
    Component string

    positionalArgs  []string
    passThroughArgs []string
}

// Usage examples:
// interpreter.LogsLevel    // ✅ From GlobalFlags
// interpreter.Pager        // ✅ From GlobalFlags
// interpreter.Stack        // ✅ From HelmfileInterpreter
```

#### Custom Command Interpreter
For custom commands where schema isn't known at compile time:

```go
type CustomCommandInterpreter struct {
    GlobalFlags  // Embedded: same global flags

    // Generic map for dynamic flags from atmos.yaml
    Flags map[string]interface{}

    positionalArgs  []string
    passThroughArgs []string
}

// Convenience methods
func (c *CustomCommandInterpreter) GetString(key string) string
func (c *CustomCommandInterpreter) GetBool(key string) bool
func (c *CustomCommandInterpreter) Has(key string) bool

// Usage examples:
// interpreter.LogsLevel            // ✅ From GlobalFlags
// interpreter.Flags["environment"] // ✅ From custom YAML config
```

### Identity Selector Type

Identity flag has special semantics (interactive selection), modeled as a special type:

```go
// IdentitySelector represents the state of the identity flag.
type IdentitySelector struct {
    value    string
    provided bool
}

// IsInteractiveSelector returns true if --identity was used without a value.
func (i IdentitySelector) IsInteractiveSelector() bool {
    return i.value == cfg.IdentityFlagSelectValue // "__SELECT__"
}

// Value returns the identity value.
func (i IdentitySelector) Value() string {
    return i.value
}

// IsEmpty returns true if no identity was provided.
func (i IdentitySelector) IsEmpty() bool {
    return i.value == ""
}
```

### Universal Precedence Resolution

**Key Principle**: Precedence (CLI > ENV > config > defaults) is resolved ONCE, universally, by the parser:

```go
// parseWithPrecedence is the universal precedence resolver.
// ALL parsers use this to resolve flag values with proper precedence.
func (p *baseParser) parseWithPrecedence(registry *FlagRegistry, viper *viper.Viper) map[string]interface{} {
    resolved := make(map[string]interface{})

    if viper == nil {
        return resolved
    }

    // Viper handles precedence automatically
    // We just need type-specific getters for proper conversion
    for _, flag := range registry.All() {
        flagName := flag.GetName()
        viperKey := getViperKey(flagName)

        if viper.IsSet(viperKey) {
            switch flag.(type) {
            case *BoolFlag:
                resolved[flagName] = viper.GetBool(viperKey)
            case *IntFlag:
                resolved[flagName] = viper.GetInt(viperKey)
            case *StringFlag:
                resolved[flagName] = viper.GetString(viperKey)
            }
        }
    }

    return resolved
}
```

### Parser Implementation

Parsers resolve precedence and return strongly-typed interpreters:

```go
type TerraformParser struct {
    registry *FlagRegistry
    viper    *viper.Viper
}

func (p *TerraformParser) Parse(ctx context.Context, args []string) (*TerraformInterpreter, error) {
    // Step 1: Extract CLI flags and args
    positionals, passthrough, err := p.extractArgs(args)
    if err != nil {
        return nil, err
    }

    // Step 2: UNIVERSAL PRECEDENCE RESOLUTION
    // This is the ONLY place precedence is handled
    resolvedFlags := p.parseWithPrecedence(p.registry, p.viper)

    // Step 3: Build strongly-typed interpreter
    return NewTerraformInterpreter(resolvedFlags, positionals, passthrough), nil
}
```

### Usage Examples

```go
// cmd/terraform_utils.go
func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
    // Parser resolves precedence and returns strongly-typed interpreter
    interpreter, err := terraformParser.Parse(ctx, args)
    if err != nil {
        return err
    }

    // ✅ Strongly typed - no GetString() calls!
    // ✅ Precedence already applied by parser
    // ✅ IDE autocomplete works
    info.Stack = interpreter.Stack
    info.DryRun = interpreter.DryRun

    // ✅ Clear identity handling
    if interpreter.Identity.IsInteractiveSelector() {
        handleInteractiveIdentitySelection(&info)
    } else if !interpreter.Identity.IsEmpty() {
        info.Identity = interpreter.Identity.Value()
    }
}

// cmd/helmfile.go
func helmfileRun(cmd *cobra.Command, commandName string, args []string) error {
    interpreter, err := helmfileParser.Parse(ctx, args)

    // ✅ Different type, different structure, same precedence!
    info.Stack = interpreter.Stack
    info.DryRun = interpreter.DryRun
}
```

### Benefits

1. **Type Safety**: Compiler catches typos and type errors
2. **IDE Support**: Autocomplete shows available fields
3. **Self-Documenting**: Interpreter struct shows exactly what's available
4. **No Magic Strings**: `interpreter.Stack` not `interpreter.GetString("stack")`
5. **Refactoring**: Rename fields and compiler finds all usages
6. **Testing**: Easy to mock - `&TerraformInterpreter{Stack: "test"}`
7. **Precedence Transparency**: Users don't think about precedence, it just works

## Rollout Difficulty Assessment

### Overview

The strongly-typed interpreter design is a **significant architectural change** but can be implemented **incrementally** without breaking existing functionality. Below is a detailed analysis of effort, risks, and migration strategy.

### Effort Estimation

| Phase | Scope | Estimated Effort | Complexity |
|-------|-------|------------------|------------|
| **Phase 1: Core Infrastructure** | Create interpreter interfaces and base parser | 3-4 days | Medium |
| **Phase 2: First Command Migration** | Terraform command (most complex) | 5-7 days | High |
| **Phase 3: Remaining Pass-Through** | Helmfile, Packer commands | 2-3 days | Medium |
| **Phase 4: Standard Commands** | Describe, Validate, Workflow, etc. (8-10 commands) | 4-5 days | Low-Medium |
| **Phase 5: Custom Commands** | Dynamic interpreter for custom commands | 3-4 days | Medium |
| **Phase 6: Testing & Cleanup** | Comprehensive tests, remove old code | 3-4 days | Medium |
| **Total** | | **20-27 days** (~4-5 weeks) | |

### Lines of Code Impact

**New code to write:**
- `pkg/flagparser/interpreter.go` - Base interfaces (~100 lines)
- `pkg/flagparser/terraform_interpreter.go` - Terraform interpreter (~150 lines)
- `pkg/flagparser/helmfile_interpreter.go` - Helmfile interpreter (~100 lines)
- `pkg/flagparser/custom_interpreter.go` - Custom command interpreter (~150 lines)
- `pkg/flagparser/identity.go` - IdentitySelector type (~80 lines)
- Parser updates to return typed interpreters (~200 lines across all parsers)
- **Total new code: ~780 lines**

**Code to modify:**
- `cmd/terraform_utils.go` - Update to use TerraformInterpreter (~50 line changes)
- `cmd/helmfile.go` - Update to use HelmfileInterpreter (~40 line changes)
- `cmd/packer.go` - Update to use PackerInterpreter (~40 line changes)
- `cmd/auth_exec.go` - Update to use IdentitySelector (~30 line changes)
- Custom command handling - Dynamic interpreter (~60 line changes)
- **Total modifications: ~220 lines**

**Code to remove:**
- None! Old code stays during migration, removed after verification.
- Post-migration cleanup: ~150 lines of old flag access patterns

**Net change: +780 lines (new) + 220 lines (modified) = ~1,000 lines of code**

### Breaking vs. Non-Breaking Changes

**✅ NON-BREAKING (99% of changes):**
- All interpreter changes are **internal only**
- User-facing CLI syntax stays **identical**
- All existing flags, env vars, configs work **as-is**
- Precedence behavior remains **unchanged**
- Backward compatibility **100% maintained**

**⚠️ POTENTIALLY BREAKING (minimal risk):**
- If custom code directly accesses `parsedConfig.AtmosFlags` map
  - **Mitigation**: Deprecated accessors can be provided for transition period
  - **Example**: Keep `parsedConfig.AtmosFlags["stack"]` working alongside `interpreter.Stack`
- If tests rely on internal parser structures
  - **Mitigation**: Update tests incrementally, provide test helpers

### Migration Complexity by Command Type

#### 1. **Pass-Through Commands** (Terraform, Helmfile, Packer)
**Complexity: HIGH (first one), MEDIUM (subsequent)**

**Terraform (first migration):**
- Most complex command with most flags
- Multiple flag types: bool, string, optional bool
- Identity integration
- Pass-through args handling
- **Effort: 5-7 days** (includes building infrastructure)

**Helmfile, Packer (subsequent):**
- Reuse patterns from Terraform
- Similar structure, fewer flags
- **Effort: 1-2 days each**

**Changes required:**
```go
// BEFORE (current)
info.Stack = parsedConfig.AtmosFlags["stack"].(string)
info.DryRun = parsedConfig.AtmosFlags["dry-run"].(bool)

// AFTER (strongly-typed)
info.Stack = interpreter.Stack
info.DryRun = interpreter.DryRun
```

**Test updates:**
```go
// BEFORE (current)
mock := &ParsedConfig{
    AtmosFlags: map[string]interface{}{
        "stack": "test-stack",
        "dry-run": true,
    },
}

// AFTER (strongly-typed)
mock := &TerraformInterpreter{
    Stack: "test-stack",
    DryRun: true,
}
```

**Effort breakdown:**
- Create TerraformInterpreter struct: 1 hour
- Update parser to return interpreter: 2-3 hours
- Update command code to use typed fields: 2-3 hours
- Update tests: 4-6 hours
- Integration testing: 3-4 hours

#### 2. **Standard Commands** (Describe, Validate, Workflow, etc.)
**Complexity: LOW-MEDIUM**

**Why easier:**
- No pass-through args complexity
- Fewer flags per command
- Standard patterns
- Well-defined schemas

**Commands to migrate (~8-10 commands):**
- `describe` (stack, component, config)
- `validate` (component, stack, stacks)
- `workflow`
- `list`
- `vendor`
- `version`
- `atlantis`
- `aws`

**Per command effort: 2-4 hours**

**Pattern (nearly identical across commands):**
```go
type DescribeInterpreter struct {
    BaseInterpreter

    Stack     string
    Format    string
    Validate  bool
    Component string
}
```

#### 3. **Custom Commands**
**Complexity: MEDIUM**

**Why medium:**
- Dynamic schema from YAML
- Need generic interpreter
- NoOptDefVal support
- Type validation at runtime

**Effort: 3-4 days**

**Approach:**
```go
type CustomCommandInterpreter struct {
    BaseInterpreter

    Flags map[string]interface{}  // Dynamic flags
}

// Convenience methods for common types
func (c *CustomCommandInterpreter) GetString(key string) string
func (c *CustomCommandInterpreter) GetBool(key string) bool
func (c *CustomCommandInterpreter) Has(key string) bool
```

**Schema extension needed:**
```yaml
# atmos.yaml
commands:
  - name: deploy-app
    flags:
      - name: environment
        type: string
        required: true
      - name: identity
        type: string
        no_opt_default: "__SELECT__"  # NEW: Enable identity pattern
```

#### 4. **Identity Flag Integration**
**Complexity: MEDIUM**

**Why medium:**
- Special semantics (interactive selection)
- Used across multiple commands
- NoOptDefVal interaction
- Needs dedicated type

**Effort: 1-2 days**

**IdentitySelector type:**
```go
type IdentitySelector struct {
    value    string
    provided bool
}

func (i IdentitySelector) IsInteractiveSelector() bool
func (i IdentitySelector) Value() string
func (i IdentitySelector) IsEmpty() bool
```

**Usage update:**
```go
// BEFORE (current)
identity := parsedConfig.AtmosFlags["identity"].(string)
if identity == IdentityFlagSelectValue {
    handleInteractiveSelection()
}

// AFTER (strongly-typed)
if interpreter.Identity.IsInteractiveSelector() {
    handleInteractiveSelection()
} else if !interpreter.Identity.IsEmpty() {
    identity := interpreter.Identity.Value()
}
```

### Testing Requirements

#### Unit Tests
**Coverage target: 85-90%**

**New tests needed:**
- Interpreter construction from resolved flags (~10 tests per interpreter type)
- IdentitySelector behavior (~8 tests)
- Parser returns correct interpreter type (~5 tests per parser)
- Type safety edge cases (~10 tests)
- **Estimated: 150-200 new test cases**
- **Effort: 2-3 days**

#### Integration Tests
**Existing tests should mostly pass unchanged**

**Updates needed:**
- Mock parsedConfig → Mock interpreter in test helpers
- Update assertions to use typed fields
- Verify backward compatibility
- **Estimated: 50-80 test updates**
- **Effort: 1-2 days**

#### Regression Tests
**Critical: Ensure nothing breaks**

**Test scenarios:**
- All existing CLI commands with current syntax
- Environment variable precedence
- Config file precedence
- Flag precedence
- Pass-through args
- Identity flag patterns
- Custom commands
- **Effort: Built into each phase, ~1 day dedicated verification**

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Breaking existing functionality** | Low | High | Comprehensive integration tests, backward compatibility layer |
| **Type assertion errors** | Medium | Medium | Extensive unit tests, defensive programming |
| **Performance regression** | Low | Low | Benchmark tests, profiling |
| **Incomplete migration** | Medium | Medium | Phase-by-phase rollout, feature flags |
| **Test maintenance burden** | Medium | Low | Update tests incrementally, provide helpers |
| **IdentitySelector complexity** | Low | Low | Well-defined interface, clear documentation |

### Migration Strategy

#### Phase-by-Phase Rollout

**Phase 1: Infrastructure (3-4 days)**
- Create interpreter interfaces
- Create IdentitySelector type
- Update base parser infrastructure
- Add universal precedence helper
- **Risk: Low** (no user impact)
- **Validation: Unit tests only**

**Phase 2: Terraform Command (5-7 days)**
- Most complex command, sets the pattern
- Create TerraformInterpreter
- Update parser to return typed interpreter
- Update command code
- Comprehensive testing
- **Risk: Medium** (most-used command)
- **Validation: Integration tests + manual testing**

**Phase 3: Helmfile & Packer (2-3 days)**
- Reuse Terraform patterns
- Create interpreters
- Update parsers and commands
- **Risk: Low** (pattern proven)
- **Validation: Integration tests**

**Phase 4: Standard Commands (4-5 days)**
- Migrate 8-10 simpler commands
- Nearly identical pattern per command
- Batch testing
- **Risk: Low** (simpler structure)
- **Validation: Existing test suites**

**Phase 5: Custom Commands (3-4 days)**
- Generic interpreter
- YAML schema extension
- Dynamic flag handling
- **Risk: Medium** (dynamic nature)
- **Validation: Custom command test suite**

**Phase 6: Cleanup (3-4 days)**
- Remove deprecated code
- Update documentation
- Final regression testing
- **Risk: Low**
- **Validation: Full integration test suite**

#### Backward Compatibility Strategy

**Transition period (2-3 releases):**
1. **Release N**: Introduce interpreters, keep map access working
2. **Release N+1**: Deprecate map access, add warnings
3. **Release N+2**: Remove map access (breaking change, major version bump)

**Compatibility layer:**
```go
// Provide deprecated access during transition
type ParsedConfig struct {
    // New: strongly-typed interpreters
    TerraformFlags *TerraformInterpreter
    HelmfileFlags  *HelmfileInterpreter

    // Deprecated: for backward compatibility (kept 2-3 releases)
    AtmosFlags map[string]interface{}
}

// Auto-populate map from interpreter (transition period only)
func (p *ParsedConfig) PopulateDeprecatedMap() {
    if p.TerraformFlags != nil {
        p.AtmosFlags["stack"] = p.TerraformFlags.Stack
        p.AtmosFlags["dry-run"] = p.TerraformFlags.DryRun
        // ... etc
    }
}
```

### Success Criteria

**Must achieve before rollout:**
- [ ] **Zero breaking changes** in user-facing CLI
- [ ] **100% backward compatibility** for existing syntax
- [ ] **85%+ test coverage** for interpreter code
- [ ] **All existing tests pass** with new implementation
- [ ] **Performance neutral** (no measurable regression)

**Quality gates per phase:**
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Manual testing checklist completed
- [ ] Code review approved
- [ ] Documentation updated

### Rollback Plan

**Per-phase rollback:**
- Each phase is a separate PR
- Can revert individual PRs without cascading failures
- Old code remains until all phases complete
- Feature flags for gradual rollout (if needed)

**Emergency rollback:**
- Revert to previous commit
- Old implementation still present during transition
- Can disable new parsers via config flag

### Recommendations

**Recommended approach: INCREMENTAL**

1. **Start with Terraform** (highest complexity, most benefit)
   - Validates entire architecture
   - Sets pattern for other commands
   - Provides immediate value for most-used command

2. **Complete pass-through commands** (Helmfile, Packer)
   - Proves pattern scales
   - Completes most complex commands

3. **Batch standard commands**
   - Low risk, high velocity
   - Nearly identical patterns

4. **Finish with custom commands**
   - Requires infrastructure from previous phases
   - Tests dynamic interpreter approach

5. **Clean up after all migrations complete**
   - Remove old code only when safe
   - Maintain compatibility during transition

**Timeline: 4-5 weeks** for full implementation

**Effort: ~20-27 developer-days**

**Risk: LOW to MEDIUM** with incremental approach

## Architecture

### Package Structure

```
pkg/flagparser/
├── parser.go           // FlagParser interface
├── standard.go         // Standard flag parser implementation
├── passthrough.go      // PassThroughHandler interface and impl
├── precedence.go       // Precedence enforcement logic
├── registry.go         // Flag registry for reuse
├── testing.go          // Test helpers
└── parser_test.go      // Comprehensive tests

pkg/config/
├── loader.go           // ConfigLoader interface
├── viper_loader.go     // Viper-based implementation
├── precedence.go       // Precedence order enforcement
└── loader_test.go

cmd/internal/middleware/
├── config.go           // Config loading middleware
├── auth.go             // Authentication middleware
├── validation.go       // Flag validation middleware
└── middleware_test.go
```

### Core Components

#### 1. FlagParser

Handles flag registration and parsing:

```go
// StandardFlagParser implements FlagParser for typical commands
type StandardFlagParser struct {
    flags    *FlagRegistry
    bindings map[string]string // flag name -> viper key
}

func (p *StandardFlagParser) Parse(ctx context.Context, args []string) (*ParsedConfig, error) {
    // Use cobra's native parsing
    // Extract values into ParsedConfig
    // No manual parsing of os.Args
}

func (p *StandardFlagParser) RegisterFlags(cmd *cobra.Command) {
    // Add flags from registry
    for _, flag := range p.flags.All() {
        p.addFlag(cmd, flag)
    }
}

func (p *StandardFlagParser) BindToViper(v *viper.Viper) error {
    // Bind each flag to its Viper key
    for flagName, viperKey := range p.bindings {
        flag := cmd.Flags().Lookup(flagName)

        // Special handling for flags with NoOptDefVal (identity pattern)
        if flag.NoOptDefVal != "" {
            // Only bind environment variables, NOT the flag itself
            // This prevents viper.BindPFlag from interfering with NoOptDefVal detection
            envVars := p.getEnvVarsForFlag(flagName)
            if len(envVars) > 0 {
                if err := v.BindEnv(viperKey, envVars...); err != nil {
                    return err
                }
            }
        } else {
            // Standard flags: bind both flag and env vars
            if err := v.BindPFlag(viperKey, flag); err != nil {
                return err
            }
            // Also bind env vars if specified
            envVars := p.getEnvVarsForFlag(flagName)
            if len(envVars) > 0 {
                if err := v.BindEnv(viperKey, envVars...); err != nil {
                    return err
                }
            }
        }
    }
    return nil
}
```

#### 2. PassThroughFlagParser

Specialized parser for Terraform/Helmfile/Packer that handles the complex flag scenarios:

```go
// PassThroughFlagParser handles commands that pass flags to underlying tools
type PassThroughFlagParser struct {
    atmosFlags       *FlagRegistry        // Known Atmos-specific flags
    handler          PassThroughHandler   // Arg separation logic
    optionalBoolFlags []string            // Flags like --upload-status
}

func (p *PassThroughFlagParser) Parse(ctx context.Context, args []string) (*ParsedConfig, error) {
    cfg := &ParsedConfig{}

    // Step 1: Check for explicit double-dash separator
    beforeDash, afterDash := p.handler.SplitAtDoubleDash(args)

    var atmosArgs, toolArgs []string

    if len(afterDash) > 0 {
        // Explicit mode: -- separator present
        // Everything before -- is for Atmos (but may contain tool args mixed in)
        // Everything after -- goes straight to tool
        atmosArgs = beforeDash
        toolArgs = afterDash

        // Extract Atmos flags from beforeDash, leave rest for tool
        atmosFlagsMap, remaining, err := p.handler.ExtractAtmosFlags(atmosArgs)
        if err != nil {
            return nil, err
        }

        cfg.AtmosFlags = atmosFlagsMap

        // Remaining args before -- are positional args or tool flags
        // Prepend them to toolArgs
        toolArgs = append(remaining, toolArgs...)
    } else {
        // Implicit mode: no -- separator
        // Extract Atmos flags, everything else goes to tool
        atmosFlagsMap, remaining, err := p.handler.ExtractAtmosFlags(args)
        if err != nil {
            return nil, err
        }

        cfg.AtmosFlags = atmosFlagsMap
        toolArgs = remaining
    }

    // Step 2: Handle optional boolean flags (--upload-status)
    for _, flagName := range p.optionalBoolFlags {
        value, present, err := p.parseOptionalBoolFlag(toolArgs, flagName)
        if err != nil {
            return nil, err
        }
        if present {
            cfg.AtmosFlags[flagName] = value
            // Remove flag from toolArgs
            toolArgs = p.removeFlag(toolArgs, flagName)
        }
    }

    // Step 3: Extract positional arguments (component, subcommand)
    positional, remaining, err := p.handler.ExtractPositionalArgs(toolArgs, 2)
    if err != nil {
        return nil, err
    }

    if len(positional) > 0 {
        cfg.SubCommand = positional[0]
    }
    if len(positional) > 1 {
        cfg.ComponentName = positional[1]
    }

    // Everything remaining goes to the tool
    cfg.PassThroughArgs = remaining

    return cfg, nil
}

// parseOptionalBoolFlag handles --flag, --flag=true, --flag=false patterns
func (p *PassThroughFlagParser) parseOptionalBoolFlag(args []string, flagName string) (bool, bool, error) {
    flagPrefix := "--" + flagName
    flagEquals := flagPrefix + "="

    for _, arg := range args {
        if arg == flagPrefix {
            // --flag alone = true
            return true, true, nil
        }
        if strings.HasPrefix(arg, flagEquals) {
            // --flag=value
            value := strings.TrimPrefix(arg, flagEquals)
            value = strings.TrimSpace(value)

            switch strings.ToLower(value) {
            case "true", "1", "yes":
                return true, true, nil
            case "false", "0", "no":
                return false, true, nil
            case "":
                // --flag= (empty value) = true
                return true, true, nil
            default:
                return false, false, fmt.Errorf("invalid boolean value for --%s: %s", flagName, value)
            }
        }
    }

    // Flag not present
    return false, false, nil
}

// removeFlag removes all instances of a flag from args
func (p *PassThroughFlagParser) removeFlag(args []string, flagName string) []string {
    flagPrefix := "--" + flagName
    flagEquals := flagPrefix + "="

    var result []string
    for _, arg := range args {
        if arg == flagPrefix || strings.HasPrefix(arg, flagEquals) {
            continue // Skip this arg
        }
        result = append(result, arg)
    }

    return result
}
```

**Key Features**:
1. **Handles both explicit and implicit modes**: With or without `--` separator
2. **Optional boolean flags**: Supports `--flag`, `--flag=true`, `--flag=false`
3. **Preserves tool args**: Doesn't parse or modify Terraform/Helmfile flags
4. **Order preservation**: Maintains argument order for tool execution
5. **Error handling**: Clear errors for malformed flags

#### 3. ConfigLoader

Manages configuration loading with precedence:

```go
// ViperConfigLoader implements ConfigLoader using Viper
type ViperConfigLoader struct {
    viper    *viper.Viper
    parser   FlagParser
    envVars  map[string][]string // viper key -> env var names
}

func (l *ViperConfigLoader) Load(ctx context.Context, opts ...LoadOption) (*Config, error) {
    // 1. Set defaults
    l.setDefaults()

    // 2. Load config files
    if err := l.loadConfigFiles(); err != nil {
        return nil, err
    }

    // 3. Bind environment variables
    l.bindEnvVars()

    // 4. Flags already bound via BindToViper()
    //    Viper automatically maintains precedence

    // 5. Marshal into Config struct
    cfg := &Config{}
    if err := l.viper.Unmarshal(cfg); err != nil {
        return nil, err
    }

    return cfg, nil
}
```

#### 4. CustomCommandFlagParser

Dynamically creates parser from YAML configuration:

```go
// CustomCommandFlagParser handles flags defined in atmos.yaml
type CustomCommandFlagParser struct {
    commandSpec *schema.CustomCommand // From atmos.yaml
    flags       *FlagRegistry
}

func NewCustomCommandFlagParser(spec *schema.CustomCommand) (*CustomCommandFlagParser, error) {
    parser := &CustomCommandFlagParser{
        commandSpec: spec,
        flags:       NewFlagRegistry(),
    }

    // Dynamically register flags from YAML spec
    for _, flagSpec := range spec.Flags {
        if err := parser.registerFlagFromSpec(flagSpec); err != nil {
            return nil, err
        }
    }

    return parser, nil
}

func (p *CustomCommandFlagParser) registerFlagFromSpec(spec *schema.FlagSpec) error {
    switch spec.Type {
    case "string":
        flag := &StringFlag{
            Name:        spec.Name,
            Shorthand:   spec.Shorthand,
            Default:     spec.Default,
            Description: spec.Description,
            Required:    spec.Required,
        }

        // Support NoOptDefVal for identity pattern
        if spec.NoOptDefault != "" {
            flag.NoOptDefVal = spec.NoOptDefault
        }

        p.flags.Register(flag)

    case "bool":
        flag := &BoolFlag{
            Name:        spec.Name,
            Shorthand:   spec.Shorthand,
            Default:     spec.Default.(bool),
            Description: spec.Description,
        }
        p.flags.Register(flag)

    // ... other types (int, float, etc.)
    }

    return nil
}

func (p *CustomCommandFlagParser) RegisterFlags(cmd *cobra.Command) {
    for _, flag := range p.flags.All() {
        switch f := flag.(type) {
        case *StringFlag:
            cmd.Flags().StringP(f.Name, f.Shorthand, f.Default, f.Description)

            // Set NoOptDefVal if specified (identity pattern)
            if f.NoOptDefVal != "" {
                flagObj := cmd.Flags().Lookup(f.Name)
                if flagObj != nil {
                    flagObj.NoOptDefVal = f.NoOptDefVal
                }
            }

        case *BoolFlag:
            cmd.Flags().BoolP(f.Name, f.Shorthand, f.Default, f.Description)
        }
    }
}

func (p *CustomCommandFlagParser) ValidateRequired(cmd *cobra.Command) error {
    for _, flag := range p.flags.All() {
        if flag.IsRequired() && !cmd.Flags().Changed(flag.GetName()) {
            return fmt.Errorf("%w: required flag --%s not set", errUtils.ErrFlagValidation, flag.GetName())
        }
    }
    return nil
}
```

**Example YAML config with NoOptDefVal:**
```yaml
commands:
  - name: deploy-app
    description: Deploy application to environment
    flags:
      - name: environment
        shorthand: e
        type: string
        required: true
        description: Target environment (prod, staging, dev)

      - name: identity
        shorthand: i
        type: string
        description: Identity to assume (use alone to select interactively)
        no_opt_default: "__SELECT__"  # Enables NoOptDefVal pattern
        env_vars:
          - ATMOS_IDENTITY
          - IDENTITY

      - name: dry-run
        type: bool
        default: false
        description: Perform dry run without making changes
```

**Key features:**
1. **Dynamic flag registration** - Flags created from YAML config
2. **NoOptDefVal support** - `no_opt_default` field enables identity pattern
3. **Type safety** - Validates flag types from config
4. **Required validation** - Enforces required flags
5. **Environment binding** - Custom env var names per flag
6. **Precedence** - Works with standard precedence system

#### 5. Middleware

Composable middleware for configuration pipeline:

```go
// ConfigMiddleware loads configuration before command execution
func ConfigMiddleware(loader ConfigLoader) CobraMiddleware {
    return func(cmd *cobra.Command, args []string) error {
        ctx := cmd.Context()

        cfg, err := loader.Load(ctx)
        if err != nil {
            return fmt.Errorf("%w: %v", errUtils.ErrLoadConfig, err)
        }

        // Store in context
        cmd.SetContext(config.WithConfig(ctx, cfg))
        return nil
    }
}

// AuthMiddleware handles authentication if --identity is set
func AuthMiddleware(authManager AuthManager) CobraMiddleware {
    return func(cmd *cobra.Command, args []string) error {
        cfg := config.FromContext(cmd.Context())

        if cfg.Identity == "" {
            return nil // No auth needed
        }

        if err := authManager.Authenticate(cmd.Context(), cfg.Identity); err != nil {
            return fmt.Errorf("%w: %v", errUtils.ErrAuth, err)
        }

        return nil
    }
}

// Compose middleware
func ComposeMiddleware(middlewares ...CobraMiddleware) CobraMiddleware {
    return func(cmd *cobra.Command, args []string) error {
        for _, mw := range middlewares {
            if err := mw(cmd, args); err != nil {
                return err
            }
        }
        return nil
    }
}
```

### Usage Examples

#### Command Registry Pattern (Recommended)

**For new commands using command registry:**

```go
package describe

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
    "github.com/cloudposse/atmos/pkg/flagparser"
    "github.com/cloudposse/atmos/pkg/config"
)

// DescribeCommandProvider implements CommandProvider.
type DescribeCommandProvider struct {
    parser flagparser.FlagParser
    loader config.ConfigLoader
}

func init() {
    // Create parser with describe-specific flags
    parser := flagparser.NewStandardFlagParser(
        flagparser.WithFlag("stack", "s", "", "Stack name"),
        flagparser.WithFlag("format", "f", "yaml", "Output format (yaml, json)"),
        flagparser.WithBoolFlag("validate", "v", false, "Validate configuration"),
    )

    // Create provider
    provider := &DescribeCommandProvider{
        parser: parser,
        loader: config.NewViperLoader(),
    }

    // Register with command registry
    internal.Register(provider)
}

func (d *DescribeCommandProvider) GetCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "describe [subcommand]",
        Short: "Describe stack configurations",
    }

    // Register flags using unified parser
    d.parser.RegisterFlags(cmd)

    // Bind to Viper (handles precedence automatically)
    if err := d.parser.BindToViper(d.loader.Viper()); err != nil {
        panic(err) // Only during initialization
    }

    // Middleware chain
    cmd.PersistentPreRunE = middleware.ConfigMiddleware(d.loader)

    return cmd
}

func (d *DescribeCommandProvider) GetName() string {
    return "describe"
}

func (d *DescribeCommandProvider) GetGroup() string {
    return "Stack Introspection"
}
```

**Benefits:**
- Parser is injected, making provider testable
- Flags are self-contained within the provider
- No global state or imports in init()
- Follows command registry pattern exactly
- Works with TestKit for isolated testing

#### Standard Command (e.g., Validate)

```go
func NewValidateCmd(
    loader ConfigLoader,
    parser FlagParser,
    validator Validator,
) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "validate",
        Short: "Validate stack configuration",
    }

    // Register flags
    parser.RegisterFlags(cmd)

    // Bind to Viper
    if err := parser.BindToViper(loader.Viper()); err != nil {
        panic(err) // During initialization only
    }

    // Middleware chain
    cmd.PersistentPreRunE = ComposeMiddleware(
        ConfigMiddleware(loader),
        ValidationMiddleware(),
    )

    // Business logic
    cmd.RunE = func(cmd *cobra.Command, args []string) error {
        cfg := config.FromContext(cmd.Context())
        return validator.Validate(cmd.Context(), cfg)
    }

    return cmd
}
```

#### Pass-Through Command (e.g., Terraform)

```go
func NewTerraformCmd(
    loader ConfigLoader,
    parser PassThroughFlagParser,
    executor TerraformExecutor,
) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "terraform [command] -- [terraform flags]",
        Short: "Execute Terraform commands",
    }

    // Register Atmos flags only
    parser.RegisterFlags(cmd)

    // Bind to Viper
    if err := parser.BindToViper(loader.Viper()); err != nil {
        panic(err)
    }

    // Middleware
    cmd.PersistentPreRunE = ComposeMiddleware(
        ConfigMiddleware(loader),
        AuthMiddleware(executor.AuthManager()),
    )

    // Business logic
    cmd.RunE = func(cmd *cobra.Command, args []string) error {
        cfg := config.FromContext(cmd.Context())

        // Execute with both Atmos config and tool args
        return executor.Execute(cmd.Context(), cfg, cfg.PassThroughArgs)
    }

    return cmd
}
```

## Implementation Plan

### Phase 1: Core Infrastructure (Week 1)

**Goal**: Build foundational interfaces and implementations

- [ ] Create `pkg/flagparser/` package
  - [ ] Define `FlagParser` interface
  - [ ] Implement `StandardFlagParser`
  - [ ] Implement `PassThroughHandler`
  - [ ] Add comprehensive unit tests (90% coverage target)

- [ ] Create `pkg/config/` refactor
  - [ ] Define `ConfigLoader` interface
  - [ ] Implement `ViperConfigLoader`
  - [ ] Extract precedence logic
  - [ ] Add unit tests

- [ ] Create `cmd/internal/middleware/` package
  - [ ] Define `CobraMiddleware` type
  - [ ] Implement `ConfigMiddleware`
  - [ ] Implement `AuthMiddleware`
  - [ ] Implement `ComposeMiddleware`
  - [ ] Add unit tests

### Phase 2: Pass-Through Commands (Week 2)

**Goal**: Migrate Terraform, Helmfile, Packer to unified system

- [ ] Implement `PassThroughFlagParser`
  - [ ] Double dash separator support
  - [ ] Atmos flag extraction
  - [ ] Integration tests

- [ ] Migrate Terraform command
  - [ ] Update to use `PassThroughFlagParser`
  - [ ] Remove custom flag parsing logic
  - [ ] Add/update tests
  - [ ] Verify backward compatibility

- [ ] Migrate Helmfile command
  - [ ] Same pattern as Terraform
  - [ ] Remove duplicated code
  - [ ] Add/update tests

- [ ] Migrate Packer command
  - [ ] Handle special flags (--template, --query)
  - [ ] Use unified parser
  - [ ] Add/update tests

### Phase 3: Standard Commands (Week 3)

**Goal**: Migrate standard commands to unified system

- [ ] Migrate Validate command
  - [ ] Use `StandardFlagParser`
  - [ ] Apply middleware pattern
  - [ ] Add/update tests

- [ ] Migrate Describe command
  - [ ] Use `StandardFlagParser`
  - [ ] Apply middleware pattern
  - [ ] Add/update tests

- [ ] Migrate Workflow command
  - [ ] Use `StandardFlagParser`
  - [ ] Apply middleware pattern
  - [ ] Add/update tests

- [ ] Migrate remaining commands
  - [ ] Vendor, Version, List, etc.
  - [ ] Consistent pattern
  - [ ] Full test coverage

### Phase 4: Global Flags (Week 4)

**Goal**: Ensure global flags work consistently

- [ ] Create `GlobalFlagParser`
  - [ ] Registers global flags
  - [ ] Binds to Viper
  - [ ] Used by all commands

- [ ] Update RootCmd
  - [ ] Use `GlobalFlagParser`
  - [ ] Apply middleware pattern
  - [ ] Remove custom flag handling

- [ ] Verify propagation
  - [ ] Test global flags in subcommands
  - [ ] Test with pass-through commands
  - [ ] Integration tests

### Phase 5: Custom Commands (Week 5)

**Goal**: Migrate custom commands to unified system

- [ ] Create `CustomCommandParser`
  - [ ] Dynamic flag registration from YAML config
  - [ ] Support for all flag types (bool, string, int, float)
  - [ ] Required flag validation
  - [ ] NoOptDefVal support for string flags (identity pattern)
  - [ ] Equals and space form support for all flag types
  - [ ] Shorthand flag support

- [ ] Extend flag config schema
  - [ ] Add `no_opt_default` field to flag spec in atmos.yaml
  - [ ] Support identity integration for custom commands
  - [ ] Document YAML schema for custom command flags
  - [ ] Update JSON schemas in `pkg/datafetcher/schema/`

- [ ] Migrate custom command execution
  - [ ] Use unified parser with dynamic flag registration
  - [ ] Remove duplicated logic
  - [ ] Preserve precedence order
  - [ ] Add comprehensive tests

- [ ] Test with various custom commands
  - [ ] Simple commands with few flags
  - [ ] Commands with many flags (10+)
  - [ ] Commands with identity flag
  - [ ] Commands with NoOptDefVal string flags
  - [ ] Commands with required flags
  - [ ] Commands with boolean optional values
  - [ ] Commands mixing equals and space forms

### Phase 6: Cleanup & Documentation (Week 6)

**Goal**: Remove old code and document new system

- [ ] Remove deprecated code
  - [ ] `cmd/cmd_utils.go` custom parsing
  - [ ] `internal/exec/cli_utils.go` duplicated logic
  - [ ] `pkg/config/config.go` manual parsing

- [ ] Update documentation
  - [ ] Add architecture docs
  - [ ] Update CLAUDE.md
  - [ ] Add usage examples
  - [ ] Document testing patterns

- [ ] Final testing
  - [ ] Full integration test suite
  - [ ] Backward compatibility tests
  - [ ] Performance benchmarks
  - [ ] Coverage report

## Testing Strategy

### Unit Tests (Target: 90% coverage)

- **FlagParser**: Test flag registration, parsing, Viper binding
- **ConfigLoader**: Test precedence order, file loading, env vars
- **Middleware**: Test composition, error handling, context propagation
- **PassThroughHandler**: Test double dash separation, flag extraction

### Integration Tests

- **Commands**: Test each command with various flag combinations
- **Global Flags**: Test propagation across commands
- **Precedence**: Test flags override env vars, env vars override config
- **Pass-Through**: Test double dash with real tool invocations

### Backward Compatibility Tests

- **Existing Flags**: Ensure all current flags still work
- **Config Files**: Verify existing configs still load
- **Environment Variables**: Check all env vars respected
- **Command Syntax**: Confirm no breaking changes

## Migration Strategy

### Backward Compatibility

1. **No breaking changes**: All existing flags, env vars, config options continue to work
2. **Deprecation period**: If flags need renaming, keep aliases for 2+ versions
3. **Config migration**: Auto-migrate old config formats if needed
4. **Documentation**: Clear migration guide for any changes

### Rollout Plan

1. **Phase 1-2**: Internal infrastructure, no user-facing changes
2. **Phase 3**: Command migrations, maintain identical behavior
3. **Phase 4-5**: Feature completeness, global flags improvements
4. **Phase 6**: Cleanup and documentation

### Rollback Plan

- Each phase is independently deployable
- Old code remains until new code is fully tested
- Feature flags for gradual rollout if needed
- Can revert to old implementation per command

## Risks & Mitigations

### Risk: Breaking Existing Functionality

**Mitigation**:
- Comprehensive integration tests
- Backward compatibility tests
- Gradual rollout with monitoring
- Beta testing period

### Risk: Performance Regression

**Mitigation**:
- Benchmark tests for flag parsing
- Performance tests in CI
- Heatmap analysis for bottlenecks
- Optimization phase if needed

### Risk: Complexity Increase

**Mitigation**:
- Clear documentation
- Simple, focused interfaces
- Code examples for each pattern
- Consistent architecture across commands

## Success Metrics

- [ ] **Test Coverage**: 80-90% for new flag parsing code
- [ ] **Code Reduction**: Remove 500+ lines of duplicated flag parsing logic
- [ ] **Performance**: No regression in command execution time
- [ ] **Consistency**: All commands use unified system
- [ ] **Maintainability**: New commands require <50 lines for flag setup

## Future Enhancements

- **Flag Validation DSL**: Declarative validation rules
- **Auto-completion**: Better shell completion for dynamic flags
- **Configuration UI**: Interactive configuration wizard
- **Flag Groups**: Mutually exclusive or required-together flags
- **Remote Configuration**: Load config from remote stores

## References

- [Sting of the Viper: Cobra and Viper Integration](https://carolynvanslyck.com/blog/2020/08/sting-of-the-viper/)
- [Docker CLI TopLevelCommand Pattern](https://github.com/docker/cli/blob/master/cli/cobra.go)
- [Testing Flag Parsing in Go](https://eli.thegreenplace.net/2020/testing-flag-parsing-in-go-programs/)
- [Functional Options Pattern](https://www.codingexplorations.com/blog/functional-options-pattern-go)
- Atmos existing PRDs:
  - `docs/prd/command-registry-pattern.md` - Command registration system
  - `docs/prd/testing-strategy.md` - Testing approach and coverage
  - `docs/prd/flag-handling/` - Strongly-typed interpreter system (NEW)
    - `global-flags-pattern.md` - Global flags handling via embedding
    - `global-flags-examples.md` - Real-world examples with --logs-level and --identity
    - `default-values-pattern.md` - Default value handling across all layers
