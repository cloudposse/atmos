# Planfile Store Should Support Per-Store Identity

> Status: Verified against the codebase (current `main`). The reported behavior is real and reproducible
> from the source. The proposed direction is correct, but the fix sketch needs one correction - it must
> also attach the auth resolver, not just set the identity name (see Proposed Solution and Verification
> below).

## Problem

When a component authenticates with a non-AWS identity (e.g., `azure/subscription`), the S3 planfile store inherits that
identity via `attachIdentity()` and fails because it cannot resolve AWS credentials from a non-AWS identity.

The planfile store needs AWS credentials (for `aws/s3` backend), but the component's active identity may be for a
different cloud provider. There is no way to specify which identity the planfile store should use independently of the
component's Terraform identity.

## Where It Surfaces

A typical multi-cloud CI setup:

```yaml
auth:
  providers:
    azure-oidc:
      kind: azure/oidc
      spec:
        tenant_id: "..."
        client_id: "..."

  identities:
    aws-backend:
      kind: ambient      # or aws/assume-role, aws/permission-set, etc.

    azure-plan:
      kind: azure/subscription
      default: true
      via:
        provider: azure-oidc
      principal:
        subscription_id: "..."

components:
  terraform:
    planfiles:
      default: "s3"
      stores:
        s3:
          type: aws/s3
          options:
            bucket: "my-planfiles-bucket"
            prefix: "atmos/planfiles/"
            region: "us-east-1"
```

When `atmos terraform plan` runs:

1. Auth hook authenticates with `azure-plan` (default identity)
2. `info.Identity` is set to `"azure-plan"`
3. Terraform plan succeeds (Azure OIDC credentials are set in the environment)
4. `onAfterPlan` hook triggers planfile upload
5. `createPlanfileStore()` calls `attachIdentity()` which sets `opts.Identity = "azure-plan"`
6. S3 store's `NewStore()` sees non-empty identity + defers to `initIdentityClient`
7. `initIdentityClient` tries to resolve AWS auth context from the `azure-plan` identity + fails

Error:

```
failed to upload planfile: failed to load AWS configuration:
failed to resolve AWS auth context for identity "azure-plan":
auth context not available for identity
```

The same issue applies in reverse: a GCP component with an S3 planfile store, or an AWS component with an Azure Blob
Storage planfile store.

> Note (verified): the `aws/s3` planfile backend exists, but there is no Azure Blob or GCS planfile backend
> registered today (only `aws/s3`, `local/dir`, and `github` are registered). The reverse example is therefore
> currently hypothetical - per-store identity is necessary but not sufficient for it; the Azure/GCS backends
> would also have to be implemented. The shared auth plumbing is already multi-cloud (see Verification).

## Root Cause

`attachIdentity()` in `pkg/ci/executor.go` unconditionally propagates the component's active identity to the planfile
store, setting both the identity name and the auth resolver:

```go
func attachIdentity(artOpts *artifact.StoreOptions, info *schema.ConfigAndStacksInfo) {
    if info == nil || info.Identity == "" {
        return
    }
    artOpts.Identity = info.Identity
    // ... also sets artOpts.Resolver from info.AuthManager (the authbridge resolver)
}
```

`createPlanfileStore()` (same file) reads the default store spec and always calls `attachIdentity(&artOpts, opts.Info)` -
there is no per-store identity, so the component identity always wins.

The S3 store in `pkg/ci/artifact/s3/store.go` uses the default AWS credential chain only when `Identity` is empty:

```go
if opts.Identity == "" {
    if err := store.initDefaultClient(context.Background()); err != nil {
        return nil, err
    }
}
```

When `Identity` is non-empty, it defers initialization to `initIdentityClient`, which resolves credentials through the
injected `AuthContextResolver`. For a non-AWS identity the resolver authenticates the identity successfully, but no AWS
auth context is produced, so it returns `ErrAuthContextNotAvailable` ("auth context not available for identity"). The
precise wrapped chain is:

```
failed to resolve AWS auth context for identity "azure-plan":
auth context not available for identity:
AWS auth context not available after authenticating identity "azure-plan"
```

In other words, the failure is not "the identity could not be authenticated" - it is "the identity authenticated fine
but yields no AWS credentials." The result is the same: the planfile upload fails.

## Proposed Solution

Add an optional `identity` field to `PlanfileStoreSpec`:

```go
// PlanfileStoreSpec defines a planfile storage backend.
type PlanfileStoreSpec struct {
    Type     string         `yaml:"type" json:"type" mapstructure:"type"`
    Options  map[string]any `yaml:"options,omitempty" json:"options,omitempty" mapstructure:"options"`
    Identity string         `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}
```

Update `createPlanfileStore()` in `pkg/ci/executor.go` to prefer the store-level identity over the component identity.

> Correction (verified against the code): simply setting `artOpts.Identity = storeSpec.Identity` is NOT enough. The
> artifact registry (`artifact.NewStore`) only injects the resolver into the backend when `opts.Resolver != nil`, and
> the S3 store's `initIdentityClient` falls back to the default AWS credential chain when no resolver was injected.
> So an identity name without a resolver is silently ignored - it happens to "work" for an `ambient` identity (the
> default chain picks up the runner credentials) but NOT for `aws/assume-role`, `aws/permission-set`, etc. That
> contradicts the requirement below that it work with any identity kind. The fix must also attach the resolver, and it
> must do so even when the component itself has no identity (today `attachIdentity` returns early when
> `info.Identity == ""`, so it would never attach a resolver for a store-only identity).

A correct implementation attaches the resolver for whichever identity is chosen:

```go
// Store-level identity takes precedence over the component's active identity.
storeIdentity := storeSpec.Identity
if storeIdentity == "" {
    storeIdentity = opts.Info.Identity
}
// Attach the identity AND the auth resolver (from the component's auth manager).
// The resolver is required; without it the identity-aware backend falls back to
// the default credential chain and the named identity is ignored.
attachIdentityFor(&artOpts, opts.Info, storeIdentity)
```

where `attachIdentityFor` generalizes the existing `attachIdentity` to take an explicit identity name and to attach the
resolver whenever an identity is present (component or store) and an auth manager is available:

```go
func attachIdentityFor(artOpts *artifact.StoreOptions, info *schema.ConfigAndStacksInfo, identity string) {
    if info == nil || identity == "" {
        return
    }
    artOpts.Identity = identity
    if info.AuthManager == nil {
        return
    }
    if authManager, ok := info.AuthManager.(authtypes.AuthManager); ok {
        artOpts.Resolver = defaultResolverFactory(authManager, info)
    }
}
```

This allows:

```yaml
components:
  terraform:
    planfiles:
      default: "s3"
      stores:
        s3:
          type: aws/s3
          identity: aws-backend    # uses this identity for S3 access
          options:
            bucket: "my-planfiles-bucket"
            prefix: "atmos/planfiles/"
            region: "us-east-1"
```

The component continues to use its default identity (`azure-plan`) for Terraform operations, while the S3 planfile store
uses `aws-backend` for storage operations.

## Additional Considerations

- The `identity` field should work with any identity kind - not just `ambient`. A production setup might use
  `aws/assume-role` for the planfile store and `azure/subscription` for the component. (This is exactly the case the
  resolver-attachment correction above is needed for.)
- When `identity` is empty (not specified), the current behavior is preserved: the component's active identity is used.
- The same pattern already exists in the `stores` configuration (`stores.*.identity` in
  `examples/auth-stores/atmos.yaml`), so this would bring consistency between general stores and planfile stores. The
  general `stores` config already has an `identity` field (`pkg/store/config.go`), and the shared resolver already
  supports AWS, Azure, and GCP.
- The `identity` value should reference an identity defined in `auth.identities`, validated at config load time. Today
  an unknown `identity:` key under a planfile store is silently ignored (the config decoder does not reject unknown
  keys), so adding the field has no effect until `createPlanfileStore` reads it - another reason to add explicit
  validation alongside the field.

## Workaround

Until this is fixed, use one of:

1. **`ambient` as default identity**: Set the AWS-capable identity as `default: true` and handle the non-AWS cloud auth
   via environment variables (e.g., `ARM_USE_OIDC=true` for Azure). Planfile store works because ambient passes through
   AWS credentials from the runner environment.

2. **Disable planfile storage**: Remove the `planfiles` configuration. Plans still run but planfiles are not
   auto-uploaded. Manual upload via `atmos terraform planfile upload` with `--identity=aws-backend` works for the CLI
   commands (they accept `--identity`).

## Verification Against the Codebase

Checked against current `main`. All referenced symbols exist and behave as described; file:line references are
approximate and may drift.

- `attachIdentity()` and `createPlanfileStore()` - `pkg/ci/executor.go` (`attachIdentity` ~L242, `createPlanfileStore`
  ~L185). Confirmed: the config path always calls `attachIdentity(&artOpts, opts.Info)` with no per-store identity.
- `PlanfileStoreSpec` - `pkg/schema/schema.go` (~L1135). Confirmed: it has only `Type` and `Options`, no `Identity`.
  The config key is `components.terraform.planfiles` (yaml tag `planfiles`).
- S3 store - `pkg/ci/artifact/s3/store.go`. Confirmed: eager default-chain init when `Identity` is empty
  (`NewStore` / `initDefaultClient`); deferred `initIdentityClient` otherwise, which falls back to the default chain
  when no resolver was injected.
- `StoreOptions` already carries both `Identity` and `Resolver` - `pkg/ci/artifact/store.go`. So the end-to-end
  plumbing to pass a per-store identity already exists; the only missing pieces are the config field and the wiring in
  `createPlanfileStore`.
- Resolver injection is gated on `opts.Resolver != nil` - `pkg/ci/artifact/registry.go` (`NewStore` calls
  `SetAuthContext(opts.Resolver, opts.Identity)` only when a resolver is set). This is why setting the identity name
  alone is insufficient.
- The resolver is multi-cloud - `pkg/store/authbridge/resolver.go` implements `ResolveAWSAuthContext`,
  `ResolveAzureAuthContext`, and `ResolveGCPAuthContext`, each authenticating the passed identity name via the auth
  manager. The S3 backend calls `ResolveAWSAuthContext`.
- Error sentinel - `pkg/store/errors.go`: `ErrAuthContextNotAvailable = "auth context not available for identity"`,
  matching the reported error output.
- Registered planfile backends - `aws/s3`, `local/dir`, and `github` only (`artifact.Register` call sites). No Azure
  Blob or GCS backend exists yet, so the reverse multi-cloud example is currently hypothetical.
- Workaround 2 - `atmos terraform planfile upload` exists (`cmd/terraform/planfile/upload.go`) and `--identity` is a
  global flag handled in `cmd/root.go`, so it is inherited by the subcommand.
