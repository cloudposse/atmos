# PRD: Atmos Operator (Kubernetes Controller for Day-2 Reconciliation)

**Status:** Proposed
**Version:** 0.1
**Last Updated:** 2026-06-25
**Author:** Atmos Team

**Related PRDs:**
- [Atmos Git (GitOps Enablement)](./git-ops.md)
- [Component Registry Pattern](./component-registry-pattern.md)
- [Provisioner System](./provisioner-system.md)
- [Atmos Pro STS](./atmos-pro-sts.md)
- [Backend Provisioner](./backend-provisioner.md)

---

## Executive Summary

Atmos today is a **GitOps publisher**: it computes desired state and hands it to external
reconcilers (CI, Argo CD, Flux). The [Atmos Git PRD](./git-ops.md) states this explicitly —
*"Atmos is the producer side of a GitOps pipeline, not the reconciler."*

This PRD proposes a **second mode of operation**: an **Atmos Operator** — a Kubernetes
controller that continuously reconciles infrastructure declared as Atmos config, *in addition
to* the existing one-shot CLI. The same `atmos` binary plays three roles: the imperative CLI
(today), the in-cluster operator + runner, and a `kubectl`/`tfctl`-like client for the new
custom resources.

The model is borrowed directly from **[tofu-controller](https://github.com/flux-iac/tofu-controller)**
(the Flux OpenTofu/Terraform controller), with one decisive difference: where a tofu-controller
`Terraform` CR is **fat and self-contained** (hand-authored path, vars, backend), an Atmos
`AtmosComponent` CR is **thin** — a `(repository, stack, component)` pointer — and the controller
runs the **Atmos stack processor in-cluster** to *compute* the full desired state (deep-merged
imports/inheritance, backend, providers, vars). You keep Atmos's DRY config model **and** gain
pull-based reconciliation.

This is **day-2** infrastructure on top of an already-running platform. It does **not** replace
how the foundational platform (accounts, networking, the cluster itself) is provisioned — that
stays imperative `atmos terraform apply` via CI, because an operator cannot bootstrap the
cluster it runs inside.

---

## Problem Statement

1. Atmos can compute rich, DRY desired state but cannot *enforce* it — drift and remediation
   are delegated to CI pipelines or external reconcilers.
2. External reconcilers (Argo CD, Flux, tofu-controller) require either Kubernetes manifests or
   self-contained Terraform CRs. Neither understands Atmos stacks, imports, inheritance, or the
   component DAG, so users lose the DRY model the moment they hand off to a reconciler.
3. There is no Kubernetes-native, continuous, pull-based way to manage day-2 resources *as Atmos
   components* — with transparent, gated approval and drift detection.
4. Approval of infra changes is ad hoc (a CI job, a manual `apply`). There is no first-class,
   auditable "suspend until approved" surface.

Atmos needs a controller that reconciles Atmos components in-cluster while preserving the Atmos
configuration model, with transparent approval and drift handling.

---

## Goals

1. **Continuous reconciliation** of Atmos components declared as Kubernetes custom resources.
2. **Preserve the Atmos config model** — the CRD is a thin pointer; desired state is computed
   in-cluster by the Atmos stack processor from a referenced repository.
3. **Polymorphic over component kinds** — reuse the existing component-kind registry
   (`pkg/component/registry.go`: terraform, helmfile, packer, ansible, container).
4. **Transparent, gated approval** — a plan → approve → apply lifecycle. Works without Atmos
   Pro (git-commit approval, à la tofu-controller); Atmos Pro is the premium approval UX.
5. **Drift detection** — periodic re-plan with status reporting and optional remediation.
6. **CLI-native lifecycle** — `atmos operator install` to bootstrap the controller, and
   `atmos apply|get|plan|reconcile|suspend|resume` over the CRs (one binary, no `kubectl`
   required for day-to-day).

## Non-Goals

1. **Layer-0 / foundational provisioning** — accounts, networking, the cluster itself, and the
   bootstrap of the operator stay imperative `atmos terraform apply` via CI. The operator manages
   resources *on* the platform, never the accounts/cluster it runs *on*.
2. **Auto-generating CRs** from a repository (Argo `ApplicationSet`-style discovery). v1 uses
   **hand-authored** CRs. Generation is a possible future enhancement.
3. **Replacing Atmos Pro's CI-based drift/remediation** — the operator is an *additional*,
   in-cluster execution backend, not a replacement.

---

## The Hybrid / Bootstrapping Boundary

Every GitOps-for-infra tool draws the same line — Flux, Argo CD, and Crossplane must all be
bootstrapped onto a pre-existing cluster, then they manage everything after. Atmos is no
different.

- **Layer 0 — foundational (Atmos CLI as today):** accounts, networking, the EKS/k8s cluster,
  and the bootstrap of the operator + GitOps CD. Imperative `atmos terraform apply`, run from CI.
  Cannot be operator-managed (chicken-and-egg).
- **Layer 1 — day-2 (the operator):** everything layered onto the running platform, declared as
  CRs and reconciled continuously. Runs in a **management cluster** (same cluster as workloads,
  or a dedicated control-plane cluster — see Open Questions).

---

## Architecture: Three Planes

- **Config plane** — the git repo (`AtmosRepository`), source of truth for `atmos.yaml`, stacks,
  components, imports, inheritance.
- **Execution plane** — the operator + **ephemeral `atmos`-runner Pods** reconciling in-cluster.
- **Control / approval plane** — **Atmos Pro**: shows plans, suspends until approved, surfaces
  drift, records the audit trail. Optional — the OSS operator runs without it.

### Controller ↔ Runner split (from tofu-controller)

tofu-controller does not run Terraform inside the controller. It spawns an **ephemeral runner
Pod per `Terraform` resource** and talks to it over gRPC, then tears the Pod down. The Atmos
operator adopts the same pattern: each `AtmosComponent` reconcile spawns a runner Pod that runs
`atmos <kind> plan|apply` — **the `atmos` binary is the runner image**. This makes the
"`AtmosComponent` ≈ Pod" mental model literally true, and it naturally supports heterogeneous
component kinds because `atmos` already dispatches by kind.

The controller, before invoking a runner, runs the **Atmos stack processor** against the
checked-out `AtmosRepository` to compute the component's effective configuration (deep-merged
vars, backend, providers, `depends_on`). The runner receives a fully-resolved component to plan
and apply.

---

## CRD Reference

API group (proposed): `atmos.tools/v1alpha1` — confirm.

### `AtmosRepository` — the config-plane source

Handle to a git repo + revision. Gives the controller awareness of a project. Analogous to Flux
`GitRepository`. The single onboarding primitive.

```yaml
apiVersion: atmos.tools/v1alpha1
kind: AtmosRepository
metadata:
  name: platform
  namespace: atmos-system
spec:
  url: https://github.com/acme/platform.git
  ref:
    branch: main             # or tag: / semver: / commit:
  interval: 1m               # how often to poll the source for new revisions
  secretRef:
    name: git-credentials    # optional; reuses Atmos Auth / Git service
status:
  resolvedRevision: main@sha1:abc123...
  atmosConfig: discovered    # atmos.yaml found and parsed
  conditions:
    - type: Ready
      status: "True"
      reason: ImportsResolved
```

### `AtmosStack` — heterogeneous, DAG-ordered aggregate

A first-class aggregate of a *heterogeneous*, DAG-ordered set of components — the thing
tofu-controller lacks (it leans on Flux `Kustomization.dependsOn`). Status rolls up from
children. Loosely "Deployment-like" (parent reconciles children, health bubbles up), but its
children are **heterogeneous** (unlike a Deployment's homogeneous Pods), so ordering follows the
Atmos DAG (like Argo sync-waves), not replica semantics.

```yaml
apiVersion: atmos.tools/v1alpha1
kind: AtmosStack
metadata:
  name: plat-ue2-prod
  namespace: atmos-system
spec:
  repositoryRef:
    name: platform
  stack: plat-ue2-prod
  # v1: components are hand-authored as AtmosComponent CRs and reference this stack.
  # The stack rolls up their status and enforces DAG ordering.
status:
  components: 7
  ready: 6
  pending: 1        # e.g. one component awaiting approval
  conditions:
    - type: Ready
      status: "False"
      reason: ComponentPendingApproval
```

### `AtmosComponent` — the atomic reconciled unit (~ Pod)

Thin reference; polymorphic over the component-kind registry. The controller computes the rest.

```yaml
apiVersion: atmos.tools/v1alpha1
kind: AtmosComponent
metadata:
  name: vpc-plat-ue2-prod
  namespace: atmos-system
spec:
  repositoryRef:
    name: platform
  stack: plat-ue2-prod
  component: vpc
  kind: terraform              # terraform | helmfile | packer | ansible | container
  approvePlan: ""              # "" = hold for approval; "auto"; or "plan-<rev>-<hash>"
  interval: 10m                # reconcile cadence
  driftDetectionInterval: 1h   # re-plan to detect drift
  writeOutputsToSecret:
    name: vpc-outputs          # outputs persisted to a namespace-scoped Secret
  destroyResourcesOnDeletion: true   # finalizer runs `atmos terraform destroy`
  serviceAccountName: atmos-runner   # identity for the runner Pod (IRSA / Workload Identity)
status:
  lastPlannedRevision: main@sha1:abc123...
  plan: plan-main-abc123        # pending plan id to approve
  lastAppliedRevision: ""
  drift: false
  conditions:
    - type: Ready
      status: "False"
      reason: PendingApproval
```

---

## Reconciliation Lifecycle

Borrowed from tofu-controller: **Plan → Approve → Apply → Output**, re-triggered on
source-revision change, spec change, or drift interval.

1. **Source** — `AtmosRepository` polls git; a new revision produces an artifact.
2. **Compute** — controller runs the Atmos stack processor to resolve the component's effective
   config for its `stack`.
3. **Plan** — a runner Pod runs `atmos <kind> plan`; the plan id is written to
   `status.plan` and the component condition becomes `PendingApproval` (unless `approvePlan: auto`).
4. **Approve** — see the approval model below; flips the gate.
5. **Apply** — a runner Pod runs `atmos <kind> apply` against the approved plan.
6. **Output** — outputs written to a namespace-scoped Secret (`writeOutputsToSecret`).
7. **Drift** — every `driftDetectionInterval`, re-plan; a non-empty plan sets `status.drift: true`
   and (optionally) triggers remediation.

Dependencies (`dependsOn`) are derived from the **Atmos DAG** (`depends_on`, output wiring), not
hand-authored — dependents read producer outputs via remote-state, consistent with Atmos today.

---

## Approval Model

Transparent, gated approval is a first-class characteristic, with two tiers:

- **OSS tier (no Pro) — git-commit fallback (à la tofu-controller):** the controller surfaces
  the plan id; an operator releases it by setting `spec.approvePlan: "plan-<rev>-<hash>"`
  (in the CR / in git) or `auto` for unattended apply. `suspend`/`resume` pause and resume
  reconciliation.
- **Pro tier — premium approval UX:** Atmos Pro is the **control/approval plane**. The plan/diff
  is shown transparently in the dashboard; the deployment sits **Suspended → PendingApproval**;
  an approval click flips the `approvePlan` gate so the runner proceeds. Pro already owns the
  primitives — `list_approvals`/`get_approval`, `get_deployment`/`summarize_deployments`,
  `get_drift_schedule`/`remediate_drift`, `list_instances` — so the operator **drives Pro's
  existing approval + drift model from the cluster side** rather than inventing a new one.

---

## CLI Surface (the `tfctl` analogue)

One binary manages bootstrap *and* the resource lifecycle:

- `atmos operator install` / `uninstall` — bootstrap the controller + CRDs onto a cluster
  (like `tfctl install`, `flux install`).
- `atmos apply | get | plan | delete | reconcile | suspend | resume` — operate on the CRs from
  the same CLI, no `kubectl` required for day-to-day.

---

## Mapping to tofu-controller (reference)

| tofu-controller | Atmos operator |
|---|---|
| Flux `GitRepository` source | `AtmosRepository` |
| `Terraform` CRD | `AtmosComponent` (polymorphic kind) |
| *(none)* | `AtmosStack` (heterogeneous DAG aggregate) |
| ephemeral runner Pod runs `terraform` | runner Pod runs `atmos <kind> plan\|apply` (atmos = runner image) |
| `spec.approvePlan: auto \| plan-<rev>-<hash>` | `spec.approvePlan` (plan→approve→apply gate) |
| `spec.sourceRef` | `spec.repositoryRef` + `stack` + `component` |
| `spec.vars / varsFrom` | **computed by the Atmos stack processor** (not hand-authored) |
| `writeOutputsToSecret` | `writeOutputsToSecret` (namespace-scoped Secret) |
| `dependsOn` between Terraform CRs | `dependsOn` derived from the Atmos DAG |
| `driftDetectionInterval` | `driftDetectionInterval` → status (+ optional remediation) |
| `destroyResourcesOnDeletion` | finalizer runs `atmos <kind> destroy` |
| `tfctl install/get/plan/reconcile/...` | `atmos operator install` + `atmos apply/get/plan/...` |

---

## Phasing

- **Phase 1 (MVP):** the three CRDs (`AtmosRepository`, `AtmosStack`, `AtmosComponent`), a
  reconcile loop, the `terraform` kind only, ephemeral runner Pods, `approvePlan` git-commit
  gate, outputs-to-Secret, destroy finalizer. E2E on k3s/kind against a **mock component**
  (no cloud creds): apply CRs → runner Pods spawn → plan produced → approve → apply → outputs
  in Secret → deletion triggers destroy.
- **Phase 2:** remaining kinds (helmfile, packer, ansible, container); drift remediation.
- **Phase 3:** Atmos Pro approval/control-plane integration (suspend-until-approved, transparent
  diff, audit).
- **Future (out of scope):** `ApplicationSet`-style auto-generation of CRs from `AtmosRepository`.

---

## Open Questions

1. **Desired-state computation site (recommend "embed"):** operator embeds the Atmos stack
   processor and computes in-cluster from `AtmosRepository` (recommended — keeps the DRY model)
   vs. CLI/CI renders and the operator only applies (thin operator).
2. **Management cluster:** same cluster as workloads vs. a dedicated control-plane cluster.
3. **State/backend:** reuse existing Atmos backends (S3/GCS/Azure, via the Backend Provisioner)
   vs. a k8s-secret backend like tofu-controller's default.
4. **API group/version:** confirm `atmos.tools/v1alpha1`.
5. **Runner identity:** how `serviceAccountName` maps to cloud identity (IRSA / Workload
   Identity) and integrates with Atmos Auth.

---

## Prior Art / References

- [`docs/prd/git-ops.md`](./git-ops.md) — Atmos as publisher; explicitly *not* a reconciler today
  (this PRD adds the reconciler mode for day-2).
- [`pkg/component/registry.go`](../../pkg/component/registry.go) — the component-kind registry
  the operator is polymorphic over.
- [`docs/prd/provisioner-system.md`](./provisioner-system.md) — self-registering lifecycle hooks
  (adjacent pattern).
- [tofu-controller](https://github.com/flux-iac/tofu-controller) /
  [docs](https://flux-iac.github.io/tofu-controller/) — the architectural precedent.
