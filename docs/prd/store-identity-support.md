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
| aws-ssm-parameter-store | Yes | AWS SDK config loaded from `AWSAuthContext` credential/config files |
| azure-key-vault | Yes | `DefaultAzureCredential` with tenant hint from `AzureAuthContext` |
| google-secret-manager | Yes | Credentials file from `GCPAuthContext` |
| redis | No | Env vars / connection string |
| artifactory | No | Access tokens |

## Realm Compatibility

Store identity support is fully compatible with [Atmos auth realms](https://atmos.tools/cli/auth). Realms provide credential isolation between different repositories or customer environments by namespacing credential file paths.

### How Realms Work with Store Identities

When a realm is configured (via `auth.realm` in `atmos.yaml` or `ATMOS_AUTH_REALM` env var), the auth system embeds the realm into all credential file paths. The authbridge resolver passes these realm-scoped absolute paths through to stores unchanged, so stores automatically use realm-isolated credentials.

The flow is:

1. Auth manager creates realm-scoped credential paths during `Authenticate()`.
2. `PostAuthenticate` populates `AuthContext` with absolute paths containing the realm directory.
3. The authbridge resolver reads these paths from `AuthContext` and passes them to the store.
4. The store uses the paths directly — no realm awareness needed in store code.

### Per-Provider Realm Behavior

| Provider | Realm Mechanism | Example Path |
|---|---|---|
| AWS SSM | `CredentialsFile` and `ConfigFile` include realm in path | `~/.config/atmos/{realm}/aws/{provider}/credentials` |
| GCP GSM | `CredentialsFile` includes realm in path | `~/.config/atmos/{realm}/gcp/{provider}/adc/{identity}/application_default_credentials.json` |
| Azure KV | Auth sets MSAL cache + env vars during `Authenticate()`; `CredentialsFile` available for future use | `~/.azure/atmos/{realm}/{provider}/credentials.json` |

### Design Decisions

- **Store code is realm-unaware.** Stores receive pre-resolved absolute paths and never need to know about realms. This keeps the store layer simple and avoids coupling it to the realm system.
- **Auth config types mirror schema types.** `AWSAuthConfig`, `AzureAuthConfig`, and `GCPAuthConfig` in `pkg/store/identity.go` carry all realm-relevant fields from `schema.AWSAuthContext`, `schema.AzureAuthContext`, and `schema.GCPAuthContext` respectively.
- **Empty realm is backward-compatible.** When no realm is configured, credential paths use the legacy layout without a realm subdirectory. Stores work identically in both cases.

## Files Changed

- `pkg/store/config.go` — Add `Identity` field.
- `pkg/store/identity.go` — New interfaces and auth config types (mirrors schema auth contexts).
- `pkg/store/errors.go` — New error sentinels.
- `pkg/store/aws_ssm_param_store.go` — Lazy init + identity.
- `pkg/store/azure_keyvault_store.go` — Lazy init + identity.
- `pkg/store/google_secret_manager_store.go` — Lazy init + identity.
- `pkg/store/registry.go` — Pass identity, add `SetAuthContextResolver`.
- `pkg/store/authbridge/resolver.go` — Resolver implementation (bridges store and auth packages).
- `internal/exec/terraform.go` — Inject resolver after auth.
