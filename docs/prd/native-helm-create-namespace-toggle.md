# PRD: Native Helm `create_namespace` Toggle

## Overview

This PRD adds a `create_namespace` setting to native Helm components. It controls whether Helm
creates the target namespace during the first install. The setting defaults to `true` (the existing
behavior), and setting it to `false` makes Helm install into a pre-existing namespace instead of
creating one.

The motivation is least-privilege delivery: a namespace-scoped identity (for example a CI identity
with namespace-scoped Kubernetes RBAC) cannot create namespaces, so the forced namespace creation
turns the first `atmos helm apply` into a `403`. The toggle lets the platform own the namespace and
lets the scoped identity run the install.

## Problem Statement

### Current State

Native Helm deploys a release through the Helm Go SDK. On the install path, Atmos set the SDK's
`CreateNamespace` field to a hardcoded `true`:

```go
// pkg/component/helm/client.go (before)
client.CreateNamespace = true
```

With `CreateNamespace = true`, the Helm install action issues a namespace `CREATE` API call before
installing the release objects. Helm only ignores the result when the API returns `AlreadyExists`.

### Gaps

1. **No way to disable namespace creation.** There was no component setting and no CLI flag to turn
   the behavior off. Every first install attempted to create the namespace.
2. **Fails for namespace-scoped identities.** When the identity running `atmos helm apply` is scoped
   to a single namespace and lacks cluster-level permission to create namespaces, the namespace
   `CREATE` call is rejected with `403 Forbidden`. This happens *even when the namespace already
   exists*, because Helm still issues the `CREATE` and the `403` arrives before the `AlreadyExists`
   check can matter.
3. **The workaround is awkward.** The only way through was to run the first install as a
   higher-privileged operator (which creates the release), after which the scoped identity's later
   applies succeed as upgrades — the upgrade path does not run the namespace-create block. Requiring
   an operator step for every new app or environment is friction, and it couples release ownership to
   a privilege the app pipeline should not need.

This is the natural division of responsibility for platform teams: the platform owns the namespace
and its guardrails (NetworkPolicies, quotas, labels), and the application pipeline only deploys
workloads into it. The forced namespace creation broke that split.

## Proposed Solution

Add a per-component `create_namespace` boolean setting, defaulting to `true` for backward
compatibility. Thread it into the resolved chart spec and onto the Helm install action so
`create_namespace: false` installs into a pre-existing namespace without a namespace `CREATE`.

```yaml
components:
  helm:
    typescript-api:
      chart: "../../../apps/typescript-api/charts/typescript-api"
      namespace: typescript-api
      create_namespace: false   # platform pre-creates the namespace; CI installs into it
      values:
        replicaCount: 1
```

Namespace creation only happens on the install path (a fresh release), so the setting only affects
the first install. Upgrades are unchanged.

### Why config, not a CLI flag

`create_namespace` is a declarative, stable property of the deployment contract — it mirrors the
existing `namespace` setting, which is config-only with no CLI flag. It is tied to the RBAC shape of
the identity that deploys the component, which does not change from run to run. Keeping it in
versioned, reviewable stack configuration (rather than an ad-hoc flag) avoids drift where a deploy
only works because someone passed a flag by hand. A per-invocation override flag can be added later
if a concrete need appears (for example an operator bootstrap), designed as a tri-state override so a
bool default does not clobber the component config.

## Detailed Design

### Schema Changes

Add `create_namespace` (boolean) to the `helm_component_manifest` definition in the manifest and
stack-config JSON schemas, alongside `namespace`:

- `pkg/datafetcher/schema/atmos/manifest/1.0.json`
- `pkg/datafetcher/schema/stacks/stack-config/1.0.json`
- `tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` (test fixture kept in parity)

The manifest schema uses `additionalProperties: false`, so the property must be enumerated or stack
validation rejects it. The website schema copy is generated from the embedded manifest schema and
needs no manual edit.

### Implementation Changes

`pkg/component/helm`:

1. **`chartSpec.CreateNamespace bool`** (`chart.go`) — the resolved spec carries the setting.
2. **`resolveCreateNamespace(section)`** (`values.go`) — reads the top-level `create_namespace` key,
   defaulting to `true`. Backed by a new `boolFieldDefault(section, key, fallback)` helper that
   distinguishes an unset key from an explicit `false` (a plain type assertion cannot).
3. **`buildChartSpec`** (`values.go`) — populates `CreateNamespace` from `resolveCreateNamespace`.
4. **`newInstallClient(actx, spec, dryRun)`** (`client.go`) — extracted from `installRelease`; wires
   `client.CreateNamespace = spec.CreateNamespace` instead of the hardcoded `true`, keeping
   `installRelease` a thin call plus `runInstall`.

The default of `true` preserves existing behavior for every component that does not set the key.

### Behavior

| Setting | First install (new release) | Upgrade (existing release) |
|---|---|---|
| absent / `create_namespace: true` | Helm creates the namespace if missing (existing behavior) | Namespace untouched |
| `create_namespace: false` | Helm installs into the existing namespace; no `CREATE` issued | Namespace untouched |

### Operating Pattern

With `create_namespace: false`, the recommended flow is:

1. The platform creates the namespace and its guardrails (NetworkPolicies, quotas, labels) as
   platform-owned infrastructure — a native Kubernetes component, Terraform, or equivalent.
2. The application's namespace-scoped identity runs `atmos helm apply`, including the first install,
   installing only namespaced resources into the pre-existing namespace.

No operator bootstrap step is required.

## Testing

Unit tests in `pkg/component/helm` (`create_namespace_test.go`, `values_extra_test.go`):

- `resolveCreateNamespace`: default-true, explicit true/false, non-bool fallback.
- `boolFieldDefault`: present true/false, absent, non-bool value.
- `buildChartSpec` propagation: absent → `true`, explicit → `false`.
- `newInstallClient`: mirrors `spec.CreateNamespace` onto the install action (true and false), plus
  release name, namespace, and version wiring, exercised against an in-memory action context.
- End-to-end install with `create_namespace: false` against the in-memory storage driver.

Package coverage rose from 89.3% to 90.1%, with the changed functions at 100%.

## Documentation

- `website/docs/stacks/components/helm.mdx` — documents the setting under the component settings.
- `agent-skills/skills/atmos-helm/SKILL.md` — adds the setting to the component field reference.

## Backward Compatibility

Fully backward compatible. The setting defaults to `true`, so components that do not set it keep the
exact prior behavior. No migration is required.

## Future Work

- Optional per-invocation override flag (`--create-namespace` / `--create-namespace=false`) on
  `apply`/`deploy`, as a tri-state override that inherits the component config when unset, if an
  operator-bootstrap need materializes.
