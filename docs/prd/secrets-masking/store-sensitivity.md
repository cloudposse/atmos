# Store Sensitivity Awareness PRD

## Status

Draft — Part of the [secrets-masking](.) PRD series

## Executive Summary

Extend the Atmos store interface to preserve sensitivity metadata, so that sensitive Terraform outputs written to stores are encrypted at rest (e.g., SSM `SecureString`) and automatically masked on retrieval via `io.RegisterSecret()`.

## Problem Statement

Stores (`pkg/store/`) are opaque key-value stores for sharing Terraform outputs between components. Today, all values are stored identically regardless of whether the source Terraform output was marked `sensitive = true`. This means:

1. A sensitive database password stored in SSM is written as a plain `String` parameter, not a KMS-encrypted `SecureString`.
2. When `!store` retrieves a sensitive value, it's not registered with the I/O masker — it appears in cleartext in logs and CLI output.
3. There is no way for a store consumer to know whether a value originated from a sensitive output.

The [Sensitive Terraform Outputs PRD](sensitive-terraform-outputs.md) addresses masking at the `!terraform.output` and `atmos.Component()` resolution points. This PRD addresses the **store layer** — when stores are used as the intermediate between producer and consumer components.

## Proposed Changes

### Store Interface Extension

Add a `SensitiveStore` interface that extends the existing `Store`:

```go
// pkg/store/store.go

// SensitiveStore extends Store with sensitivity metadata.
type SensitiveStore interface {
    Store
    // SetWithSensitivity stores a value with sensitivity metadata.
    SetWithSensitivity(stack, component, key string, value any, sensitive bool) error
    // GetWithSensitivity retrieves a value and its sensitivity flag.
    GetWithSensitivity(stack, component, key string) (value any, sensitive bool, err error)
}
```

Store providers that don't support sensitivity simply implement `Store` as before. Providers that support native encryption implement `SensitiveStore`. Callers check at runtime:

```go
if ss, ok := store.(SensitiveStore); ok {
    ss.SetWithSensitivity(stack, component, key, value, sensitive)
} else {
    store.Set(stack, component, key, value)
}
```

### Provider Mappings

Each store provider maps sensitivity to its native equivalent:

| Provider | Sensitive | Non-sensitive |
|----------|-----------|---------------|
| AWS SSM Parameter Store | `SecureString` (KMS-encrypted) | `String` |
| AWS Secrets Manager | Already encrypted by default | Already encrypted by default |
| Azure Key Vault | Already a secrets store | Already a secrets store |
| GCP Secret Manager | Already a secrets store | Already a secrets store |

For SSM specifically, the provider already interacts with the SSM API — this is a matter of setting the `Type` parameter to `SecureString` based on sensitivity metadata. On retrieval, the provider reads the parameter type back and returns the sensitivity flag.

### Retrieval-Side Masking

When `!store` resolves a value from a sensitivity-aware store, it checks the sensitivity flag and registers with the masker:

```go
value, sensitive, err := store.GetWithSensitivity(stack, component, key)
if sensitive {
    if s, ok := value.(string); ok {
        io.RegisterSecret(s)
    }
}
```

This ensures that sensitive values flowing through stores get the same automatic masking as values resolved directly via `!terraform.output`.

## Key Files

| File | Role |
|------|------|
| `pkg/store/store.go` | Store interface — add `SensitiveStore` |
| `pkg/store/aws_ssm.go` | SSM provider — implement `SecureString` support |
| `internal/exec/yaml_func_store.go` | `!store` resolution — add masking on retrieval |
| `pkg/io/global.go` | `RegisterSecret()` — the masking registration API |

## Testing Strategy

- Unit tests for `SensitiveStore` interface: SSM provider stores sensitive values as `SecureString` and non-sensitive as `String`.
- Unit tests for `!store` resolution: values retrieved from `SecureString` parameters are auto-registered with the masker.
- Negative test: values from non-sensitive store entries are NOT registered with the masker.
- Mock-based tests: verify the SSM API is called with correct `Type` parameter.

## References

- [Sensitive Terraform Outputs PRD](sensitive-terraform-outputs.md)
- [Secrets Management PRD](../secrets-management.md)
- [I/O Handling Strategy PRD](../io-handling-strategy.md)
- Existing store implementation: `pkg/store/`
