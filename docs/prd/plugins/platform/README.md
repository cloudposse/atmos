# Plugin platform workstream

**Status:** Native pilot implemented with remaining release and follow-on work; engineering design for [Independently Versioned Atmos Plugins](../overview.md). The product PRDs say what plugins must do; these documents say what Atmos must build. [All plugin PRDs](../README.md).

The [first-release scope](../first-release.md) is authoritative for delivery gates. The [gap analysis](gap-analysis.md) records the original inventory; use the first-release checkpoint for current implementation status.

| Document | Contents |
|---|---|
| [Gap analysis](gap-analysis.md) | Every existing registry with its duplicate policy, context plumbing, reachability, and closed lists; registries to create; registries to change; missing injection points; internal types that must not cross the boundary; follow-on surfaces. |
| [Interfaces](interfaces.md) | Go package layout under `pkg/plugin`, core types, installed-plugin registry, runtimes, API services, one adapter per extension type, installer and catalog interfaces, bootstrap order. |
| [Protocol](protocol.md) | `hashicorp/go-plugin` transport, `atmos.plugin.v1` services, API services over the broker, shared messages, versioning, error model, Starlark parity, data runtime, Wasm follow-on. |
| [Configuration](configuration.md) | The `plugins:` section, user-scope configuration, `.atmos.plugins.lock.yaml`, the four manifest kinds, catalog and test configuration, `path` packages, new stack fields, on-disk layout, schema touchpoints. |
| [Errors](errors.md) | Sentinel errors with hints and exit codes, wire error classes, masking rules, per-extension failure semantics. |
| [Plugin policy](../policy.md) | Shared CEL/Starlark admission, company registries, inspected metadata, cache/layout/development origins, and trust boundaries. |
| [Decisions](decisions.md) | Numbered decision records with alternatives, and open questions. |
| [Shared OCI foundation](oci-reuse.md) | Existing OCI consumers, artifacts-design convergence, ownership, auth, and round-trip verification. |
| [Delivery plan](delivery-plan.md) | Registry foundations validated with the pilot, then package lifecycle, runtimes, extension adapters, and authoring/release, as work packages with dependencies and done-criteria. |

## Decisions that shape these documents

- **Transport.** Native plugins use `hashicorp/go-plugin` in gRPC mode with Atmos-owned `.proto` definitions. Wasm through Extism is the declared follow-on runtime. Native commands use `Commands.Run` with streamed stdout, stderr, and a terminal exit event; the proposed Starlark runtime runs in-process. [Decision 1](decisions.md), [Decision 2](decisions.md).
- **Validate together, split after.** The auth registry refactor and first-release foundations remain with the working OIDC/plugin implementation until validated. Split into reviewable PRs afterward. Remaining registry work accompanies its adapter. [Decision 13](decisions.md).
- **Shared admission and presentation.** CEL/Starlark `on: plugin` gates use the existing policy engine. Plugin views use `pkg/list`; parallel downloads use the toolchain batch UI. [Plugin policies](../policy.md), [first release](../first-release.md).
- **Complete inventory.** The gap analysis covers all 12 planned extension types in full and every follow-on surface in one row each. [Decision 16](decisions.md).
- **Atmos API and Plugin SDK.** `pkg/api` is the shared programmatic interface (renamed from `pkg/automation` in P0.12). Language bindings expose it as `atmos.*`; the plugin wire representation is `atmos.api.v1`. The nested `sdk/` module supplies plugin authoring tools and API bindings, versioned in lockstep with Atmos. [Decision 17](decisions.md), [Decision 18](decisions.md).
- **Contract testing.** Schema compatibility is a `buf breaking` gate on the proto; behavioral conformance is the Atmos-shipped `atmos plugin test` suite plus pinned fixture plugins. The plugin boundary is not a Pact candidate. [Decision 15](decisions.md).

## Reading order

1. [Gap analysis](gap-analysis.md): what exists and what is missing.
2. [Interfaces](interfaces.md) and [protocol](protocol.md): what Atmos and the wire look like.
3. [Configuration](configuration.md) and [errors](errors.md): what users and authors see.
4. [Decisions](decisions.md): why.
5. [Delivery plan](delivery-plan.md): in what order.
