# PRD: Identity Selection Support for Atmos Stores

## Summary

Add an `identity` field to store configuration so stores can authenticate using the same identity system used by `atmos auth` instead of relying solely on default credential chains.

## Problem

Atmos stores (`!store` YAML function) currently authenticate using default credential chains (environment variables, default AWS profiles, etc.). Users who already configure named identities in `atmos auth` cannot reuse those identities for store access. This forces separate credential management for secrets access vs. Terraform execution.

## Solution

Add an optional `identity` field to `StoreConfig` that references an Atmos auth identity. When set, the store uses that identity's credentials instead of the default credential chain. The implementation uses lazy client initialization to avoid circular dependency issues (stores are created during config loading, but auth happens later during command execution).

## User Stories

1. **As a DevOps engineer**, I want to configure a store to use my prod-admin identity so I can access production secrets without managing separate credentials.
2. **As a platform team member**, I want stores to automatically use the correct identity based on stack configuration so developers don't need to manage cloud credentials manually.
3. **As an existing user**, I want my stores that don't specify an identity to continue working exactly as before.

## Target Configuration

```yaml
stores:
  prod/aws-ssm:
    type: aws-ssm-parameter-store
    identity: prod-admin          # NEW: references atmos auth identity
    options:
      region: us-east-1
```

## Architecture

### Lazy Client Initialization

Stores are created during `InitCliConfig()` (config loading phase), but authentication happens later during command execution. To bridge this gap, stores support lazy client initialization:

1. Store constructors accept an identity name but don't create cloud clients immediately.
2. An `ensureClient()` method lazily initializes the client on first `Get`/`Set` call.
3. After auth completes, a resolver is injected into identity-aware stores via `SetAuthContext()`.

### Avoiding Circular Dependencies

The dependency chain is: `pkg/config` imports `pkg/store`, `pkg/auth` imports `pkg/config`. To avoid circular deps, the resolver lives in `pkg/store/authbridge/` — a sub-package that `pkg/store` never imports, but which can import `pkg/auth`.

### Interface Design

- `AuthContextResolver` — resolves identity names to cloud-specific auth contexts.
- `IdentityAwareStore` — extends `Store` with `SetAuthContext()` for identity injection.

### Backward Compatibility

- Existing stores without `identity` field work identically to before.
- Redis and Artifactory stores do not support identity (no cloud provider mapping) and emit a warning if `identity` is set.

## Supported Store Types

| Store Type | Identity Support | Auth Mechanism |
|---|---|---|
| aws-ssm-parameter-store | Yes | `LoadConfigWithAuth()` with `AWSAuthContext` |
| azure-key-vault | Yes | Credentials from `AzureAuthContext` |
| google-secret-manager | Yes | Credentials file from `GCPAuthContext` |
| redis | No | Env vars / connection string |
| artifactory | No | Access tokens |

## Files Changed

- `pkg/store/config.go` — Add `Identity` field.
- `pkg/store/identity.go` — New interfaces.
- `pkg/store/errors.go` — New error sentinels.
- `pkg/store/aws_ssm_param_store.go` — Lazy init + identity.
- `pkg/store/azure_keyvault_store.go` — Lazy init + identity.
- `pkg/store/google_secret_manager_store.go` — Lazy init + identity.
- `pkg/store/registry.go` — Pass identity, add `SetAuthContextResolver`.
- `pkg/store/authbridge/resolver.go` — Resolver implementation.
- `internal/exec/terraform.go` — Inject resolver after auth.
