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

| Provider | Encryption-at-rest for sensitive | Sensitivity metadata source (for `GetWithSensitivity`) |
|----------|----------------------------------|--------------------------------------------------------|
| AWS SSM Parameter Store | `SecureString` (KMS-encrypted); `String` when non-sensitive | Parameter `Type` (`SecureString` ⇒ sensitive) |
| AWS Secrets Manager | Encrypted by default | Reserved resource **Tag** `atmos:sensitive=true` |
| Azure Key Vault | Encrypted by default | Reserved secret **Tag** `atmos-sensitive=true` |
| GCP Secret Manager | Encrypted by default | Reserved secret **Label** `atmos-sensitive=true` |

#### Why a metadata source is required

For SSM, sensitivity is self-describing: the parameter `Type` (`SecureString` vs
`String`) tells `GetWithSensitivity` whether a value is sensitive. For the dedicated
secret managers (ASM, Azure Key Vault, GCP Secret Manager) everything is encrypted at
rest, so encryption alone **cannot** tell us whether a given value originated from a
sensitive Terraform output. To make `GetWithSensitivity(...)` return a *reliable*
boolean, the provider must persist the sensitivity flag explicitly:

- **AWS SSM** — set `Type=SecureString` on write; read `Type` back on retrieval.
- **AWS Secrets Manager** — write the reserved tag `atmos:sensitive=true`; read tags on
  retrieval (`DescribeSecret`).
- **Azure Key Vault** — write the reserved tag `atmos-sensitive=true` on the secret;
  read tags on `GetSecret`.
- **GCP Secret Manager** — write the reserved label `atmos-sensitive=true` on the
  secret; read labels on access.

> **Why the separator differs:** AWS Secrets Manager tag keys permit a colon, so the
> colon form `atmos:sensitive=true` is used there. Azure Key Vault and GCP Secret
> Manager use the hyphen form `atmos-sensitive=true` because GCP label keys allow only
> lowercase letters, digits, `_`, and `-` (no colon), and Azure follows the same
> convention for consistency.

**Fallback when metadata is absent:** if the reserved tag/label is missing (e.g. the
value pre-dates this feature or was written out-of-band), the provider defaults to
`sensitive=true` for the dedicated secret managers (fail safe — mask rather than leak)
and `sensitive=false` for SSM `String` parameters. The SSM asymmetry is intentional: its
`Type` field is authoritative and self-describing — `SecureString` has always been the
documented mechanism for sensitive data, so a `String` parameter has never been a place
secrets were stored. A `secret: true` store (see the
[Secrets Management PRD](../secrets-management.md)) always writes the sensitive variant.

### Retrieval-Side Masking

When `!store` resolves a value from a sensitivity-aware store, it checks the sensitivity flag and registers with the masker:

```go
value, sensitive, err := store.GetWithSensitivity(stack, component, key)
if sensitive {
    // Register all secret-bearing representations, not only plain strings.
    // Shares the recursive helper defined in the Sensitive Terraform Outputs PRD
    // (Phase 2a): registers scalar strings and walks maps/slices to register nested
    // string leaves, so structured store values (objects/lists) cannot leak.
    registerSensitiveValue(value)
}
```

This ensures that sensitive values flowing through stores get the same automatic masking as values resolved directly via `!terraform.output`. The `registerSensitiveValue` helper is the single shared implementation referenced by both PRDs — see [Sensitive Terraform Outputs PRD](sensitive-terraform-outputs.md) (Phase 2a).

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
