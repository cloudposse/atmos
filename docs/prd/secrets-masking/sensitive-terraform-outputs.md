# Sensitive Terraform Output Handling PRD

## Status

Draft — Addendum to [Secrets Management PRD](../secrets-management.md)

## Executive Summary

Automatically detect and mask sensitive Terraform outputs as they flow between components via `!terraform.output`, `atmos.Component()`, and `atmos describe`. Terraform already provides a `sensitive` boolean on each output; Atmos's I/O masking layer (`io.RegisterSecret()`) is fully implemented. This PRD wires them together.

## Problem Statement

The [Secrets Management PRD](../secrets-management.md) covers **human-provisioned secrets** — API keys, tokens, and passwords managed through `atmos secret set/get`. It explicitly scopes stores as "machine-written, machine-read state" and secrets as "human-managed configuration."

This leaves a gap: **sensitive Terraform outputs that flow between components are never masked.** A database component that outputs a `password` marked `sensitive = true` in Terraform will have that password appear in cleartext when:

1. Another component references it via `!terraform.output database password`
2. A template uses `atmos.Component("database", "prod").outputs.password`
3. A user runs `atmos describe component database -s prod`
4. A user runs `atmos terraform output database -s prod`
5. CI/CD logs capture any of the above

Terraform itself masks sensitive outputs in `terraform plan` and `terraform apply` console output, but `terraform output -json` intentionally includes the raw values (with a `sensitive: true` metadata flag). Atmos reads this JSON and discards the sensitivity flag.

## Current Architecture

### What Terraform Provides

`terraform output -json` returns `OutputMeta` per output:

```json
{
  "password": {
    "sensitive": true,
    "type": "string",
    "value": "hunter2"
  },
  "vpc_id": {
    "sensitive": false,
    "type": "string",
    "value": "vpc-abc123"
  }
}
```

The Go SDK (`github.com/hashicorp/terraform-exec/tfexec`) exposes this as:

```go
type OutputMeta struct {
    Sensitive bool            `json:"sensitive"`
    Type      json.RawMessage `json:"type"`
    Value     json.RawMessage `json:"value"`
}
```

### Where Sensitivity Is Discarded

In `pkg/terraform/output/executor.go`, `processOutputs()` (line 504) converts `OutputMeta` to `map[string]any`, extracting only `v.Value` and discarding `v.Sensitive`:

```go
func processOutputs(outputMeta map[string]tfexec.OutputMeta, atmosConfig *schema.AtmosConfiguration) map[string]any {
    return lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
        s := string(v.Value)
        d, err := u.ConvertFromJSON(s)
        // ... returns (k, d) — Sensitive field is never read
    })
}
```

### Where Values Flow Unmasked

1. **`!terraform.output`** — `internal/exec/yaml_func_terraform_output.go` line 118 calls `outputGetter.GetOutput()` and returns the raw value without calling `io.RegisterSecret()`.

2. **`atmos.Component()`** — `internal/exec/template_funcs_component.go` calls `ExecuteWithSections()` which returns all outputs unmasked.

3. **`atmos describe component`** — Includes terraform outputs in the component sections, displayed in cleartext.

4. **`atmos terraform output`** — Formats and displays all outputs without masking.

### What's Ready to Use

`io.RegisterSecret()` in `pkg/io/global.go` (line 179) registers a value with the global masker. Once registered, the value is automatically redacted in all writes to `io.Data` (stdout) and `io.UI` (stderr), including base64, URL-encoded, and JSON-encoded variants.

## Proposed Changes

### Phase 1: Preserve Sensitivity Metadata

**Goal:** Propagate Terraform's `sensitive` flag through the output pipeline.

#### 1a. New return type for sensitive-aware outputs

```go
// pkg/terraform/output/types.go

// OutputValue wraps a terraform output value with its sensitivity metadata.
type OutputValue struct {
    Value     any
    Sensitive bool
}
```

#### 1b. New processing function that preserves sensitivity

Add a new function alongside `processOutputs` that returns sensitivity metadata:

```go
// pkg/terraform/output/executor.go

// processOutputsWithSensitivity converts OutputMeta preserving the Sensitive flag.
func processOutputsWithSensitivity(outputMeta map[string]tfexec.OutputMeta, atmosConfig *schema.AtmosConfiguration) map[string]OutputValue {
    return lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, OutputValue) {
        s := string(v.Value)
        d, err := u.ConvertFromJSON(s)
        if err != nil {
            return k, OutputValue{Value: nil, Sensitive: v.Sensitive}
        }
        return k, OutputValue{Value: d, Sensitive: v.Sensitive}
    })
}
```

The existing `processOutputs` remains unchanged for backward compatibility. New callers that need sensitivity metadata use `processOutputsWithSensitivity`.

### Phase 2: Auto-Register Sensitive Values with Masker

**Goal:** Any sensitive terraform output that flows through Atmos is automatically masked in all output.

#### 2a. Register at the output retrieval boundary

When `GetOutput()` or `GetAllOutputs()` returns values, register sensitive ones:

```go
// After processing outputs with sensitivity metadata
for _, ov := range sensitiveOutputs {
    if ov.Sensitive {
        if s, ok := ov.Value.(string); ok {
            io.RegisterSecret(s)
        }
    }
}
```

#### 2b. Register in `!terraform.output` resolution

In `internal/exec/yaml_func_terraform_output.go`, after line 118 where the value is retrieved, register it if sensitive:

```go
value, exists, err := outputGetter.GetOutput(...)
// ... error handling ...
if exists && isSensitive {
    if s, ok := value.(string); ok {
        io.RegisterSecret(s)
    }
}
```

This requires `GetOutput` to also return sensitivity metadata (or a separate lookup).

#### 2c. Register in `atmos.Component()` resolution

In `internal/exec/template_funcs_component.go`, when terraform outputs are merged into the component sections, register any sensitive values.

### Phase 3: Mask in Describe/List Output

**Goal:** `atmos describe component` and `atmos terraform output` mask sensitive values.

Since sensitive values are registered with `io.RegisterSecret()` at retrieval time, all subsequent writes through `io.Data` and `io.UI` will automatically mask them. No additional per-command changes are needed — the masking is global.

However, for the `atmos terraform output` command specifically, the `--format` flag should respect sensitivity:

- **Default (human):** Sensitive values shown as `(sensitive)` (matching Terraform's own behavior).
- **`--format json`:** Values included but masked via I/O layer.
- **`--format raw`:** Bypass masking (explicit opt-in for automation that needs raw values, e.g., piping to another tool). This requires the user to explicitly request unmasked output.

### Store Sensitivity Awareness

See [Store Sensitivity PRD](store-sensitivity.md) — covers extending the store interface to preserve sensitivity metadata (e.g., SSM `SecureString`) and auto-mask on retrieval via `!store`.

## Integration with Secrets Management PRD

This PRD is **complementary** to the Secrets Management PRD:

| Concern | Secrets PRD | This PRD |
|---------|-------------|----------|
| What | Human-provisioned configuration | Machine-generated terraform outputs |
| Lifecycle | `atmos secret set/get` CLI | Automatic during `terraform output` |
| Declaration | Explicit YAML declarations | Implicit from Terraform `sensitive = true` |
| Masking trigger | `!secret` resolution | `!terraform.output` resolution |
| Masking mechanism | Same: `io.RegisterSecret()` | Same: `io.RegisterSecret()` |

Both systems feed into the same I/O masking layer. A value registered by either path is masked identically in all output.

## Implementation Order

1. **Phase 1** (preserve metadata) — Foundation, no behavioral change.
2. **Phase 2** (auto-register) — Core value: sensitive outputs are masked everywhere.
3. **Phase 3** (describe/list) — Mostly free once Phase 2 lands; just formatting tweaks for `terraform output`.
4. **Store awareness** — See [Store Sensitivity PRD](store-sensitivity.md).

## Key Files

| File | Role |
|------|------|
| `pkg/terraform/output/executor.go` | `processOutputs()` — where sensitivity is currently discarded |
| `internal/exec/yaml_func_terraform_output.go` | `!terraform.output` resolution — where masking should be added |
| `internal/exec/template_funcs_component.go` | `atmos.Component()` — where outputs flow to other components |
| `pkg/io/global.go` | `RegisterSecret()` — the masking registration API |
| `pkg/store/` | Store interface — see [Store Sensitivity PRD](store-sensitivity.md) |

## Testing Strategy

- Unit tests for `processOutputsWithSensitivity` verifying the `Sensitive` flag is preserved.
- Unit tests for `!terraform.output` resolution verifying `io.RegisterSecret()` is called for sensitive outputs (mock the masker).
- Integration test: `atmos describe component` with a component that has sensitive outputs, verifying masked output.
- Negative test: Non-sensitive outputs are NOT registered with the masker (avoid over-masking common values like VPC IDs).

## References

- [Secrets Management PRD](../secrets-management.md)
- [I/O Handling Strategy PRD](../io-handling-strategy.md)
- [Terraform `sensitive` outputs documentation](https://developer.hashicorp.com/terraform/language/values/outputs#sensitive-suppressing-values-in-cli-output)
- `tfexec.OutputMeta` — `github.com/hashicorp/terraform-exec/tfexec`
