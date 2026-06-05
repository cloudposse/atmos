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

| Entry Point                           | Templates      | YAML Functions | How to Disable                                                              |
|---------------------------------------|----------------|----------------|-----------------------------------------------------------------------------|
| **`ProcessComponentInStack` API**     | Default `true` | Default `true` | Pass `WithProcessTemplates(false)` and/or `WithProcessYamlFunctions(false)` |
| **`ProcessComponentFromContext` API** | Default `true` | Default `true` | Pass `WithProcessTemplates(false)` and/or `WithProcessYamlFunctions(false)` |

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

### Functional options pattern

The fix uses Go's functional options pattern to add optional processing controls to both
public functions. This follows the repo's coding conventions and provides a clean, extensible
API surface.

```go
// processOptions holds the resolved processing options (unexported).
type processOptions struct {
    processTemplates     bool
    processYamlFunctions bool
}

// ProcessOption is a functional option for ProcessComponentInStack and ProcessComponentFromContext.
type ProcessOption func(*processOptions)

// WithProcessTemplates controls whether Go templates are resolved during processing.
// When false, template expressions like {{ .settings.config.a }} are preserved as raw strings.
// Defaults to true when not specified.
func WithProcessTemplates(enabled bool) ProcessOption {
    return func(o *processOptions) {
        o.processTemplates = enabled
    }
}

// WithProcessYamlFunctions controls whether YAML functions (!terraform.output, !terraform.state,
// !template, !store, etc.) are resolved during processing.
// When false, YAML function tags are preserved as raw strings.
// Defaults to true when not specified.
func WithProcessYamlFunctions(enabled bool) ProcessOption {
    return func(o *processOptions) {
        o.processYamlFunctions = enabled
    }
}
```

### `ProcessComponentInStack` — variadic functional options (backward compatible)

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

// After (variadic functional options — backward compatible):
func ProcessComponentInStack(
    component string,
    stack string,
    atmosCliConfigPath string,
    atmosBasePath string,
    opts ...ProcessOption,
) (map[string]any, error)
```

### `ProcessComponentFromContext` — variadic functional options (backward compatible)

```go
// Before:
func ProcessComponentFromContext(params *ComponentFromContextParams) (map[string]any, error)

// After (variadic functional options — backward compatible):
func ProcessComponentFromContext(
    params *ComponentFromContextParams,
    opts ...ProcessOption,
) (map[string]any, error)
```

### `ComponentFromContextParams` — unchanged

The struct no longer needs `*bool` fields. Processing options are passed as variadic
functional options to `ProcessComponentFromContext` instead.

```go
type ComponentFromContextParams struct {
    Component          string
    Namespace          string
    Tenant             string
    Environment        string
    Stage              string
    AtmosCliConfigPath string
    AtmosBasePath      string
}
```

### `processComponentInStackWithConfig` — accepts resolved options

```go
func processComponentInStackWithConfig(
    atmosConfig *schema.AtmosConfiguration,
    component string,
    stack string,
    opts *processOptions,
) (map[string]any, error) {
    return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
        AtmosConfig:          atmosConfig,
        Component:            component,
        Stack:                stack,
        ProcessTemplates:     opts.processTemplates,
        ProcessYamlFunctions: opts.processYamlFunctions,
    })
}
```

### Backward compatibility

- **Existing callers of `ProcessComponentInStack`**: No changes needed. The variadic `opts`
  parameter is optional — calling with 4 args still compiles and defaults both flags to `true`.
- **Existing callers of `ProcessComponentFromContext`**: No changes needed. Calling with only
  the params struct still compiles and defaults both flags to `true`.
- **Atmos CLI**: Continues to work as before. The CLI's `--process-templates` and
  `--process-functions` flags are handled at a higher level and are unaffected.
- **Tests**: All existing tests pass without modification.

## Tests Added

| Test                                                    | What It Verifies                                                                                                                    |
|---------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `TestProcessComponentInStackTemplatesDisabledOnly`      | `WithProcessTemplates(false)` preserves raw Go template strings (`{{ .settings.config.a }}`) while YAML functions are still enabled |
| `TestProcessComponentInStackTemplatesEnabledOnly`       | `WithProcessTemplates(true)` resolves Go templates to values (`component-1-a`) while YAML functions are disabled                    |
| `TestProcessComponentInStackYamlFunctionsDisabledOnly`  | `WithProcessYamlFunctions(false)` preserves raw YAML function tags (`!template hello-world`) while templates are still enabled      |
| `TestProcessComponentInStackYamlFunctionsEnabledOnly`   | `WithProcessYamlFunctions(true)` resolves YAML function tags (JSON-decoded values) while templates are disabled                     |
| `TestProcessComponentInStackBackwardCompatNoOptions`    | Old 4-arg call (no options) still works and returns correct vars                                                                    |
| `TestProcessComponentFromContextWithProcessingDisabled` | `ProcessComponentFromContext` respects `WithProcessTemplates(false)` functional option                                              |

### Test fixtures used

- **`stack-templates`** — Contains Go template expressions in component vars (`{{ .settings.config.a }}`).
  Used to verify that `WithProcessTemplates(false)` preserves raw templates and `WithProcessTemplates(true)` resolves them.
- **`atmos-template-yaml-function`** — Contains `!template` YAML function tags in component vars.
  Used to verify that `WithProcessYamlFunctions(false)` preserves raw tags and `WithProcessYamlFunctions(true)` resolves them.

Each flag is tested independently against its own fixture, proving the two flags are wired
independently and do not interfere with each other.

## Files Changed

- `pkg/describe/component_processor.go` — added `processOptions` struct, `ProcessOption` type,
  `WithProcessTemplates` and `WithProcessYamlFunctions` functional option constructors,
  `defaultProcessOptions` and `applyProcessOptions` helpers, variadic options on both
  `ProcessComponentInStack` and `ProcessComponentFromContext`, updated `processComponentInStackWithConfig`
- `pkg/describe/component_processor_test.go` — added 6 new tests verifying each flag independently
  with real fixtures

## Consumer Changes (separate PR)

The `terraform-provider-utils` provider will be updated in a separate PR to:

1. Update `go.mod` to the Atmos version containing this fix
2. Pass `WithProcessTemplates(false)` and `WithProcessYamlFunctions(false)` in both
   `ProcessComponentInStack` and `ProcessComponentFromContext` call sites

```go
// Example provider usage:
result, err = p.ProcessComponentInStack(
    component, stack, atmosCliConfigPath, atmosBasePath,
    p.WithProcessTemplates(false),
    p.WithProcessYamlFunctions(false),
)

result, err = p.ProcessComponentFromContext(
    &p.ComponentFromContextParams{
        Component:          component,
        Namespace:          namespace,
        Tenant:             tenant,
        Environment:        environment,
        Stage:              stage,
        AtmosCliConfigPath: atmosCliConfigPath,
        AtmosBasePath:      atmosBasePath,
    },
    p.WithProcessTemplates(false),
    p.WithProcessYamlFunctions(false),
)
```
