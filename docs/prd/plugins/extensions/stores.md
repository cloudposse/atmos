# Store extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. [All extension types](README.md).

A store extension adds a backend to normal Atmos store operations and YAML resolution. It can share a package with [secret providers](secret-providers.md), auth, commands, and tools.

```shell
atmos plugin create acme-storage --capability store --capability secret.provider --runtime go
atmos plugin install acme/storage --as storage --version '~1.0'
```

Example configuration:

```yaml
stores:
  corporate:
    kind: acme/kv
    options:
      endpoint: https://store.acme.example
```

The exported kind supplies its options schema; core configuration must not need a hardcoded entry for each backend. Existing `kind`, legacy `type`, `identity`, and `secret` behavior remains supported.

## Contract

Adapt the [store registry](../../../../pkg/store/registry.go) and [store interfaces](../../../../pkg/store/store.go). Core operations are scoped Set/Get and direct-key lookup. Optional features include raw reads, deletion, existence checks, and key enumeration. Unsupported operations fail explicitly, never as an empty successful response.

Requests carry typed values, stack/component/key coordinates, options, scoped identity context, cancellation, and deadlines. Values preserve null versus missing, structured versus raw data, and backend error categories. Resolve plugin metadata before store references are evaluated; detect configuration/auth/store cycles with their dependency path.

Secret-capable stores declare that capability and their sensitive-storage mode. Replace hardcoded backend capability lists with registered metadata for external kinds, while retaining built-in defaults. Metadata is not independent proof of encryption or publisher trust. Atmos policy decides which backends are allowed for sensitive storage.

A store marked `secret: true` follows the existing secret-only access path. Generic store access cannot bypass secret masking or membership rules. Auth contexts and connection reuse remain isolated by configured instance and realm.

## Integration

| Aspect | Today |
|---|---|
| Registry | [`store.Register(kind string, factory StoreFactory)`](../../../../pkg/store/registry.go); `StoreFactory func(name, StoreConfig) (Store, error)`. The legacy `type` key maps to `kind`. |
| Duplicate policy | Panics on duplicate. |
| Context on the interface | No. [`Store`](../../../../pkg/store/store.go) methods `Set(stack, component, key, value)`, `Get(stack, component, key)`, and `GetKey(key)` take no context. |
| Reachable from outside the module | Yes. |
| Closed lists to open | `secretByDefaultKinds` and `secretIncapableKinds` are hardcoded in `registry.go`. |
| Boundary leaks | [`IdentityAwareStore`](../../../../pkg/store/identity.go) uses a per-cloud `AuthContextResolver`. Optional `RawStore` and `DeletableStore` interfaces exist. |

**Adapter:** `pkg/plugin/adapter/store` implements `store.Store` and the optional interfaces the manifest declares, and calls the `Store` wire service. It consumes the `api.Credentials`, `api.HTTP`, and `api.Log` API services.

**Injection points:**
- `NewStoreRegistry`: a kind miss consults plugin stores. The adapter must check for collisions first, because `store.Register` panics.
- Secret capability: read registered capability metadata from the manifest instead of the two hardcoded lists, for external kinds only.
- Identity-aware stores: pass a generic scoped identity reference that Atmos resolves, instead of a per-cloud resolver.

**Prerequisites:** P0.4 Context plumbing (`Store`); Phase 3 work package WP3.4 (stores and secrets). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `AuthContextResolver` (per cloud): replaced by a generic `IdentityRef{Name, Realm}` that the plugin resolves through `api.Credentials`.
- `StoreConfig` internals and `*schema.AtmosConfiguration`: replaced by the options map validated against the exported kind schema.

## Acceptance

Both runtimes pass typed/raw round trips, missing values, optional operations, scope isolation, invalid options, auth failures, cancellation, crash, and redaction tests. A plugin backend works through existing store/secret adapters without a core rebuild. Tests reject collisions before the internal registry can panic. Writes/deletes are not automatically retried after an uncertain outcome.

Blob artifacts, caches, and Terraform state-backend lifecycle are separate surfaces; implementing a store does not implicitly implement them.
