# CI Summary Templates

## Overview

Rich markdown templates for CI job summaries, replacing the oversimplified table output with tfcmt-style templates featuring badges, collapsible sections, resource counts, and CI metadata.

## Problem Statement

The current CI summary output is a basic table that lacks the rich formatting seen in the GitHub Actions (`github-action-atmos-terraform-plan` and `github-action-atmos-terraform-apply`). Users expect:

- Status badges with resource counts (CREATE, CHANGE, REPLACE, DESTROY)
- Collapsible sections for terraform output
- Error highlighting and deletion warnings
- CI metadata (commit SHA, PR number, actor)
- Consistent branding

## Solution

A provider-based system where component types (terraform, helmfile, etc.) declare their CI integration behavior through hook bindings. Rich templates are embedded via `embed.FS` with layered override support.

## Architecture

### Component CI Provider Interface

```go
// ComponentCIProvider is implemented by component types that support CI integration.
type ComponentCIProvider interface {
    // Identity
    GetType() string  // "terraform", "helmfile"

    // Hook bindings - declares which events this provider handles
    GetHookBindings() []HookBinding

    // Templates
    GetDefaultTemplates() embed.FS
    BuildTemplateContext(info, ciCtx, output, command) (*TemplateContext, error)

    // Output parsing
    ParseOutput(output, command string) (*OutputResult, error)

    // CI outputs ($GITHUB_OUTPUT)
    GetOutputVariables(result *OutputResult, command string) map[string]string

    // Artifacts (planfiles)
    GetArtifactKey(info, command string) string
}
```

### Hook Bindings

Each provider declares which hook events it handles and what actions occur at each:

```go
type HookAction string

const (
    ActionSummary  HookAction = "summary"   // Write to $GITHUB_STEP_SUMMARY
    ActionOutput   HookAction = "output"    // Write to $GITHUB_OUTPUT
    ActionUpload   HookAction = "upload"    // Upload artifact (planfile)
    ActionDownload HookAction = "download"  // Download artifact
    ActionCheck    HookAction = "check"     // Validate/check
)

type HookBinding struct {
    Event    string       // "after.terraform.plan"
    Actions  []HookAction // [ActionSummary, ActionOutput, ActionUpload]
    Template string       // "plan" -> templates/plan.md
}
```

### Terraform Hook Bindings

| Event | Actions | Template | Purpose |
|-------|---------|----------|---------|
| `after.terraform.plan` | summary, output, upload | `plan` | Write summary, set outputs, upload planfile |
| `after.terraform.apply` | summary, output | `apply` | Write summary, set outputs |
| `before.terraform.apply` | download | - | Download planfile for apply |

## Configuration

```yaml
# atmos.yaml
ci:
  enabled: true
  templates:
    # Optional: base directory for custom templates
    base_path: "templates/ci"

    # Per-component-type template overrides
    terraform:
      plan: "terraform/plan.md"
      apply: "terraform/apply.md"

    helmfile:
      diff: "helmfile/diff.md"
      apply: "helmfile/apply.md"
```

### Template Override Precedence

1. Explicit file from config (`ci.templates.terraform.plan`)
2. Convention-based file from base_path (`{base_path}/terraform/plan.md`)
3. Embedded default templates

## Package Structure

```
pkg/ci/
├── component_provider.go     # ComponentCIProvider interface, HookBinding types
├── component_registry.go     # Provider registry (thread-safe)
├── component_registry_test.go
├── executor.go               # Execute() - unified action executor
├── templates/
│   └── loader.go             # Template loading with override support
└── terraform/
    ├── provider.go           # Self-registering via init()
    ├── provider_test.go
    ├── parser.go             # Parse plan/apply output
    ├── parser_test.go
    └── templates/
        ├── plan.md           # Default plan template
        └── apply.md          # Default apply template
```

## Template Context

Templates receive a `TemplateContext` with all available data:

```go
type TemplateContext struct {
    Component     string            // Component name
    ComponentType string            // "terraform", "helmfile"
    Stack         string            // Stack name
    Command       string            // "plan", "apply"
    CI            *Context          // CI platform metadata
    Result        *OutputResult     // Parsed output data
    Output        string            // Raw command output
    Custom        map[string]any    // Custom variables
}

type OutputResult struct {
    ExitCode   int
    HasChanges bool
    HasErrors  bool
    Errors     []string
    Data       any  // Component-specific (e.g., *TerraformOutputData)
}

type TerraformOutputData struct {
    ResourceCounts ResourceCounts  // Create, Change, Replace, Destroy
}
```

## Default Templates

### Plan Template Features

- Status icon (✅/⚠️/❌) based on changes/errors
- Resource count summary table with colored badges
- Collapsible terraform output section
- Error list when present
- CI metadata footer (stack, component, commit, PR, actor)

### Apply Template Features

- Success/failure status indicator
- Resource change summary (added, changed, destroyed)
- Collapsible apply output section
- Error details when present
- CI metadata footer

## Usage

### Automatic CI Integration

The `RunCIHooks()` function should be called after terraform commands:

```go
// In terraform execution flow
output, err := runTerraformPlan(...)
if err != nil {
    return err
}

// Automatically runs CI actions based on provider bindings
hooks.RunCIHooks(hooks.AfterTerraformPlan, atmosConfig, info, output)
```

### Self-Registration

Providers register themselves via `init()` and blank imports:

```go
// pkg/ci/terraform/provider.go
func init() {
    ci.RegisterComponentProvider(&Provider{})
}

// pkg/hooks/hooks.go
import (
    _ "github.com/cloudposse/atmos/pkg/ci/terraform"  // Register provider
)
```

## Schema Addition

```go
// pkg/schema/schema.go

type CITemplatesConfig struct {
    BasePath  string            `yaml:"base_path,omitempty"`
    Terraform map[string]string `yaml:"terraform,omitempty"`
    Helmfile  map[string]string `yaml:"helmfile,omitempty"`
}

type CIConfig struct {
    Enabled      bool              `yaml:"enabled,omitempty"`
    // ... existing fields ...
    Templates    CITemplatesConfig `yaml:"templates,omitempty"`
}
```

## Implementation Status

**Status: ✅ Complete**

All files specified in this PRD have been implemented and tested.

## Files Implemented

| File | Purpose | Status |
|------|---------|--------|
| `pkg/ci/component_provider.go` | `ComponentCIProvider` interface, `HookBinding`, `HookAction` types | ✅ Done |
| `pkg/ci/component_registry.go` | Thread-safe provider registry | ✅ Done |
| `pkg/ci/component_registry_test.go` | Registry tests | ✅ Done |
| `pkg/ci/executor.go` | Unified `Execute()` function | ✅ Done |
| `pkg/ci/executor_test.go` | Executor tests | ✅ Done |
| `pkg/ci/templates/loader.go` | Template loading with override support | ✅ Done |
| `pkg/ci/templates/loader_test.go` | Template loader tests | ✅ Done |
| `pkg/ci/terraform/provider.go` | Terraform CI provider (self-registering) | ✅ Done |
| `pkg/ci/terraform/provider_test.go` | Provider tests | ✅ Done |
| `pkg/ci/terraform/parser.go` | Parse terraform plan/apply output | ✅ Done |
| `pkg/ci/terraform/parser_test.go` | Parser tests | ✅ Done |
| `pkg/ci/terraform/context.go` | Terraform-specific template context | ✅ Done |
| `pkg/ci/terraform/template_test.go` | Template rendering tests | ✅ Done |
| `pkg/ci/terraform/templates/plan.md` | Default plan template | ✅ Done |
| `pkg/ci/terraform/templates/apply.md` | Default apply template | ✅ Done |

## Files Modified

| File | Changes |
|------|---------|
| `pkg/schema/schema.go` | Added `CITemplatesConfig` struct |
| `pkg/hooks/hooks.go` | Added `RunCIHooks()` function, terraform provider import |

## Future Enhancements

1. **Helmfile Provider** - Implement `pkg/ci/helmfile/` with diff/apply templates
2. **Infracost Integration** - Add cost estimation data to template context
3. **Custom Template Functions** - Expose atmos-specific template functions
4. **PR Comments** - Extend to support PR comment posting (beyond job summaries)
5. **Drift Detection** - Implement `ActionCheck` for scheduled drift detection

## Testing

All components include comprehensive tests:

- `pkg/ci/component_registry_test.go` - Registry operations, binding lookup
- `pkg/ci/terraform/parser_test.go` - Output parsing for various terraform outputs
- `pkg/ci/terraform/provider_test.go` - Provider interface implementation

Run tests:
```bash
go test ./pkg/ci/... -v
```
