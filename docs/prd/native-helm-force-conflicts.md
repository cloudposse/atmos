# PRD: Native Helm Server-Side Apply Conflict Control

**Status:** Implemented

**Last Updated:** 2026-10-02

**Related:** [Native Helm Release Lifecycle](./native-helm-release-lifecycle.md), [Native Helm create_namespace Toggle](./native-helm-create-namespace-toggle.md), [PR #2667](https://github.com/cloudposse/atmos/pull/2667)

**Upstream references:**

-
Kubernetes - [Server-Side Apply: conflicts and field management](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
- Helm 4 - `action.Install.ForceConflicts` / `action.Install.ServerSideApply` and `action.Upgrade.ForceConflicts` /
  `action.Upgrade.ServerSideApply`, plumbed to `kube.ClientUpdateOptionServerSideApply(serverSideApply, forceConflicts)`
-
Kubernetes - [#136949 "Do not remove managedFields if conversion webhook fails"](https://github.com/kubernetes/kubernetes/issues/136949)
(the scenario that orphans field ownership on affected clusters)

## Summary

Native Helm applies releases with Helm 4, which uses Kubernetes server-side apply by default. Under server-side apply,
every field of a managed object is owned by a field manager, and an apply that writes a field already owned by a
different manager fails with a conflict unless the apply is told to force it. Helm 4 exposes exactly this control on its
install and upgrade actions (`ForceConflicts`, alongside the `ServerSideApply` mode), but Atmos configures neither, so
the controls sit at their defaults and a field-ownership conflict aborts the release.

This PRD exposes `server_side_apply` and `force_conflicts` as native Helm release-policy settings, resolved through the
same stack configuration, base-component inheritance, and command-line override path as the rest of the release
lifecycle, and plumbed to the Helm 4 action. With `force_conflicts` enabled, a release resolves a field-ownership
conflict in the normal deploy path and becomes the sole manager of the contested fields, instead of failing and
requiring an out-of-band manual repair. The default preserves current behavior: forcing conflicts is an explicit opt-in,
because it overrides other field managers.

## Problem

Helm 4 applies release manifests server-side by default, so field ownership of a managed object is shared with any other
actor that writes the same fields. Two situations put another field manager on fields a release also sets:

1. **A controller reconciles the same object.** An operator that continuously updates an object it also received from a
   release can take ownership of fields the release declares.
2. **An object's field-ownership ledger is orphaned.** When an object's `managedFields` is lost - for example, a custom
   resource whose CRD hosts its own conversion webhook loses its ledger if a conversion fails during a controller
   disruption, on Kubernetes versions before the upstream fix - the next server-side apply synthesizes a stand-in
   manager that owns the pre-existing fields. A subsequent release apply that changes those fields then conflicts with
   the stand-in.

In both cases the server-side apply reports a field-ownership conflict and Atmos aborts the install or upgrade. Because
Atmos sets no conflict-resolution option, there is no way to clear the conflict through the normal deploy path. The
operator must repair the object out of band (hand-edit `managedFields`, or run a forced
`kubectl apply --server-side --force-conflicts`) and then re-run Atmos. This:

- breaks dependency-ordered rollouts, since the conflicted release never reaches a successful completion state and its
  dependents stay blocked;
- cannot be remediated in CI, where out-of-band cluster surgery is not available; and
- hides a control Helm 4 already implements, because Atmos never surfaces it.

## Current State

| Capability             | Current native Helm behavior                                                                       | Consequence                                                         |
|------------------------|----------------------------------------------------------------------------------------------------|---------------------------------------------------------------------|
| Apply method           | Helm 4 default (server-side apply)                                                                 | Managed-object fields participate in field-ownership tracking       |
| Force conflicts        | Unset - Helm action `ForceConflicts` defaults to `false`                                           | A field owned by another manager makes the apply fail               |
| Server-side apply mode | Unset - Helm action default (`true` for install, `"auto"` for upgrade)                             | No component-level control and no escape hatch to client-side apply |
| Conflict recovery      | Out-of-band only (manual `managedFields` repair or `kubectl apply --force-conflicts`), then re-run | Ordered rollouts and CI cannot self-heal a conflict                 |

## Goals

- Expose `force_conflicts` on the native Helm release policy and plumb it to the Helm 4 install and upgrade action
  `ForceConflicts`.
- Expose `server_side_apply` on the release policy and plumb it to the Helm 4 action `ServerSideApply`, honoring the
  Helm 4 shape difference between install and upgrade.
- Default to current behavior: when unset, `force_conflicts` is off and the apply method is the Helm 4 default.
- Resolve both settings through stack type defaults, base-component inheritance, concrete component configuration, and
  command-line overrides, consistent with the existing release lifecycle.
- Validate the configured values before chart download or cluster mutation.
- Document when forcing conflicts is appropriate and the ownership consequence of enabling it.

## Non-Goals

- Repairing objects whose `managedFields` ledger was already orphaned. That is a one-time operational action on the
  cluster, independent of this control (forcing conflicts on the next apply resolves the conflict going forward, but
  does not retroactively reconstruct lost ownership history).
- Changing Helm's default apply method or forcing server-side apply off by default.
- Per-field or per-object conflict policy. The setting is release-scoped, matching the Helm action surface.
- Any identity, chart-acquisition, or external-target delivery change.

## Design

### Configuration surface

Add two keys to the native Helm `release` policy block, alongside the existing wait, timeout, history, install, and
upgrade controls:

```yaml
components:
  helm:
    my-component:
      release:
        # Apply method. Omit to use the Helm 4 default.
        server_side_apply: true
        # Resolve field-ownership conflicts by overwriting the contested fields
        # and becoming their sole manager. Opt-in; default false.
        force_conflicts: false
```

A release-wide setting is the common case. Where a release needs different behavior for first install versus later
upgrades, the keys may also be set under the existing per-phase `install` and `upgrade` policy blocks, mirroring how
release timeout already supports a per-phase override over a release-wide default.

### Helm 4 action shape

The two Helm 4 actions do not type `ServerSideApply` identically, and the plumbing must preserve that:

- Install exposes `ServerSideApply bool` (default `true`) and `ForceConflicts bool`.
- Upgrade exposes `ServerSideApply string` with values `auto`, `true`, `false` (default `auto`, which inherits the prior
  release's apply method), and `ForceConflicts bool`.

The configuration accepts `auto | true | false` for `server_side_apply`. For the install action, `auto` and `true` both
map to `true`; for the upgrade action the value passes through. `force_conflicts` is a boolean mapped directly to each
action's `ForceConflicts`.

### Plumbing

The settings follow the existing release-policy path: new fields on the install and upgrade policy input structs,
decoded alongside the other release keys, resolved into the effective release policy, and set on the Helm action where
the other lifecycle options are already configured for install and upgrade. No Helm SDK modification is required - both
controls are first-class action fields that flow to the kube client's server-side apply options.

### Command-line overrides

Add `--force-conflicts` and `--server-side-apply` overrides to the apply and deploy operations, consistent with the
existing per-operation lifecycle flag overrides, so an operator can force a single recovery apply without editing stack
configuration.

## Behavior

With `force_conflicts` enabled, a release apply that meets a field owned by another manager overwrites the contested
fields and the release's field manager becomes their sole owner, per Kubernetes server-side apply semantics. This
resolves an orphaned-ownership or controller-ownership conflict in the normal `atmos helm apply` path, so the release
reaches a successful completion state and its dependents proceed, and CI needs no out-of-band cluster surgery.

Because forcing overrides other managers, the control is opt-in and release-scoped. When a controller legitimately
co-owns fields a release also sets, enabling `force_conflicts` means each release apply reclaims those fields from the
controller; that trade-off is the operator's to make per component, which is why the default leaves conflicts fatal and
visible.

## Acceptance

- A release whose managed object has a field owned by a different field manager fails with a conflict when
  `force_conflicts` is unset (current behavior) and succeeds, taking ownership of the contested fields, when
  `force_conflicts` is enabled.
- `server_side_apply` toggles the apply method: `auto | true | false` honored for upgrade, boolean for install; omitting
  it preserves the Helm 4 default.
- Both settings resolve correctly through stack type defaults, base-component inheritance, concrete component
  configuration, and command-line override, and are validated before any chart download or cluster mutation.
- With both settings unset, native Helm behavior is byte-for-byte unchanged from today.

## Implementation

The settings reuse the existing native Helm release-policy pipeline:

- **Configuration keys** - `server_side_apply` (accepts `auto`, `true`, or `false`; a
  YAML boolean is normalized to its string form) and `force_conflicts` (boolean) are
  added to the release-wide `release` block and the per-phase `install` and `upgrade`
  blocks. They are rejected on the `delete` block, which has no server-side apply
  surface. Section-name constants live in `pkg/config/const.go`.
- **Decode** - `pkg/component/helm/lifecycle_decode.go` decodes both keys into the
  presence-aware input structs. A dedicated `optionalServerSideApplyField` accepts a
  YAML boolean or the string `auto`, and `rejectUnknownFields` allow-lists them only
  where they apply.
- **Resolve** - `pkg/component/helm/lifecycle.go` resolves the apply method through
  built-in default (unset) -> release-wide -> per-phase -> CLI flag, for install and
  upgrade only. `parseServerSideApply` validates the enum before any chart download or
  cluster mutation (the decode pass resolves every operation up front).
- **Plumb** - `pkg/component/helm/client.go` sets the Helm 4 action fields, honoring the
  shape difference: `action.Install.ServerSideApply` is a bool (`auto`/`true` map to
  `true`), `action.Upgrade.ServerSideApply` is the pass-through string, and both actions
  take `ForceConflicts`. When `server_side_apply` is unset, Atmos sets nothing so the
  Helm default is preserved.
- **Command-line overrides** - `cmd/helm/helm.go` adds `--server-side-apply` (bare value
  selects `true`) and `--force-conflicts` to the `apply` and `deploy` operations, mapped
  into the lifecycle flag overlay.
- **Schema** - the native Helm policy definitions in both
  `pkg/datafetcher/schema/atmos/manifest/1.0.json` and
  `pkg/datafetcher/schema/stacks/stack-config/1.0.json` model the two keys on the
  install, upgrade, and release policies (not delete). `server_side_apply` uses a
  `oneOf` of boolean or the string enum.
- **Error** - `ErrHelmServerSideApplyInvalid` in `errors/errors.go`.

Tests cover parsing, release-wide and per-phase resolution precedence, CLI flag
precedence, delete inapplicability, bool-and-string decoding, schema validation (both
copies), and the install/upgrade action plumbing including the unset-preserves-default
guarantee.
