# Secret provider extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. [All extension types](README.md).

A secret-provider extension adds a backend for Atmos secret declarations and commands. It is distinct from an authentication extension that obtains session credentials. A package may export both, or expose a secret-capable store through the existing store-backed adapter.

## Consumer experience

```shell
atmos plugin install acme/storage --as storage --version '~1.0'
atmos secret list -s production -c app
atmos secret get API_TOKEN -s production -c app
```

A secret-capable plugin store uses existing `store:` declarations. A direct provider needs a general provider selector rather than another hardcoded backend key. Proposed component configuration:

```yaml
components:
  terraform:
    app:
      secrets:
        providers:
          corporate:
            kind: acme/secrets
            options:
              endpoint: https://secrets.acme.example
        vars:
          API_TOKEN:
            provider: corporate
            scope: instance
```

`provider` is a new selector for a named registered provider, mutually exclusive with existing `store` and `sops` selectors for one declaration. Existing forms retain their behavior. Provider-specific configuration comes from its packaged schema.

## Contract

Extend the [secret provider construction path](../../../../pkg/secrets/providers/registry.go) and [provider capabilities](../../../../pkg/secrets/providers/provider.go). The existing registry dispatches backend tracks; a generic external-provider track must resolve plugin kind metadata rather than adding a hardcoded track for every enterprise backend.

- Support typed Set/Get, existence status, declared deletion support, and instance/stack/global scopes. Unsupported scopes/operations fail before writes.
- Optional raw reads preserve original bytes/text as defined by the credential/value contract. Key generation, rotation, export, file reset, or other advanced operations require separate explicit support; they are not assumed.
- Existence checks must not retrieve/decrypt values. Preserve local-versus-remote status behavior: normal listing does not implicitly authenticate or contact remote backends; explicit verification can do so.
- Atmos applies identity selection, secret masking, safe output behavior, scope addressing, and persistence policy. Secret values cannot appear in manifests, catalogs, lockfiles, tests, or diagnostic context.
- Distinguish missing from denied, unavailable, and unsupported. A write timeout may have changed the backend; never silently retry or claim rollback.
- Detect provider/auth/store bootstrap cycles before invocation. OCI authentication cannot depend on installing the same plugin first.

## Integration

| Aspect | Today |
|---|---|
| Registry | [`providers.Register(track string, c Constructor)`](../../../../pkg/secrets/providers/registry.go), keyed by **track** (`store`, `sops`), not by kind. `Constructor` receives `*schema.AtmosConfiguration`, a name, and `sectionProviders map[string]any`. |
| Duplicate policy | Panics on duplicate. |
| Context on the interface | No. [`Provider`](../../../../pkg/secrets/providers/provider.go) methods `Set`, `Get`, `Delete`, and `Status` take a `Coordinate`, not a context. |
| Reachable from outside the module | Yes. |
| Closed lists to open | The track set. Secret declarations choose `store` or `sops`; the `provider:` selector is new. |
| Boundary leaks | The constructor takes `*schema.AtmosConfiguration`. Optional `RawGetter`, `FileResettable`, `LocalStatus`, and key-generation interfaces exist. |

**Adapter:** `pkg/plugin/adapter/secret` implements `providers.Provider` and registers a generic external track keyed by kind. It calls the `SecretProvider` wire service and consumes the `api.Credentials`, `api.HTTP`, and `api.Log` API services. A secret-capable plugin store reuses the existing store-backed adapter and the `Store` service instead.

**Injection points:**
- `providers.New`: a generic external track resolves `kind` through the plugin registry.
- Secret declaration parsing: accept `provider:` as mutually exclusive with `store` and `sops`.
- `atmos secret` commands and `!secret` resolution: unchanged.
- Local-versus-remote status: listing stays offline; explicit verification may contact the backend.

**Prerequisites:** P0.4 Context plumbing (secrets `Provider`); P0.9 Secret-provider kind registry under a generic track; Phase 3 work package WP3.4 (stores and secrets). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `*schema.AtmosConfiguration` (constructor input): replaced by the validated options map and scope addressing.
- Secret values in diagnostics, manifests, and lockfiles: never serialized outside the `Get`/`Set` payload.

## Authoring and acceptance

`plugin create --capability secret.provider` supports Go and Starlark with mock backends, schemas, positive/negative scenarios, and release workflows. Test existing secret commands and YAML resolution, typed/raw values, scopes, missing/denied states, no-fetch listing, explicit verification, secret-store restrictions, credential isolation, cancellation, and redaction.

A package exposing both store and secret-provider extensions installs once and updates atomically. Built-in store/SOPS declarations remain compatible; all tests use synthetic secrets by default.
