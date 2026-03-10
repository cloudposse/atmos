# Expose `ProcessTemplates` and `ProcessYamlFunctions` Flags in Public API

**Related Provider Issue:** `cloudposse/terraform-provider-utils` — `data "utils_component_config"`
crashes with "text file busy" (`ETXTBSY`) when stack YAML contains `!terraform.output` tags

**Affected Atmos Version:** v1.207.0+ (introduced `ProcessYamlFunctions: true` hardcoded in
`processComponentInStackWithConfig`)

**Severity:** Critical for embedded consumers — `!terraform.output` resolution spawns child
`terraform init` processes inside a Terraform provider plugin, conflicting with the parent
process's plugin cache

## Background

The `ProcessComponentInStack` and `ProcessComponentFromContext` functions in `pkg/describe` are
the public API used by `terraform-provider-utils` to resolve component configurations from stack
manifests. These functions delegate to `internal/exec.ExecuteDescribeComponent`, which accepts
`ProcessTemplates` and `ProcessYamlFunctions` flags.

However, `processComponentInStackWithConfig` — the shared implementation used by both public
functions — hardcodes both flags to `true`:

```go
// pkg/describe/component_processor.go (before this fix)
func processComponentInStackWithConfig(
    atmosConfig *schema.AtmosConfiguration,
    component string,
    stack string,
) (map[string]any, error) {
    return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
        AtmosConfig:          atmosConfig,
        Component:            component,
        Stack:                stack,
        ProcessTemplates:     true,   // hardcoded
        ProcessYamlFunctions: true,   // hardcoded
    })
}
```

This means external consumers of the public API have no way to disable template or YAML
function resolution, even though:

- The Atmos CLI already supports `--process-templates` and `--process-functions` flags
- Template processing has a config gate in `atmos.yaml` (`templates.settings.enabled`)
- Stack imports support `skip_templates_processing` per-import

The public API is the only entry point that lacks this control.

## How Processing Is Controlled Across Entry Points

| Entry Point                                        | Templates                                                                      | YAML Functions                    | How to Disable                                           |
|----------------------------------------------------|--------------------------------------------------------------------------------|-----------------------------------|----------------------------------------------------------|
| **`atmos.yaml`**                                   | Requires `templates.settings.enabled: true`. Sprig/Gomplate individually gated | Always active (no config gate)    | `templates.settings.enabled: false`                      |
| **Atmos CLI** (`atmos describe component`)         | Enabled by default                                                             | Enabled by default                | `--process-templates=false`, `--process-functions=false` |
| **Stack imports**                                  | Enabled by default                                                             | N/A                               | `skip_templates_processing: true` per import             |
| **`ProcessComponentInStack` API** (before fix)     | Hardcoded `true` — **no control**                                              | Hardcoded `true` — **no control** | Not possible                                             |
| **`ProcessComponentFromContext` API** (before fix) | Hardcoded `true` — **no control**                                              | Hardcoded `true` — **no control** | Not possible                                             |

After this fix, the API row becomes:

| Entry Point                           | Templates      | YAML Functions | How to Disable                                            |
|---------------------------------------|----------------|----------------|-----------------------------------------------------------|
| **`ProcessComponentInStack` API**     | Default `true` | Default `true` | Pass `ProcessComponentInStackOptions` with `*bool` fields |
| **`ProcessComponentFromContext` API** | Default `true` | Default `true` | Set `*bool` fields on `ComponentFromContextParams`        |

## Impact on `terraform-provider-utils`

When `ProcessYamlFunctions` is `true`, YAML tags like `!terraform.output` are eagerly resolved
during stack config processing. The resolution chain spawns child processes:

```text
processTagTerraformOutput()
  → outputGetter.GetOutput()
    → runInit()                   [terraform init via terraform-exec]
    → runOutput()                 [terraform output]
```

Inside a Terraform provider plugin (gRPC process), these child `terraform init` processes try
to install providers into the shared plugin cache. On Linux, this fails with `ETXTBSY` ("text
file busy") because the parent OpenTofu process is already executing binaries from the same
cache directory.

The provider only needs component backend config, workspace, and vars — it does not need
resolved `!terraform.output`, `!terraform.state`, or `!store` values. With this fix, the
provider can pass `false` for both flags to avoid the crash entirely.

## Fix

### New `ProcessComponentInStackOptions` struct

```go
type ProcessComponentInStackOptions struct {
    ProcessTemplates     *bool // Controls Go template resolution. Defaults to true if nil.
    ProcessYamlFunctions *bool // Controls YAML function resolution. Defaults to true if nil.
}
```

### `ProcessComponentInStack` — variadic options (backward compatible)

The function signature changes from 4 positional args to 4 positional args + variadic options.
Existing callers that pass 4 args continue to compile without changes.

```go
// Before:
func ProcessComponentInStack(
    component string,
    stack string,
    atmosCliConfigPath string,
    atmosBasePath string,
) (map[string]any, error)

// After:
func ProcessComponentInStack(
    component string,
    stack string,
    atmosCliConfigPath string,
    atmosBasePath string,
    opts ...ProcessComponentInStackOptions,
) (map[string]any, error)
```

### `ComponentFromContextParams` — new optional fields (backward compatible)

Adding `*bool` fields to a struct is backward compatible — existing struct literals that don't
set these fields get `nil`, which defaults to `true`.

```go
type ComponentFromContextParams struct {
    Component            string
    Namespace            string
    Tenant               string
    Environment          string
    Stage                string
    AtmosCliConfigPath   string
    AtmosBasePath        string
    ProcessTemplates     *bool // Optional: defaults to true if nil
    ProcessYamlFunctions *bool // Optional: defaults to true if nil
}
```

### `processComponentInStackWithConfig` — uses `boolDefault` helper

```go
func boolDefault(p *bool, defaultVal bool) bool {
    if p != nil {
        return *p
    }
    return defaultVal
}

func processComponentInStackWithConfig(
    atmosConfig *schema.AtmosConfiguration,
    component string,
    stack string,
    processTemplates *bool,
    processYamlFunctions *bool,
) (map[string]any, error) {
    return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
        AtmosConfig:          atmosConfig,
        Component:            component,
        Stack:                stack,
        ProcessTemplates:     boolDefault(processTemplates, true),
        ProcessYamlFunctions: boolDefault(processYamlFunctions, true),
    })
}
```

### Backward compatibility

- **Existing callers of `ProcessComponentInStack`**: No changes needed. The variadic `opts`
  parameter is optional — calling with 4 args still compiles and defaults both flags to `true`.
- **Existing callers of `ProcessComponentFromContext`**: No changes needed. Struct literals
  that don't set `ProcessTemplates` or `ProcessYamlFunctions` get `nil` → defaults to `true`.
- **Atmos CLI**: Continues to work as before. The CLI's `--process-templates` and
  `--process-functions` flags are handled at a higher level and are unaffected.
- **Tests**: All existing tests pass without modification.

## Tests Added

| Test                                                      | What It Verifies                                                            |
|-----------------------------------------------------------|-----------------------------------------------------------------------------|
| `TestBoolDefault`                                         | `boolDefault` helper: `nil` returns default, non-nil returns pointed value  |
| `TestProcessComponentInStackWithOptionsDisabled`          | Both flags `false` → result still contains vars, backend_type, backend      |
| `TestProcessComponentInStackWithOptionsNilDefaultsToTrue` | Options struct with nil fields behaves like no options                      |
| `TestProcessComponentInStackBackwardCompatNoOptions`      | Old 4-arg call (no options) still works                                     |
| `TestProcessComponentInStackDisabledMatchesEnabled`       | For configs without templates/YAML functions, vars are identical either way |
| `TestProcessComponentFromContextWithOptionsDisabled`      | Struct-based API with both flags disabled returns correct results           |

## Files Changed

- `pkg/describe/component_processor.go` — added `ProcessComponentInStackOptions` struct,
  variadic options on `ProcessComponentInStack`, `*bool` fields on `ComponentFromContextParams`,
  `boolDefault` helper, updated `processComponentInStackWithConfig`
- `pkg/describe/component_processor_test.go` — added 6 new tests

## Consumer Changes (separate PR)

The `terraform-provider-utils` provider will be updated in a separate PR to:

1. Update `go.mod` to the Atmos version containing this fix
2. Pass `ProcessTemplates: false` and `ProcessYamlFunctions: false` in both
   `ProcessComponentInStack` and `ProcessComponentFromContext` call sites
