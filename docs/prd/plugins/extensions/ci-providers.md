# CI provider extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. New configuration/SDK symbols below are proposed APIs. [All extension types](README.md).

## Platform providers and component reporting

A CI provider integrates the platform running Atmos: environment detection, run/repository/review context, base resolution, outputs, summaries, checks, and comments. Component CI extensions interpret infrastructure results. These are different contracts and may ship in different integration packages.

Reuse the [provider registry](../../../../pkg/ci/registry_provider.go) and [component CI registry](../../../../pkg/ci/plugin_registry.go). Adapt versioned wire records to their internal interfaces; internal Go structs, writers, and callbacks are not the public SDK.

## Consumer experience

```shell
atmos plugin install acme/ci --as corporate-ci --version '~1.0'
atmos plugin info corporate-ci
atmos plugin install --frozen
atmos terraform plan vpc -s production --ci
```

Example proposed configuration:

```yaml
ci:
  provider: acme/runner
```

The plugin exports `acme/runner`; identity and package alias need not match. Project-selected providers participate in CI resolution; unrelated global installations do not. Explicit selection wins and must fail clearly if unavailable or incompatible.

Automatic detection collects candidates, rather than accepting the first registry-map iteration match. Static environment predicates narrow candidates without running every installed plugin during ordinary CLI startup. Bounded detection callbacks may inspect declared runner context when CI is needed; they must not write externally. Ambiguous matches require selection. With no selection/match, preserve the built-in generic/local fallback.

## Author contract

```shell
atmos plugin create acme-ci --capability ci.provider --runtime go --kind acme/runner
```

The Starlark scaffold supports the same contract. Generate detection/context fixtures, mock reporting APIs, schemas, and release workflows.

| Operation | Contract |
|---|---|
| Detection/context | Runner identity, repository, commit, review/run IDs, event, and trust context. |
| Base resolution | Explicit base ref or SHA, head SHA, target branch, checkout classification, and resolution source. Unknown values remain explicit. |
| Outputs/summaries | Structured output and report requests; adapters handle platform-specific files/APIs. |
| Checks/status | Create/update checks, stable identifiers, conclusions, and annotations. |
| Comments | Explicit review/commit targets and stable markers for updates. |
| Optional features | Explicit support declarations for log grouping, labels, commit comments, debug context, or other supported API contracts. |

Not every CI platform supports every reporting operation. The protocol defines these surfaces and an explicit unsupported result. Required configured reporting fails if unsupported; optional reporting follows Atmos's documented local-render fallback. Native interfaces must not fabricate checks or report a write as successful when it was skipped.

Atmos owns credentials, masking, CI enablement, fork/review trust gates, target validation, and reporting policy. Plugin annotations or detection results cannot override machine/CI policy. Infrastructure operations are not retried because reporting failed. Any reporting retry must use the operation's explicit idempotency/update contract.

An engine's `ci.component` extension receives authoritative operation outcomes and calls Atmos reporting service. Atmos dispatches to the selected platform. Engine code does not implement a separate GitHub/GitLab client.

External artifact/cache backend contracts remain a follow-on; preserve existing built-in behavior and return unsupported for unimplemented optional operations rather than assuming every provider supplies storage.

## Integration

| Aspect | Today |
|---|---|
| Registry | Providers: [`ci.Register(p provider.Provider)`](../../../../pkg/ci/registry_provider.go) keyed by `Name()`. Component reporting: [`ci.RegisterPlugin(p plugin.Plugin) error`](../../../../pkg/ci/plugin_registry.go). |
| Duplicate policy | `ci.Register` has no validation and replaces by name. `ci.RegisterPlugin` errors on duplicates. |
| Context on the interface | Yes for providers. Component plugins receive it through `HookContext`. |
| Reachable from outside the module | Providers: yes, through the public alias [`ci.Provider`](../../../../pkg/ci/provider.go). Component plugins: no. |
| Closed lists to open | `Detect()` returns the first match while iterating a Go map, so detection order is nondeterministic. `ResolveProvider` falls back to `generic`. No `ci.provider` selector exists; it is new. |
| Boundary leaks | `Plugin`, `HookBinding`, and `HookContext` live in `pkg/ci/internal/plugin` with no public alias, so only code under `pkg/ci/...` can implement them (P0.5). `OutputWriter` and `io.Writer` appear on the provider interface. |

The artifact ([`artifact.Register`](../../../../pkg/ci/artifact/registry.go)) and cache ([`cache.Register`](../../../../pkg/ci/cache/registry.go)) registries remain follow-ons.

**Adapter:** `pkg/plugin/adapter/ci` implements [`provider.Provider`](../../../../pkg/ci/internal/provider/types.go) and calls the `CIProvider` wire service. `pkg/plugin/adapter/cicomponent` implements the component `Plugin` and calls `CIComponent`. Providers consume `api.HTTP`, `api.Secrets`, and `api.Log`; component extensions report through `api.CIReporter`.

**Injection points:**
- `ci.ResolveProvider`: explicit `ci.provider`, then an ordered candidate list that includes plugin providers, then the generic fallback.
- `ci.Detect`: deterministic ordering, with static environment predicates narrowing candidates before any plugin runs.
- `ci.GetPluginForEvent`: consults plugin `ci.component` extensions.
- Reporting calls route through Atmos `CIReporter` service rather than handing plugins an `OutputWriter`.

**Prerequisites:** P0.3 Duplicate policy; P0.5 Public aliases for CI component types; P0.7 Deterministic CI detection and `ci.provider` selector; Phase 3 work packages WP3.7 (CI providers) and WP3.6 (`ci.component`). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `OutputWriter` and `io.Writer`: replaced by streamed output chunks and the `api.Log` and `api.CIReporter` services.
- `HookHandler func(*HookContext)`: replaced by the `CIComponent` service with a structured outcome and reporting requests.
- `HookContext.Provider`, `TemplateLoader`, and `CreatePlanfileStore`: Atmos-side; plugins receive resolved values and reporting requests.

## Acceptance

- A Go and a Starlark provider pass mocked detection, context, base resolution, and reporting fixtures.
- A built-in engine reports through an external provider; an external engine reports through a built-in provider; both external extensions work together.
- Explicit selection, ambiguous detection, generic fallback, project/global isolation, and metadata-only startup are tested.
- Tests cover partial reporting failures, permission denial, fork trust, secret redaction, unsupported features, stable comment/check updates, cancellation, and crashes.
- Reports preserve the original infrastructure outcome even when publication fails. Plugin updates require no Atmos rebuild.
