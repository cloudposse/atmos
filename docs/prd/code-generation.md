# PRD: Generate Section in Atmos Stack Config

## Overview

Add a `generate` section to Atmos stack configuration that allows users to declaratively define files to be generated alongside Terraform components. Files are written to the component directory and support templating with full component context.

## Command

**Primary Command:** `atmos terraform generate files <component> -s <stack>`

**Batch Command:** `atmos terraform generate files --all` with optional `--stacks`, `--components` filters

**Auto-generation:** Controlled via `components.terraform.auto_generate_files: true` in `atmos.yaml`

## Configuration Structure

The `generate` section can be defined at three levels, following standard Atmos merge behavior:

```yaml
# Level 1: Global (applies to all terraform components)
terraform:
  generate:
    "common.auto.tfvars.json":
      locals:
        atmos_component: "{{ .atmos_component }}"

# Level 2: Component type level (in components.terraform section)
components:
  terraform:
    generate:
      "providers.tf.json":
        terraform:
          required_providers:
            aws:
              source: "hashicorp/aws"
              version: ">= 5.0"

# Level 3: Component level (highest priority)
components:
  terraform:
    vpc:
      generate:
        "context.auto.tfvars.json":
          namespace: "{{ .vars.namespace }}"
          environment: "{{ .vars.environment }}"
        "backend.tf": |
          terraform {
            backend "s3" {
              bucket = "{{ .backend.bucket }}"
              key    = "{{ .backend.key }}"
            }
          }
```

**Important:** Filenames containing dots (`.`) MUST be quoted in YAML to be parsed as strings.

## Value Types

### Map Value → Serialized Based on File Extension

```yaml
generate:
  # .json extension → JSON serialization
  "config.auto.tfvars.json":
    key: value
    nested:
      foo: bar

  # .yaml/.yml extension → YAML serialization
  "metadata.yaml":
    component: "{{ .atmos_component }}"
    stack: "{{ .atmos_stack }}"

  # .hcl/.tf extension → HCL serialization (simple values only)
  "locals.tf":
    locals:
      environment: "{{ .vars.environment }}"
```

### String Value → Literal Template

Use YAML folding styles for multi-line content:

```yaml
generate:
  # Literal block scalar (|) - preserves newlines exactly
  "README.md": |
    # {{ .atmos_component }}

    This component is deployed to {{ .atmos_stack }}.

  # Folded block scalar (>) - folds newlines to spaces, blank lines become newlines
  "description.txt": >
    This is a long description that will be
    folded into a single line, but blank lines

    will create paragraph breaks.

  # Literal with strip (-) - removes trailing newlines
  "version.txt": |-
    {{ .vars.version }}

  # Complex HCL is best as literal string
  "custom.tf": |
    resource "null_resource" "marker" {
      triggers = {
        component = "{{ .atmos_component }}"
      }
    }

  # Provider configuration with expressions
  "providers.tf": |
    provider "aws" {
      region = "{{ .vars.region }}"

      default_tags {
        tags = {
          Environment = "{{ .vars.environment }}"
          Component   = "{{ .atmos_component }}"
        }
      }
    }
```

**YAML Block Scalar Reference:**

| Style | Syntax | Behavior |
|-------|--------|----------|
| Literal | `\|` | Preserves newlines exactly |
| Literal strip | `\|-` | Preserves newlines, strips trailing |
| Literal keep | `\|+` | Preserves newlines, keeps trailing |
| Folded | `>` | Folds newlines to spaces |
| Folded strip | `>-` | Folds newlines, strips trailing |

## Template Context

Templates have access to the full component context:

| Variable | Description |
|----------|-------------|
| `.vars` | Component variables |
| `.settings` | Component settings |
| `.env` | Environment variables |
| `.backend` | Backend configuration |
| `.backend_type` | Backend type (s3, gcs, azurerm, etc.) |
| `.providers` | Provider configuration |
| `.atmos_component` | Component name |
| `.atmos_stack` | Stack name |
| `.atmos_stack_file` | Stack file path |
| `.namespace`, `.tenant`, `.environment`, `.stage`, `.region` | Context variables |
| `.component`, `.base_component` | Component inheritance info |
| `.workspace` | Terraform workspace |
| `.metadata` | Component metadata |

Plus all standard Atmos template functions: `atmos.Component()`, `atmos.Stack()`, etc.

## File Output Location

Files are always written relative to the component's directory:

```text
components/terraform/<component>/<filename>
```

Example:
```yaml
components:
  terraform:
    vpc:
      generate:
        "context.auto.tfvars.json": { ... }
```

Writes to: `components/terraform/vpc/context.auto.tfvars.json`

## Merge Behavior

Standard Atmos deep merge with later values winning:

1. **Global level** (`terraform.generate`) - lowest priority
2. **Component type level** (`components.terraform.generate`)
3. **Base component level** (via inheritance)
4. **Component level** (`components.terraform.vpc.generate`) - highest priority

Files merge by filename - component-level definition of same filename completely replaces higher-level definition.

## Auto-Generation

### Configuration in atmos.yaml

```yaml
components:
  terraform:
    auto_generate_files: true  # Enable auto-generation
```

### Trigger Points

When `auto_generate_files: true`, files are generated before:
- `atmos terraform plan`
- `atmos terraform apply`
- `atmos terraform deploy`
- `atmos terraform init`
- Any terraform command that processes templates

(Same trigger points as `auto_generate_backend_file`)

## CLI Commands

### Single Component

```bash
# Generate files for one component
atmos terraform generate files vpc -s prod-ue2

# Dry run (show what would be generated)
atmos terraform generate files vpc -s prod-ue2 --dry-run
```

### All Components (Batch)

```bash
# Generate for all components in all stacks
atmos terraform generate files --all

# Filter by stacks (requires --all)
atmos terraform generate files --all --stacks "prod-*"

# Filter by components (requires --all)
atmos terraform generate files --all --components "vpc,rds"

# Combined filters
atmos terraform generate files --all --stacks "prod-ue2" --components "vpc"
```

### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--stack` | `-s` | string | Stack name (required for single component) |
| `--all` | | bool | Process all components in all stacks |
| `--stacks` | | string | Filter stacks (glob pattern, requires --all) |
| `--components` | | string | Filter components (comma-separated, requires --all) |
| `--dry-run` | | bool | Show what would be generated/deleted without writing |
| `--clean` | | bool | Delete generated files instead of creating them |
| `--process-templates` | | bool | Process Go templates (default: true) |
| `--process-functions` | | bool | Process YAML functions (default: true) |

## Schema Changes

### pkg/schema/schema.go

Add to `Terraform` struct:
```go
type Terraform struct {
    // ... existing fields ...
    AutoGenerateFiles bool `yaml:"auto_generate_files" json:"auto_generate_files" mapstructure:"auto_generate_files"`
}
```

### Stack Configuration Schema

The `generate` section is a `map[string]any` where:
- Key = filename (string)
- Value = content (string for literal, map for serialized)

## Implementation Files

### New Files

| File | Purpose |
|------|---------|
| `cmd/terraform/generate/files.go` | Command following the new terraform subcommand pattern |
| `pkg/generate/generate.go` | Core file generation logic |
| `pkg/generate/generate_test.go` | Unit tests with mocks |

### Command Implementation Pattern

Following the refactored terraform command structure (PR #1813), the `files` subcommand registers directly with `GenerateCmd`:

```go
// cmd/terraform/generate/files.go
package generate

import (
    "fmt"
    "strings"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"

    errUtils "github.com/cloudposse/atmos/errors"
    e "github.com/cloudposse/atmos/internal/exec"
    cfg "github.com/cloudposse/atmos/pkg/config"
    "github.com/cloudposse/atmos/pkg/flags"
    "github.com/cloudposse/atmos/pkg/schema"
)

// filesParser handles flag parsing for files command.
var filesParser *flags.StandardParser

// filesCmd generates files for terraform components from the generate section.
var filesCmd = &cobra.Command{
    Use:   "files [component]",
    Short: "Generate files for Terraform components from the generate section",
    Long: `Generate additional configuration files for Terraform components based on
the generate section in stack configuration.

When called with a component argument, generates files for that component.
When called with --all, generates files for all components across stacks.`,
    Args:               cobra.MaximumNArgs(1),
    FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
    RunE: func(cmd *cobra.Command, args []string) error {
        // Use Viper to respect precedence (flag > env > config > default)
        v := viper.GetViper()

        // Bind files-specific flags to Viper
        if err := filesParser.BindFlagsToViper(cmd, v); err != nil {
            return err
        }

        // Get flag values from Viper
        stack := v.GetString("stack")
        all := v.GetBool("all")
        stacksCsv := v.GetString("stacks")
        componentsCsv := v.GetString("components")
        dryRun := v.GetBool("dry-run")
        clean := v.GetBool("clean")

        // Validate: component requires stack, --all excludes component
        if len(args) > 0 && all {
            return fmt.Errorf("%w: cannot specify both component and --all", errUtils.ErrInvalidFlag)
        }
        if len(args) > 0 && stack == "" {
            return fmt.Errorf("%w: --stack is required when specifying a component", errUtils.ErrInvalidFlag)
        }
        if len(args) == 0 && !all {
            return fmt.Errorf("%w: either specify a component or use --all", errUtils.ErrInvalidFlag)
        }

        // Parse CSV values
        var stacks []string
        if stacksCsv != "" {
            stacks = strings.Split(stacksCsv, ",")
        }

        var components []string
        if componentsCsv != "" {
            components = strings.Split(componentsCsv, ",")
        }

        // Initialize Atmos configuration
        atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
        if err != nil {
            return err
        }

        // Execute based on mode
        if all {
            return e.ExecuteTerraformGenerateFilesAll(&atmosConfig, stacks, components, dryRun, clean)
        }

        component := args[0]
        return e.ExecuteTerraformGenerateFiles(&atmosConfig, component, stack, dryRun, clean)
    },
}

func init() {
    // Create parser with files-specific flags using functional options.
    filesParser = flags.NewStandardParser(
        flags.WithStringFlag("stack", "s", "", "Stack name (required for single component)"),
        flags.WithBoolFlag("all", "", false, "Process all components in all stacks"),
        flags.WithStringFlag("stacks", "", "", "Filter stacks (glob pattern, requires --all)"),
        flags.WithStringFlag("components", "", "", "Filter components (comma-separated, requires --all)"),
        flags.WithBoolFlag("dry-run", "", false, "Show what would be generated without writing"),
        flags.WithBoolFlag("clean", "", false, "Delete generated files instead of creating"),
        flags.WithEnvVars("stack", "ATMOS_STACK"),
        flags.WithEnvVars("stacks", "ATMOS_STACKS"),
        flags.WithEnvVars("components", "ATMOS_COMPONENTS"),
    )

    // Register flags with the command.
    filesParser.RegisterFlags(filesCmd)

    // Bind flags to Viper for environment variable support.
    if err := filesParser.BindToViper(viper.GetViper()); err != nil {
        panic(err)
    }

    // Register with parent GenerateCmd (not internal.Register - this is a subcommand)
    GenerateCmd.AddCommand(filesCmd)
}
```

### Key Implementation Notes

1. **No CommandProvider needed**: Unlike top-level commands, subcommands register directly with their parent via `GenerateCmd.AddCommand(filesCmd)` in `init()`

2. **Update GenerateCmd help text**: Add "files" to the list in `cmd/terraform/generate/generate.go`:
   ```go
   Long: `...
   - 'files' to generate files from the generate section for an Atmos component.`,
   ```

3. **Pattern follows existing commands**: See `cmd/terraform/generate/varfiles.go` as the reference implementation

### Modified Files

| File | Change |
|------|--------|
| `cmd/terraform/generate/generate.go` | Update Long description to include 'files' subcommand |
| `pkg/schema/schema.go` | Add `AutoGenerateFiles` to Terraform struct |
| `internal/exec/terraform.go` | Call `generateFiles()` when auto-generate enabled |
| `internal/exec/terraform_generate_files.go` | New file with `ExecuteTerraformGenerateFiles()` and `ExecuteTerraformGenerateFilesAll()` |
| `internal/exec/terraform_clean.go` | Update `initializeFilesToClear()` to include generated files |
| `pkg/datafetcher/schema/atmos-manifest.json` | Add `generate` to component schema |

## HCL Serialization

For `.hcl` and `.tf` files, support simple value serialization:

- Scalars (string, number, bool)
- Lists
- Maps (nested)

Complex expressions should use string templates instead.

Example:
```yaml
generate:
  # Map serialized to HCL
  "locals.tf":
    locals:
      environment: prod
      tags:
        Project: myapp

# Produces:
# locals {
#   environment = "prod"
#   tags = {
#     Project = "myapp"
#   }
# }
```

## Internal Implementation: Lifecycle Hooks Integration

**Note:** This section describes internal implementation details not exposed to users.

File generation integrates with the existing lifecycle hooks system, following the same pattern as `auto_generate_backend_file`. This provides a consistent execution model and enables future extensibility.

### Execution Point

When `auto_generate_files: true`, the `generateFiles()` function is called at the same point as `generateBackendConfig()` in `internal/exec/terraform.go`:

```go
// internal/exec/terraform.go (around line 304-310)

// Component working directory
workingDir := constructTerraformComponentWorkingDir(&atmosConfig, &info)

err = generateBackendConfig(&atmosConfig, &info, workingDir)
if err != nil {
    return err
}

// NEW: Generate files from generate section
err = generateFiles(&atmosConfig, &info, workingDir)
if err != nil {
    return err
}
```

### Relationship to Lifecycle Hooks

The file generation happens as part of the pre-execution phase, before any lifecycle hooks like `before.terraform.plan` are triggered. This ensures generated files exist before user-defined hooks or terraform commands run.

**Execution order:**
1. Stack processing and template evaluation
2. **File generation** (from `generate` section) ← NEW
3. Backend generation (from `auto_generate_backend_file`)
4. Provider overrides generation
5. `before.terraform.*` lifecycle hooks
6. Terraform command execution
7. `after.terraform.*` lifecycle hooks

### Why Not Expose as Hooks

File generation is implemented as a built-in feature rather than user-configurable hooks because:

1. **Timing**: Files must exist before terraform commands, not after
2. **Simplicity**: Users configure what to generate, not when
3. **Consistency**: Same model as existing backend generation
4. **Reliability**: Built-in ensures files are always generated

## Error Handling

- Invalid template syntax → Error with file name and line number
- Missing template variable → Error with variable name and context
- File write failure → Error with path and OS error
- Invalid HCL structure → Error suggesting string template instead

## Testing Strategy

1. **Unit tests** in `pkg/generate/generate_test.go`
2. **Command tests** in `cmd/terraform/generate/files/files_test.go`
3. **Integration tests** in `tests/` with fixtures in `tests/test-cases/generate-files/`
4. **Test cases:**
   - Single file generation (JSON, YAML, HCL, string)
   - Multi-level merge behavior
   - Template variable substitution
   - Auto-generation trigger
   - Dry-run mode
   - Error cases

## Documentation

- `website/docs/cli/commands/terraform/terraform-generate-files.mdx`
- Update `website/docs/core-concepts/stacks/` with generate section
- Add examples to quick-start guides

## Cleanup Commands

### Dedicated Cleanup Command

```bash
# Clean generated files for one component
atmos terraform generate files --clean vpc -s prod-ue2

# Clean all generated files
atmos terraform generate files --clean --all

# Dry-run to see what would be deleted
atmos terraform generate files --clean --all --dry-run
```

### Integration with `terraform clean`

Following the existing convention for `auto_generate_backend_file`, when `auto_generate_files: true` is enabled, the generated files are **automatically included** in `terraform clean`:

```go
// internal/exec/terraform_clean.go - initializeFilesToClear()
func initializeFilesToClear(info schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) []string {
    files := []string{".terraform", varFile, planFile}

    // Existing: auto-generated backend
    if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
        files = append(files, "backend.tf.json")
    }

    // NEW: auto-generated files from generate section
    if atmosConfig.Components.Terraform.AutoGenerateFiles {
        // Get filenames from component's generate section
        generateFiles := getGenerateFilenames(info)
        files = append(files, generateFiles...)
    }

    return files
}
```

**No separate flag needed** - this follows the existing pattern where enabling auto-generation implicitly means clean should handle those files.

### Implementation Notes

- Generated files are tracked by filename from the `generate` section in the processed component config
- The `terraform clean` command accesses `info.ComponentSection["generate"]` to get the list of filenames
- Cleanup removes files that match the configured filenames in the component directory

## Documentation Updates Required

When implementing this feature, the following documentation must be updated to reflect that Atmos now supports declarative code generation:

### Files Requiring Updates

| File | Current Statement | Required Update |
|------|-------------------|-----------------|
| `website/docs/intro/faq.mdx` (line 95-97) | "Atmos does not promote code generation for root modules" | Clarify: Atmos now supports declarative file generation via the `generate` section, distinct from Terragrunt's HCL code generation |
| `website/docs/intro/features.mdx` (line 72) | "No code generation — configuration stays in YAML, HCL stays clean" | Update to reflect `generate` section capability while emphasizing declarative nature |
| `website/docs/intro/index.mdx` (line 97) | "No code generation, no templates" | Update messaging to distinguish from code generation approaches |
| `website/docs/learn/why-atmos.mdx` (line 39) | "No code generation or messy templating" | Clarify the declarative generation approach |
| `website/docs/learn/stacks/stacks.mdx` (line 26) | "without any code generation" | Update to reflect new capability |
| `website/docs/learn/mindset.mdx` (line 19) | "without the need to write any code or messy templates for code generation" | Refine messaging |
| `website/docs/best-practices/components.mdx` (line 146-157) | "Reserve Code Generation as an Escape Hatch for Emergencies" | Distinguish between arbitrary HCL code generation (discouraged) and declarative file generation (supported) |
| `website/docs/best-practices/stacks.mdx` (line 69-75) | "Reserve Code Generation for Stack Configuration" | Add reference to new `generate` section |
| `website/docs/migration/terragrunt.mdx` (line 309) | "Atmos intentionally does **not** provide full-fledged arbitrary file generation" | Update to document the new `generate` section as declarative alternative |

### Key Messaging Points

When updating documentation, emphasize these distinctions:

1. **Declarative vs. Imperative**: Atmos `generate` is declarative YAML configuration, not imperative HCL code generation like Terragrunt
2. **Purpose-Built**: The `generate` section extends the existing pattern of backend/provider generation
3. **Testable**: Generated files from declarative config are predictable and testable
4. **Exit Strategy**: Generated files can be committed and work with vanilla Terraform
5. **Not for Root Modules**: The advice against generating root module HCL code still applies - `generate` is for auxiliary files

### New Documentation to Create

| File | Purpose |
|------|---------|
| `website/docs/cli/commands/terraform/terraform-generate-files.mdx` | Command reference documentation |
| `website/docs/stacks/generate.mdx` | Conceptual documentation for generate section |

## Future Considerations (Out of Scope)

- Helmfile/Packer support (follow same pattern later)
- Checksum-based skip if unchanged
- Watch mode for development
- `.atmos-generated` manifest for precise stale file tracking
