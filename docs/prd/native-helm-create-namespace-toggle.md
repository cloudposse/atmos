# PRD: Native Helm `create_namespace` Setting

## Overview

This PRD adds a `create_namespace` setting to native Helm components. It controls whether Helm
creates the target namespace during install when it does not exist. The setting defaults to `true`,
preserving the existing native-helm behavior of creating the configured namespace automatically.
Setting it to `false` makes Helm install into a pre-existing namespace instead of creating one.

Native Helm already lets a component configure the target `namespace` so a chart that does not specify
one is not installed into `default`. Atmos then creates that namespace automatically, so a release
deploys in a single operation. This setting makes that automatic creation configurable, so a
deployment that should not create the namespace can turn it off.

## Problem Statement

### Current State

Native Helm deploys a release through the Helm Go SDK. On the install path, Atmos set the SDK's
`CreateNamespace` field to a hardcoded `true`:

```go
// pkg/component/helm/client.go (before)
client.CreateNamespace = true
```

Automatic namespace creation was therefore always on, with no way to turn it off per component.

### Gaps

1. **Not configurable.** There was no component setting and no CLI flag to control namespace creation. Every install attempted to create the namespace, whether or not that was wanted.
2. **Forced creation fails for namespace-scoped identities.** When the identity running `atmos helm apply` is scoped to a single namespace and lacks cluster-level permission to create namespaces, the forced namespace `CREATE` is rejected with `403 Forbidden`. This happens *even when the namespace already exists*, because Helm still issues the `CREATE` and the `403` arrives before the `AlreadyExists` check can matter.
3. **Conflicts with platform-owned namespaces.** When a platform team owns namespaces and their guardrails (labels, quotas, NetworkPolicies) as separate infrastructure, the release should not also create the namespace. There was no way to defer to the platform.

## Proposed Solution

Add a per-component `create_namespace` boolean setting, defaulting to `true` for backward
compatibility. Thread it into the resolved chart spec and onto the Helm install action. Set it to
`false` to install into a pre-existing namespace without a namespace `CREATE`:

```yaml
components:
  helm:
    backend-api:
      chart: "backend-api"
      namespace: backend-api
      create_namespace: false
      values:
        replicaCount: 1
```

Namespace creation only happens on the install path (a fresh release), so the setting only affects
the first install. Upgrades are unchanged.

### Why config, not a CLI flag

`create_namespace` is a declarative, stable property of the deployment contract. It mirrors the
existing `namespace` setting, which is config-only with no CLI flag, and it is tied to how the
component's namespace is managed rather than to a single invocation. Keeping it in versioned,
reviewable stack configuration (rather than an ad-hoc flag) avoids drift where a deploy only works
because someone passed a flag by hand. A per-invocation override flag can be added later if a concrete
need appears, designed as a tri-state override so a bool default does not clobber the component config.

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

1. **`chartSpec.CreateNamespace bool`** (`chart.go`): the resolved spec carries the setting.
2. **`resolveCreateNamespace(section)`** (`values.go`): reads the top-level `create_namespace` key, defaulting to `true`. Backed by a new `boolFieldDefault(section, key, fallback)` helper that distinguishes an unset key from an explicit `false` (a plain type assertion cannot).
3. **`buildChartSpec`** (`values.go`): populates `CreateNamespace` from `resolveCreateNamespace`.
4. **`newInstallClient(actx, spec, dryRun)`** (`client.go`): extracted from `installRelease`; wires `client.CreateNamespace = spec.CreateNamespace` instead of the hardcoded `true`, keeping `installRelease` a thin call plus `runInstall`.

The default of `true` preserves existing behavior for every component that does not set the key.

### Behavior

| Setting | First install (new release) | Upgrade (existing release) |
|---|---|---|
| absent / `create_namespace: true` | Helm creates the namespace if missing, then installs (existing behavior) | Namespace untouched |
| `create_namespace: false` | Helm installs into the existing namespace; no `CREATE` issued | Namespace untouched |

### When to Use `false`

Set `create_namespace: false` when the namespace is managed separately, for example when a platform
component owns namespace labels, quotas, or NetworkPolicies, or when the identity running
`atmos helm apply` is scoped to a single namespace and cannot create namespaces. In that case the
release installs into the pre-existing namespace with no cluster-level namespace-create permission.

`create_namespace` only controls the namespace-create API call; it does not restrict what the chart
installs. The deploying identity still needs permission for every object in the chart, so a chart that
includes cluster-scoped objects (for example a `ClusterRole`, a `ClusterRoleBinding`, or a CRD) still
requires the corresponding cluster-level permissions regardless of this setting.

## Testing

Unit tests in `pkg/component/helm` (`create_namespace_test.go`, `values_extra_test.go`):

- `resolveCreateNamespace`: default-true, explicit true/false, non-bool fallback.
- `boolFieldDefault`: present true/false, absent, non-bool value.
- `buildChartSpec` propagation: absent → `true`, explicit → `false`.
- `newInstallClient`: mirrors `spec.CreateNamespace` onto the install action (true and false), plus release name, namespace, and version wiring, exercised against an in-memory action context.
- A recording kube client asserts, in both directions, that Helm builds and creates a `Namespace` only when `create_namespace` is `true`.

Package coverage is 90.7%, with the changed functions at 100%.

## Documentation

- `website/docs/stacks/components/helm.mdx`: documents the setting under the component settings.
- `agent-skills/skills/atmos-helm/SKILL.md`: adds the setting to the component field reference.

## Backward Compatibility

Fully backward compatible. The setting defaults to `true`, so components that do not set it keep the
exact prior behavior (automatic namespace creation). No migration is required.

## Future Work

- Optional per-invocation override flag (`--create-namespace` / `--create-namespace=false`) on
  `apply`/`deploy`, as a tri-state override that inherits the component config when unset, if a
  concrete need materializes.
