# Restore `pkg/component` Public API for `terraform-provider-utils`

**Related Provider Issue:** `cloudposse/terraform-provider-utils` — `data "utils_component_config"` crashes with
"Plugin did not respond"

**Affected Atmos Version:** v1.201.0+ (API removed), impacts provider v1.31.0 (embeds Atmos v1.189.0)

**Severity:** Critical — blocks all Terraform components using `cloudposse/stack-config/yaml//modules/remote-state`

## Background

The `cloudposse/utils` Terraform provider exposes Atmos capabilities as Terraform data sources. The
`data "utils_component_config"` data source is used by the `cloudposse/stack-config/yaml//modules/remote-state`
module to resolve component backend configuration for cross-component state lookups. This module is widely used —
for example, in one affected repo it appears in 56 components.

The provider imports two public functions from `pkg/component`:

```go
import p "github.com/cloudposse/atmos/pkg/component"

// Called when stack name is provided directly:
result, err = p.ProcessComponentInStack(component, stack, atmosCliConfigPath, atmosBasePath)

// Called when context (namespace/tenant/environment/stage) is provided:
result, err = p.ProcessComponentFromContext(component, namespace, tenant, environment, stage, atmosCliConfigPath, atmosBasePath)
```

Source: `terraform-provider-utils/internal/provider/data_source_component_config.go` (lines 118, 123)

## What Was Removed

In **v1.201.0** (commit `a10d9ad23`, PR #1774 — "feat: Path-based component resolution for all commands"),
the file `pkg/component/component_processor.go` was deleted entirely. This file contained the two public
functions above.

The `pkg/component` package was redesigned from a component processing API into a component
registry/provider abstraction:

| Old File (deleted)       | New Files                                     |
|--------------------------|-----------------------------------------------|
| `component_processor.go` | `provider.go` — `ComponentProvider` interface |
|                          | `registry.go` — `ComponentRegistry`           |
|                          | `resolver.go` — path-based component resolver |
|                          | `path.go` — `IsExplicitComponentPath` helper  |

The processing logic was moved to `internal/exec/describe_component.go`, specifically:

- `ExecuteDescribeComponent(params *ExecuteDescribeComponentParams) (map[string]any, error)` — line 214
- `ExecuteDescribeComponentWithContext(params DescribeComponentContextParams) (*DescribeComponentResult, error)` — line
  456

These are `internal` and cannot be imported by external Go modules like `terraform-provider-utils`.

A public wrapper already exists at `pkg/describe/describe_component.go`:

```go
func ExecuteDescribeComponent(component, stack string, processTemplates, processYamlFunctions bool, skip []string) (map[string]any, error)
```

However, this wrapper does not accept `atmosCliConfigPath` or `atmosBasePath` parameters, which the
provider needs to pass through for config initialization.

## What the Deleted Functions Did

### `ProcessComponentInStack`

1. Set `ConfigAndStacksInfo.ComponentFromArg`, `.Stack`, `.AtmosCliConfigPath`, `.AtmosBasePath`
2. Called `cfg.InitCliConfig(configAndStacksInfo, true)` to initialize Atmos configuration
3. Tried `e.ProcessStacks()` with component type Terraform
4. On failure, fell back to Helmfile, then Packer
5. Returned `configAndStacksInfo.ComponentSection` (fully-resolved component config map)

### `ProcessComponentFromContext`

1. Initialized Atmos configuration (same as above)
2. Resolved `(namespace, tenant, environment, stage)` into a stack name using either
   `stacks.name_template` (Go template) or `stacks.name_pattern` (placeholder pattern) from `atmos.yaml`
3. Delegated to `ProcessComponentInStack` with the resolved stack name

## Impact on Users

When users upgrade their Atmos CLI to v1.201.0+ and start using new stack features (stores, hooks,
gomplate templates, `!terraform.state`/`!terraform.output` YAML tags), their `atmos.yaml` and stack
files contain structures that the old Atmos v1.189.0 code in the provider cannot parse. This causes
the provider to panic at runtime, which Terraform reports as:

```
Error: Plugin did not respond
  with module.iam_roles.module.account_map.data.utils_component_config.config[0],
  The plugin encountered an error, and failed to respond to the
  plugin.(*GRPCProvider).ReadDataSource call.
```

To fix this, the provider must be upgraded to a newer Atmos version. But upgrading is blocked because
the two functions it depends on no longer exist.

## Fix

### Import cycle constraint

The functions cannot be restored in `pkg/component` because `internal/exec` now imports `pkg/component`
(for the `ComponentProvider` interface and `ComponentRegistry`). Placing the functions in `pkg/component`
would create an import cycle: `pkg/component` -> `internal/exec` -> `pkg/component`.

The functions are instead added to `pkg/describe/component_processor.go`, which already imports
`internal/exec` without a cycle. The `terraform-provider-utils` provider will update its import from
`pkg/component` to `pkg/describe` (a one-line change).

### Implementation

The restored functions in `pkg/describe` are thin wrappers that delegate to
`internal/exec.ExecuteDescribeComponent`. The implementation:

1. Preserves the exact function signatures that `terraform-provider-utils` depends on
2. Honors `atmosCliConfigPath` and `atmosBasePath` by initializing config with these values and
   passing the resulting `AtmosConfiguration` to the internal API
3. Benefits from the improved error handling in `detectComponentType` (only falls back on
   `ErrInvalidComponent`, not all errors — fixes #1864)
4. Does not reintroduce the old `log.Error()` calls before returning (callers handle errors)

### Key differences from old implementation

| Aspect                   | Old (v1.189.0)                            | New (restored)                                         |
|--------------------------|-------------------------------------------|--------------------------------------------------------|
| Package                  | `pkg/component`                           | `pkg/describe` (import cycle prevents `pkg/component`) |
| Component type detection | Swallowed all errors during fallback      | Only falls back on `ErrInvalidComponent`               |
| Internal delegation      | `e.ProcessStacks()` directly              | `e.ExecuteDescribeComponent()` (higher-level)          |
| Error logging            | `log.Error(err)` before return            | No pre-logging (caller decides)                        |
| Context resolution       | Same `GetStackNameTemplate`/`ProcessTmpl` | Same helpers, unchanged                                |

### Files changed

**Atmos:**

- `pkg/describe/component_processor.go` (new file — `ProcessComponentInStack`, `ProcessComponentFromContext`)

**terraform-provider-utils** (separate PR):

- `internal/provider/data_source_component_config.go` — change import from `pkg/component` to `pkg/describe`
- `go.mod` — bump Atmos dependency to the version containing this fix
