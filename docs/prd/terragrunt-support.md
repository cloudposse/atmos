# Terragrunt Support in Atmos - PRD

## Problem Statement

Organizations with existing Terragrunt projects face a significant barrier when adopting Atmos. Large Terragrunt codebases can take weeks or months to fully migrate, delaying access to Atmos benefits like enhanced stack management, workflow automation, and the terminal UI. This creates an all-or-nothing adoption scenario that increases risk and extends time-to-value.

Additionally, users familiar with Terragrunt concepts (units, stacks, `dependency{}`, `generate{}`, `locals{}`) need a bridge to understand how these map to Atmos equivalents.

## Proposed Solution

Add **native Terragrunt import support** to Atmos, allowing existing Terragrunt configurations to be imported directly into Atmos stacks and operated on as though they were native Atmos components.

```yaml
# stacks/dev.yaml
import:
  - catalog/defaults
  - terragrunt://live/dev/vpc/terragrunt.hcl        # Import a Terragrunt unit
  - terragrunt://live/dev/terragrunt.stack.hcl     # Import a Terragrunt stack
```

Imported Terragrunt configs merge into the current stack as `components.terraform` entries, enabling teams to:

1. **Reduce time to first value** - Start using Atmos immediately without waiting for a complete migration
2. **Lower migration urgency** - Convert Terragrunt configs to native YAML incrementally, at your own pace
3. **Mix and match** - Run Terragrunt-sourced and native Atmos components side-by-side in the same stack
4. **Familiar concepts** - Terragrunt users see how their patterns map to Atmos equivalents

### Concept Mapping

| Terragrunt | Atmos Equivalent |
|------------|------------------|
| Unit (`terragrunt.hcl`) | `components.terraform.<name>` |
| Stack (`terragrunt.stack.hcl`) | Multiple components in a stack |
| `inputs{}` | `vars:` |
| `locals{}` | `locals:` |
| `dependency{}` | `settings.depends_on` |
| `dependency.X.outputs.Y` | `!terraform.state` |
| `generate{}` | `generate:` |
| `get_aws_account_id()` | `!aws.account_id` |

## Two-Phase Approach

**Phase 1: Import Support (This Document)**
- `import: - terragrunt://path/to/terragrunt.hcl`
- Merges into the importing stack (no new stacks created)
- Components become standard `components.terraform` entries
- Uses existing Atmos primitives: `settings.depends_on`, `!terraform.state`

**Phase 2: Migration Command (Future, Separate Work)**
- `atmos migrate terragrunt <source>` to generate native Atmos YAML files
- Higher risk, more work - transforms configs permanently
- Out of scope for this document

## Registry Pattern for Extensibility

The import adapter will follow a **registry pattern** so we can support other systems (Pulumi, CDK, etc.) using the same interface.

## Example

```yaml
# stacks/dev.yaml
import:
  - catalog/defaults

  # Import a single Terragrunt unit → merges into THIS stack
  - terragrunt://live/dev/vpc/terragrunt.hcl

  # Import a Terragrunt stack → merges ALL components into THIS stack
  - terragrunt://live/dev/terragrunt.stack.hcl

# Can still have native Atmos components alongside
components:
  terraform:
    monitoring:  # Native Atmos component
      vars:
        enabled: true
```

**After import processing, the terragrunt imports synthesize into the SAME stack:**
```yaml
components:
  terraform:
    vpc:        # from terragrunt://live/dev/vpc/terragrunt.hcl
      vars: { ... from inputs{} }
      settings:
        depends_on: { ... from dependency{} }
```

---

## Prerequisites

Before implementing the Terragrunt import adapter, five prerequisite features must be added to Atmos. These are valuable independently and will be implemented as separate PRs.

### PR 1: AWS YAML Functions ✅ COMPLETED (#1843)

Add new YAML functions for AWS identity information:

```yaml
vars:
  account_id: !aws.account_id
  caller_arn: !aws.caller_identity_arn
  user_id: !aws.caller_identity_user_id
  region: !aws.region
```

**Status:** Merged. Also includes `!aws.region` which retrieves the current AWS region.

### PR 2: Native `generate:` Section 🔄 PR OPEN

Add file generation capability to components:

```yaml
components:
  terraform:
    vpc:
      generate:
        backend.tf: |
          terraform {
            backend "s3" {
              bucket = "{{ .vars.bucket_name }}"
            }
          }
        provider.tf: |
          provider "aws" {
            region = "{{ .vars.aws_region }}"
          }
```

**Implementation:**
- Add `Generate map[string]string` to component schema
- Write files to working directory before terraform runs
- Content supports Go templates with access to `.vars`, `.locals`, `.settings`

**Files:** `pkg/schema/schema.go`, `internal/exec/terraform.go`

### PR 3: `source` (Just-in-Time Vendoring) ✅ COMPLETED

Add support for remote terraform sources at the component level:

```yaml
components:
  terraform:
    vpc:
      metadata:
        component: vpc
      source: "git::git@github.com:acme/modules.git//vpc?ref=v1.0.0"
```

**Status:** Merged. Components can now specify remote sources that are downloaded just-in-time before terraform execution using the existing go-getter infrastructure.

### PR 4: Native `locals:` Section ✅ COMPLETED (#1883)

Add transient variables for intermediate calculations:

```yaml
locals:
  full_name: "{{ .vars.namespace }}-{{ .vars.environment }}-vpc"

components:
  terraform:
    vpc:
      vars:
        vpc_name: "{{ .locals.full_name }}"
```

**Status:** Merged as file-scoped locals with dependency resolution and circular dependency detection. Locals can be defined at global, component-type, and component scopes.

### PR 5: Import Adapter Registry ✅ COMPLETED

Create the extensible import adapter framework with a mock adapter for testing:

```go
// pkg/import/adapter.go
type ImportAdapter interface {
    Scheme() string
    Parse(ctx context.Context, path string, opts ParseOptions) (*ImportResult, error)
    PreExecute(ctx context.Context, component string, workDir string) error
}
```

**Status:** Merged. The import adapter registry is in place with `Register()` and `GetAdapter()` functions, enabling custom import adapters to be registered for different schemes.

### PR 6: Terragrunt Import Adapter (This PRD)

With prerequisites 1-5 in place, implement the Terragrunt adapter:

- `pkg/import/terragrunt/` package
- HCL parsing for units and stacks
- Synthesizer to convert to Atmos components
- Test fixture in `tests/fixtures/terragrunt/`

---

## Part 0: Architecture Overview

### Design Principles

1. **Import, not component type** - `terragrunt://` is a parser hint in imports, not a new component type
2. **Merge into current stack** - Imported configs become part of the stack where the import appears
3. **Use existing Atmos primitives** - Map to `settings.depends_on`, `!terraform.state`, etc.
4. **No Terragrunt binary** - Atmos parses HCL directly, executes terraform
5. **Registry pattern** - Extensible interface for future adapters (Pulumi, CDK, etc.)

### New Atmos Features Required

This PRD requires the prerequisite features documented above (PRs 1-5). These are valuable independently of Terragrunt support and will be implemented as separate PRs before the Terragrunt adapter.

### Terragrunt Concepts → Atmos Mapping

| Terragrunt | Atmos Equivalent | Notes |
|------------|------------------|-------|
| **Unit** (`terragrunt.hcl`) | `components.terraform.<name>` | Merges into current stack |
| **Stack** (`terragrunt.stack.hcl`) | Multiple `components.terraform` | All merge into current stack |
| Stack `unit` block label | Component instance name (key) | e.g., `vpc-primary` |
| Stack `unit.path` | `metadata.component` | Shared component directory |
| Stack `unit.source` | `source:` | Just-in-time vendoring target |
| Stack `unit.values` | `vars:` | Component variables |
| `inputs{}` | `vars:` | Direct translation |
| `dependency{}` | `settings.depends_on` | Existing Atmos feature |
| `dependency.X.outputs.Y` | `!terraform.state` | Existing YAML function |
| `include{}` | Internal processing | Adapter follows & merges |
| `generate{}` | `generate:` section | Keys are filenames, values are content |
| `terraform.source` | `source:` | Just-in-time vendoring |
| `locals{}` | `locals:` section | ✅ Implemented |
| `remote_state{}` | `!terraform.state` | Existing YAML function |
| `get_aws_account_id()` | `!aws.account_id` | ✅ Implemented |

### Import Adapter Registry Pattern

```go
// pkg/import/adapter.go - Registry interface for import adapters
type ImportAdapter interface {
    // Scheme returns the URL scheme this adapter handles (e.g., "terragrunt")
    Scheme() string

    // Parse reads the source and returns components to merge into the stack
    Parse(ctx context.Context, path string, opts ParseOptions) (*ImportResult, error)

    // PreExecute runs before terraform (e.g., generate{} blocks, vendoring)
    PreExecute(ctx context.Context, component string, workDir string) error
}

type ImportResult struct {
    Components map[string]any  // components.terraform entries to merge
    Warnings   []string        // Non-fatal issues to report
}

// Registry of import adapters
var adapters = make(map[string]ImportAdapter)

func Register(adapter ImportAdapter) {
    adapters[adapter.Scheme()] = adapter
}

func GetAdapter(scheme string) (ImportAdapter, bool) {
    adapter, ok := adapters[scheme]
    return adapter, ok
}
```

This pattern allows:
- `terragrunt://` adapter (this document)
- Future `pulumi://` adapter
- Future `cdktf://` adapter
- Custom adapters via plugins

---

## Part 1: Implementation Approach

### Extend Import Processing

The import processor already handles `import:` entries. We extend it to recognize the `terragrunt://` prefix:

```go
// In import processing (internal/exec/stack_processor_utils.go or similar)
func processImport(importPath string) (map[string]any, error) {
    if strings.HasPrefix(importPath, "terragrunt://") {
        path := strings.TrimPrefix(importPath, "terragrunt://")
        return processTerragruntImport(path)
    }
    // ... existing YAML import logic
}

func processTerragruntImport(path string) (map[string]any, error) {
    // 1. Parse terragrunt.hcl (or terragrunt.stack.hcl)
    // 2. Follow include{} chains, merge configs
    // 3. Synthesize components.terraform entries
    // 4. Return as map to merge into stack
}
```

### New Package: `pkg/terragrunt/`

```go
// pkg/terragrunt/parser.go
type Config struct {
    Terraform    *TerraformBlock
    Inputs       map[string]any
    Locals       map[string]any
    Dependencies []DependencyBlock
    Includes     []IncludeBlock
    Generate     []GenerateBlock
    RemoteState  *RemoteStateBlock
}

func Parse(filename string) (*Config, error)
func ParseWithIncludes(filename string) (*Config, error)  // Follows include{}

// pkg/terragrunt/synthesizer.go
type Synthesizer struct{}

func (s *Synthesizer) ToAtmosComponent(cfg *Config, name string) (map[string]any, []Warning, error)
func (s *Synthesizer) ToAtmosComponents(stackCfg *StackConfig) (map[string]any, []Warning, error)
```

### Processing Flow

```
import:
  - terragrunt://live/dev/vpc/terragrunt.hcl
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ 1. Detect terragrunt:// prefix                          │
│    - Route to Terragrunt import handler                 │
└─────────────────────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ 2. Parse terragrunt.hcl                                  │
│    - Follow include{} chains                            │
│    - Evaluate locals{} expressions                       │
│    - Merge configs (like Terragrunt does)               │
└─────────────────────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ 3. Synthesize Atmos component                            │
│    - inputs{} → vars                                    │
│    - dependency{} → depends_on + !terraform.output      │
│    - Component name from directory                       │
│    - Warn on unsupported features                       │
└─────────────────────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ 4. Return as components.terraform.<name>                 │
│    - Merged into stack like any other import            │
└─────────────────────────────────────────────────────────┘
      │
      ▼
(Later, at execution time)
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ 5. Handle generate{} blocks                              │
│    - Write files to working directory                   │
└─────────────────────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ 6. Handle terraform.source                               │
│    - Just-in-time vendor/download                       │
└─────────────────────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ 7. Execute terraform                                     │
│    - Normal atmos terraform plan/apply/destroy          │
└─────────────────────────────────────────────────────────┘
```

---

## Part 2: Terragrunt Function Handling

### Functions the Adapter Must Evaluate

| Category | Functions | Handling |
|----------|-----------|----------|
| **Path functions** | `find_in_parent_folders()`, `get_terragrunt_dir()`, etc. | Evaluate during `include` resolution |
| **Environment** | `get_env()` | Map to `!env` or evaluate |
| **AWS Identity** | `get_aws_account_id()`, etc. | ⚠️ **Should add to Atmos** - valuable independently |
| **Shell execution** | `run_cmd()` | Map to `!exec` or evaluate |
| **Config reading** | `read_terragrunt_config()` | Handle during include processing |

### Functions We Don't Need to Implement

| Function | Reason |
|----------|--------|
| `get_terraform_command()` | Runtime - Atmos controls this |
| `get_terraform_cli_args()` | Runtime - Atmos controls this |
| `path_relative_to_include()` | Only used inside TG, we just evaluate the result |
| `mark_as_read()` | Terragrunt-internal |

---

## Part 3: Key Implementation Areas

Based on our analysis, there are five areas requiring implementation work:

### Area 1: Native Atmos `generate:` Section

**New Atmos Feature:** Add `generate:` section under `components.terraform.<name>` where keys are filenames and values are file content.

```yaml
components:
  terraform:
    vpc:
      generate:
        backend.tf: |
          terraform {
            backend "s3" {
              bucket = "{{ .vars.bucket_name }}"
              key    = "{{ .vars.environment }}/vpc/terraform.tfstate"
            }
          }
        provider.tf: |
          provider "aws" {
            region = "{{ .vars.aws_region }}"
          }
```

**Implementation:**
- Add `Generate map[string]string` to component schema
- Write files to working directory before terraform runs
- Content supports Go templates with access to `.vars`, `.locals`, `.settings`

**Terragrunt Mapping:**
- Terragrunt `generate{}` blocks → Atmos `generate:` section
- Extract `path` → key, `contents` → value
- May need to rewrite Terragrunt template syntax → Go template syntax
- `if_exists` behavior: Atmos always overwrites (simpler model)

### Area 2: Native Atmos `locals:` Section

**New Atmos Feature:** Add `locals:` section for transient variables and intermediate calculations.

```yaml
components:
  terraform:
    vpc:
      locals:
        full_name: "{{ .vars.namespace }}-{{ .vars.environment }}-vpc"
        tags_json: |
          {{ .vars.tags | toJson }}

      vars:
        vpc_name: "{{ .locals.full_name }}"

      generate:
        tags.auto.tfvars.json: "{{ .locals.tags_json }}"
```

**Implementation:**
- Add `Locals map[string]any` to component schema
- Evaluate locals before vars (locals can use vars, vars can use locals)
- Support Go templates in local values
- Available in `.locals` context for templates

**Terragrunt Mapping:**
- Terragrunt `locals{}` → Atmos `locals:` section
- Evaluate HCL expressions → convert to Go template syntax where needed

### Area 3: New `!aws.account_id` YAML Function

**New Atmos Feature:** YAML function to get current AWS account ID.

```yaml
vars:
  account_id: !aws.account_id
  # Optionally with profile/region
  other_account: !aws.account_id
    profile: production
```

**Implementation:**
- Add to YAML function registry alongside `!terraform.state`, `!env`, etc.
- Call AWS STS GetCallerIdentity
- Cache result per profile/region combination
- No arguments = use default credentials

**Terragrunt Mapping:**
- `get_aws_account_id()` → `!aws.account_id`
- `get_aws_caller_identity_arn()` → `!aws.caller_identity_arn` (future)
- `get_aws_caller_identity_user_id()` → `!aws.caller_identity_user_id` (future)

### Area 4: Terragrunt HCL Function Evaluation

**Functions handled during Terragrunt import parsing:**

| Terragrunt Function | Atmos Mapping |
|---------------------|---------------|
| `get_env(NAME, DEFAULT)` | Evaluate → static value (or `!env`) |
| `get_aws_account_id()` | Convert to `!aws.account_id` |
| `run_cmd(cmd, args...)` | Evaluate → static value (or `!exec`) |
| `get_terragrunt_dir()` | Evaluate → static path |
| `get_parent_terragrunt_dir()` | Evaluate → static path |
| `read_terragrunt_config(path)` | Handle during include processing |

**Functions we DON'T need to implement:**
- `find_in_parent_folders()` - Terragrunt's import mechanism, we get final path
- `path_relative_to_include()` - Internal TG use, we get final value
- `get_terraform_command()` - Runtime, Atmos controls
- `get_terraform_cli_args()` - Runtime, Atmos controls

**Template Syntax Rewriting:**
Terragrunt uses HCL interpolation (`${...}`), Atmos uses Go templates (`{{ ... }}`):
```
# Terragrunt
"${local.region}-${var.env}"

# Atmos (after conversion)
"{{ .locals.region }}-{{ .vars.env }}"
```

### Area 5: Just-in-Time Vendoring for `terraform.source`

**Challenge:** Terragrunt's `terraform { source = "..." }` points to remote modules that need downloading.

**Approach:** Leverage existing Atmos vendoring infrastructure with on-demand triggering.

```go
// pkg/terragrunt/vendor.go
type SourceResolver struct {
    downloader *downloader.Downloader
    cacheDir   string
}

func (r *SourceResolver) ResolveSource(source string) (string, error) {
    // 1. Parse source URL (git::, s3::, etc. - same as go-getter)
    // 2. Check if already cached
    // 3. Download if needed using existing downloader
    // 4. Return local path
}
```

**Integration point:** During terraform execution, before running terraform commands:
1. Check if component has `terraform.source` from Terragrunt config
2. If remote source, resolve/download to cache
3. Set working directory or copy to component dir

**Existing infrastructure to leverage:**
- `pkg/downloader/` - Already uses go-getter (same format as Terragrunt)
- Vendor command patterns for caching and checksums

---

## Part 4: Phased Implementation Plan

### Phase 1: Core Parser & Basic Import (MVP)

**Scope:** Parse both `terragrunt.hcl` (units) and `terragrunt.stack.hcl` (stacks), extract `inputs{}`, synthesize components

**Deliverables:**
- `pkg/import/adapter.go` - ImportAdapter interface + registry
- `pkg/import/terragrunt/parser.go` - HCL block parsing (units + stacks)
- `pkg/import/terragrunt/config.go` - Config struct definitions
- `pkg/import/terragrunt/synthesizer.go` - Convert to Atmos components
- Import processor extension for `terragrunt://` prefix

**What works after Phase 1:**
```yaml
import:
  # Import a single unit → one component
  - terragrunt://live/dev/vpc/terragrunt.hcl

  # Import a stack → multiple components merged into this stack
  - terragrunt://live/dev/terragrunt.stack.hcl
```

### Phase 2: Include Resolution & Dependencies

**Scope:** Follow `include{}` chains, handle `dependency{}` blocks

**Deliverables:**
- Include chain following with config merging
- `dependency{}` → `settings.depends_on` synthesis
- `dependency.X.outputs.Y` → `!terraform.output X Y` translation
- Basic HCL function evaluation context

**What works after Phase 2:**
```yaml
import:
  - terragrunt://live/dev/vpc/terragrunt.hcl
  # Now includes are followed, dependencies resolved
```

### Phase 3: Generate Blocks & Full Function Support

**Scope:** Execute `generate{}` blocks, complete function support

**Deliverables:**
- `pkg/terragrunt/generator.go` - Write generate blocks
- Complete HCL function implementations
- AWS identity functions (`get_aws_account_id()`, etc.)
- Integration with terraform execution flow

### Phase 4: Just-in-Time Vendoring

**Scope:** Handle `terraform.source` remote modules

**Deliverables:**
- `pkg/terragrunt/vendor.go` - Source resolution
- Cache management for downloaded modules
- Integration with existing downloader infrastructure

---

## Part 5: Package Structure & Critical Files

### New Packages

```
pkg/
  import/                        # NEW: Import adapter registry
    adapter.go                   # ImportAdapter interface + registry
    adapter_test.go

  import/terragrunt/             # NEW: Terragrunt adapter implementation
    adapter.go                   # Implements ImportAdapter
    parser.go                    # Parse terragrunt.hcl files
    config.go                    # Config struct definitions
    functions.go                 # HCL function implementations
    generator.go                 # Generate block execution
    vendor.go                    # terraform.source resolution
    adapter_test.go
```

### Critical Files to Modify

**New Native Atmos Features:**

| File | Change |
|------|--------|
| `pkg/schema/schema.go` | Add `Locals`, `Generate` to component schema |
| `internal/exec/template_funcs.go` | Add `!aws.account_id` YAML function |
| `internal/exec/terraform.go` | Write `generate:` files before terraform runs |
| `internal/exec/stack_processor_utils.go` | Process `locals:` section, extend import handling |

**Import Adapter Registry:**

| File | Change | Phase |
|------|--------|-------|
| `pkg/import/adapter.go` | **NEW** - ImportAdapter interface + registry | 1 |
| `pkg/import/terragrunt/adapter.go` | **NEW** - Implements ImportAdapter | 1 |
| `pkg/import/terragrunt/parser.go` | **NEW** - Parse terragrunt.hcl files | 1 |
| `pkg/import/terragrunt/config.go` | **NEW** - Config struct definitions | 1 |
| `pkg/import/terragrunt/synthesizer.go` | **NEW** - Convert to Atmos components | 1 |
| `pkg/import/terragrunt/functions.go` | **NEW** - HCL function evaluation | 2 |
| `pkg/import/terragrunt/rewriter.go` | **NEW** - HCL `${...}` → Go `{{ ... }}` | 2 |
| `pkg/import/terragrunt/vendor.go` | **NEW** - terraform.source resolution | 3 |

---

## Summary

### What This Enables

1. **Day 1:** Import existing Terragrunt configs via `terragrunt://` - merges into current stack
2. **Progressive:** Convert individual imports to native YAML as desired
3. **Mixed:** Terragrunt-sourced and native components coexist in same stack
4. **Extensible:** Registry pattern enables future adapters (Pulumi, CDK, etc.)

### Key Design Decisions (Resolved)

| Decision | Resolution |
|----------|------------|
| How to integrate? | Import mechanism with `terragrunt://` prefix |
| Where do imports go? | Merge into the importing stack (no new stacks created) |
| Component type? | No - synthesizes to `components.terraform` |
| Dependencies? | Use existing `settings.depends_on` |
| Remote state refs? | Use existing `!terraform.state` YAML function |
| `generate{}` handling | **NEW** native `generate:` section (keys=filenames, values=content) |
| `locals{}` handling | **NEW** native `locals:` section for transient variables |
| `get_aws_account_id()` | **NEW** `!aws.account_id` YAML function |
| Template syntax | Rewrite HCL `${...}` → Go template `{{ ... }}` |
| `terraform.source` | Just-in-time vendor using existing downloader |
| Terragrunt binary? | Not required - Atmos parses HCL directly |
| Extensibility? | Registry pattern for import adapters |

### Feasibility Assessment

| Aspect | Feasibility | Notes |
|--------|-------------|-------|
| Basic `inputs{}` extraction | ✅ Easy | HCL parsing exists in `pkg/filetype/` |
| `include{}` resolution | ✅ Medium | Recursive parsing, config merging |
| `dependency{}` → `depends_on` | ✅ Easy | Direct mapping to existing feature |
| `dependency.X.outputs.Y` → `!terraform.state` | ✅ Medium | Expression rewriting |
| Native `generate:` section | ✅ Medium | Schema change + file writing |
| Native `locals:` section | ✅ Medium | Schema change + template context |
| `!aws.account_id` function | ✅ Easy | STS GetCallerIdentity call |
| Template syntax rewriting | ⚠️ Medium | HCL `${...}` → Go `{{ ... }}` |
| Just-in-time vendoring | ✅ Medium | Leverage existing downloader |
| Full Terragrunt parity | ⚠️ Hard | Diminishing returns, not a goal |

### Scope Clarification

**Phase 1 (This Document): Import Support**
- `import: - terragrunt://...` merges into current stack
- No YAML files generated - runtime interpretation only

**Phase 2 (Future): Migration Command**
- `atmos migrate terragrunt` to generate native Atmos YAML
- Separate work, higher risk, optional

### Limitations (Explicitly Not Supported)

The following Terragrunt features have **no equivalent** and will produce warnings during import:

| Terragrunt Feature | Reason |
|--------------------|--------|
| `retry_max_attempts` / `retryable_errors` | Terragrunt-specific retry logic; terraform handles its own retries |
| `terraform_version_constraint` | No constraint system; future Atmos toolchain will pin exact versions |
| `get_terraform_command()` | Runtime function returning current command ("plan", "apply"); Atmos hooks have native context |
| `get_terraform_cli_args()` | Runtime function returning CLI args; Atmos hooks have native context |
| `get_original_terragrunt_dir()` | Terragrunt-internal; not meaningful after import |
| `download_dir` | Terragrunt-internal; Atmos manages working directories |
| `sops_decrypt_file()` | Terragrunt-specific; use Atmos SOPS integration if available |

### Mapped to Atmos Equivalents (Automatic Conversion)

These Terragrunt features **map directly** to Atmos equivalents during import:

| Terragrunt Feature | Atmos Equivalent |
|--------------------|------------------|
| `before_hook` / `after_hook` | `hooks:` in component config |
| `extra_arguments` | `command:` or `env:` settings |
| `skip` | `metadata.enabled: false` |
| `prevent_destroy` | Use `prevent_destroy` in Terraform code (lifecycle) |
| `iam_role` / `iam_assume_role_duration` | Atmos profiles or AWS SDK credential resolution |
| `iam_web_identity_token` | Atmos profiles or AWS SDK credential resolution |
| `terraform_binary` | `terraform.command` in atmos.yaml |
| `run_cmd()` | `!exec` YAML function |
| `read_tfvars_file()` | Convert to `vars:` |
| `deep_merge()` | Native Atmos deep merge |
| Mock outputs in `dependency{}` | `!terraform.state` and `!terraform.output` support mocks |

**Behavior Differences:**

| Behavior | Terragrunt | Atmos |
|----------|------------|-------|
| `generate{}` `if_exists` | `overwrite`, `skip`, `error`, etc. | Always overwrites (simpler model) |
| `generate{}` signature comment | Writes Terragrunt signature | No signature comment |
| `include{}` merge strategy | `shallow`, `deep`, `no_merge` | Always deep merge |
| Dependency mock outputs | Supported for `plan` without `apply` | Not supported initially |
| Parallel execution | `terragrunt run-all` CLI command | `atmos terraform plan --all` / `apply --all` flags |

### Test Fixture: Comprehensive Terragrunt Configuration

Create a comprehensive Terragrunt fixture in `tests/fixtures/terragrunt/` that exercises **all** Terragrunt features to verify:
1. Supported features convert correctly to Atmos equivalents
2. Unsupported features produce appropriate warnings during import

**Fixture Structure:**
```
tests/fixtures/terragrunt/
├── terragrunt.hcl                    # Root config with common settings
├── terragrunt.stack.hcl              # Stack definition
├── _envcommon/
│   └── vpc.hcl                       # Shared config for include{}
├── live/
│   ├── terragrunt.hcl                # Environment root (find_in_parent_folders target)
│   ├── dev/
│   │   ├── terragrunt.hcl            # Dev environment config
│   │   ├── terragrunt.stack.hcl      # Dev stack definition
│   │   ├── vpc/
│   │   │   └── terragrunt.hcl        # VPC unit - basic inputs
│   │   ├── eks/
│   │   │   └── terragrunt.hcl        # EKS unit - dependencies on vpc
│   │   ├── rds/
│   │   │   └── terragrunt.hcl        # RDS unit - multiple dependencies
│   │   └── app/
│   │       └── terragrunt.hcl        # App unit - complex config
│   └── prod/
│       └── ... (similar structure)
└── modules/                          # Local modules for terraform.source
    └── vpc/
        └── main.tf
```

**Features to Exercise in Fixtures:**

| Category | Feature | File Location |
|----------|---------|---------------|
| **Core (Supported)** | | |
| | `inputs{}` | All unit configs |
| | `locals{}` | `live/dev/vpc/terragrunt.hcl` |
| | `include{}` with `find_in_parent_folders()` | All unit configs |
| | `dependency{}` with output references | `live/dev/eks/terragrunt.hcl` |
| | `dependencies{}` (order-only) | `live/dev/app/terragrunt.hcl` |
| | `terraform { source = "..." }` (local) | `live/dev/vpc/terragrunt.hcl` |
| | `terraform { source = "..." }` (remote git) | `live/dev/eks/terragrunt.hcl` |
| | `generate{}` backend | `live/terragrunt.hcl` |
| | `generate{}` provider | `live/terragrunt.hcl` |
| | `generate{}` arbitrary file | `live/dev/app/terragrunt.hcl` |
| | `remote_state{}` | `live/terragrunt.hcl` |
| | `skip` | `live/dev/app/terragrunt.hcl` |
| **Functions (Supported)** | | |
| | `get_env()` | `live/terragrunt.hcl` |
| | `get_aws_account_id()` | `live/terragrunt.hcl` |
| | `get_aws_caller_identity_arn()` | `live/terragrunt.hcl` |
| | `run_cmd()` | `live/dev/vpc/terragrunt.hcl` |
| | `get_terragrunt_dir()` | `live/dev/vpc/terragrunt.hcl` |
| | `get_parent_terragrunt_dir()` | `live/dev/vpc/terragrunt.hcl` |
| | `path_relative_to_include()` | `_envcommon/vpc.hcl` |
| | `read_terragrunt_config()` | `live/dev/eks/terragrunt.hcl` |
| **Hooks (Mapped)** | | |
| | `before_hook` | `live/dev/vpc/terragrunt.hcl` |
| | `after_hook` | `live/dev/vpc/terragrunt.hcl` |
| | `error_hook` | `live/dev/vpc/terragrunt.hcl` |
| **Terraform Settings (Mapped)** | | |
| | `extra_arguments` | `live/terragrunt.hcl` |
| | `terraform_binary` | `live/terragrunt.hcl` |
| **AWS Auth (Mapped)** | | |
| | `iam_role` | `live/prod/terragrunt.hcl` |
| | `iam_assume_role_duration` | `live/prod/terragrunt.hcl` |
| **Unsupported (Should Warn)** | | |
| | `retry_max_attempts` | `live/terragrunt.hcl` |
| | `retryable_errors` | `live/terragrunt.hcl` |
| | `terraform_version_constraint` | `live/terragrunt.hcl` |
| | `download_dir` | `live/terragrunt.hcl` |
| | `prevent_destroy` | `live/dev/rds/terragrunt.hcl` |
| | `get_terraform_command()` in hook | `live/dev/vpc/terragrunt.hcl` |
| | `get_terraform_cli_args()` in hook | `live/dev/vpc/terragrunt.hcl` |
| | `get_original_terragrunt_dir()` | `live/dev/vpc/terragrunt.hcl` |
| | `sops_decrypt_file()` | `live/dev/app/terragrunt.hcl` |
| | Mock outputs in dependency | `live/dev/eks/terragrunt.hcl` |

**Test Cases:**

1. **Import Single Unit** - `terragrunt://live/dev/vpc/terragrunt.hcl`
   - Verify `inputs{}` → `vars:`
   - Verify `locals{}` → `locals:`
   - Verify `generate{}` → `generate:`
   - Verify warnings for unsupported features

2. **Import Unit with Dependencies** - `terragrunt://live/dev/eks/terragrunt.hcl`
   - Verify `dependency{}` → `settings.depends_on`
   - Verify `dependency.vpc.outputs.vpc_id` → `!terraform.state`

3. **Import Stack** - `terragrunt://live/dev/terragrunt.stack.hcl`
   - Verify all units merge into single stack
   - Verify dependency ordering preserved

4. **Warning Aggregation** - Import config with all unsupported features
   - Verify each unsupported feature produces a warning
   - Verify warning includes feature name and location
   - Verify import still succeeds (warnings, not errors)

### Next Steps

1. Create comprehensive Terragrunt test fixture (`tests/fixtures/terragrunt/`)
2. Implement import adapter registry pattern (`pkg/import/adapter.go`)
3. Implement Terragrunt adapter - parser for units + stacks, synthesizer
4. Validate with test fixture
5. Add include resolution & dependency handling (Phase 2)
6. Add `generate{}` blocks and vendoring support (Phases 3-4)
