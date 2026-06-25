# PRD: Atmos Operator (Kubernetes Controller for Day-2 Reconciliation)

**Status:** Proposed
**Version:** 0.3
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

**This is an Atmos solution, not a Terraform one.** tofu-controller is the precedent for the
*controller mechanics* (a CRD-driven reconcile loop, ephemeral runner Pods, plan → approve →
apply) — it is **not** the scope. The scope is **every Atmos component kind**: terraform,
helmfile, packer, ansible, and container, via Atmos's existing component-kind registry
(`pkg/component/registry.go`). The runner runs `atmos <kind>`, so the operator is kind-agnostic
by construction. **Terraform is one use case among several.** Wherever this PRD shows a terraform
example, any other kind applies equally — an `AtmosComponent` could just as well reconcile a
Helmfile release, bake a Packer image, run an Ansible play, or manage a container.

This is **day-2** infrastructure on top of an already-running platform. It does **not** replace
how the foundational platform (accounts, networking, the cluster itself) is provisioned — that
stays imperative `atmos terraform apply` via CI, because an operator cannot bootstrap the
cluster it runs inside.

---

## Separation of Concerns: Runner vs Governance

The single most important principle in this design — it determines what belongs in the CLI, what
belongs in the controller, and what belongs in Atmos Pro:

> **The Atmos CLI is a *runner*. It handles what can be reasonably accomplished in a single
> execution.** Anything that requires more than that — tracking *pending* approvals, approval
> *history*, *presenting* them, or *bubbling up* status across many resources over time — is
> **governance**, and governance lives in **Atmos Pro**.

- **Runner (single execution):** compute one component's desired state; one `plan`; one `apply`;
  one `destroy`. Stateless and ephemeral. **Identical whether invoked by a human, by CI, or as a
  runner Pod the operator schedules** — the runner Pod is just the CLI executing one unit of work.
- **Controller (orchestration):** the long-running reconcile loop that schedules runners and holds
  only the minimal per-resource state Kubernetes needs (the CR + its status). It deliberately does
  **not** become a governance system.
- **Governance (longitudinal, stateful):** pending/historical approvals, audit, cross-resource
  roll-up, and presentation. This is **Atmos Pro**.

Every capability below is placed on one side of this line. The CLI can perform the single
mechanical *act* of approval; the *system* of approvals (who/when/policy/history/presentation)
is Pro.

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

## Positioning & Target Use Cases

The operator is a **first-class replacement for a hand-rolled control plane** — the pattern of
stitching together **Tofu-Controller + Argo CD + the external-secrets operator** to get a
Crossplane-style "control plane." That assembly paints a great picture but is rough under the
hood (and integrating Atmos with Argo CD specifically tends to be awkward). The operator makes
that a supported, native capability instead of glue.

It opens two markets the CI-publisher model doesn't reach:

1. **CI-tool independence.** Teams that don't want GitHub Actions or GitLab pipelines — they have
   a Kubernetes cluster and want it to reconcile their infrastructure. The cluster *is* the
   runtime; no external CI required.
2. **Air-gapped / isolated / government clusters.** Highly isolated environments (a common
   requirement in public-sector and regulated work) can run the operator because of its
   **egress-only posture** (see Communication & Network Posture): the cluster never needs inbound
   access from GitHub/GitLab or Atmos Pro.

These both depend on keeping controller↔repository and controller↔Pro communication limited and
secured — which the design does by construction.

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

## Multi-Tenancy & Namespace Model

The CRDs are namespaced, and that is load-bearing: a single cluster hosts **many independent
`AtmosRepository` objects**, not one. The centralized "platform" repo is **optional** — any
application repository that defines its own components can register its **own** `AtmosRepository`
and own its components and how they deploy, without overloading a central repo.

- **A namespace is a tenant.** Each namespace maps to one (or a few) `AtmosRepository` objects —
  typically one per application repo. Teams control their own namespace, repo ref, runner
  ServiceAccount (and therefore cloud identity), and approval policy.
- **No central bottleneck.** App teams `kubectl apply` their `AtmosRepository` +
  `AtmosComponent`s into their namespace. A platform team can still run a central repo for
  shared/foundational day-2 resources, but it is not a chokepoint.
- **Isolation maps to existing tofu-controller multi-tenancy.** Runner Pods run in the CR's
  namespace under `spec.serviceAccountName`, so RBAC, NetworkPolicy, resource quotas, and cloud
  identity (IRSA / Workload Identity) are all per-tenant and enforced by Kubernetes.

```yaml
# Namespace "team-payments" owns its own Atmos repo + components — no central repo required.
apiVersion: atmos.tools/v1alpha1
kind: AtmosRepository
metadata:
  name: payments
  namespace: team-payments
spec:
  url: https://github.com/acme/payments.git   # the application's OWN repo
  ref: { branch: main }
  interval: 1m
---
apiVersion: atmos.tools/v1alpha1
kind: AtmosComponent
metadata:
  name: payments-api-ue2-prod
  namespace: team-payments
spec:
  repositoryRef: { name: payments }   # namespace-local reference
  stack: plat-ue2-prod
  component: payments-api
  kind: helmfile
  serviceAccountName: payments-runner  # tenant cloud identity
```

`repositoryRef` resolves within the CR's own namespace, keeping tenants isolated by default.

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
  lastSyncedRevision: main@sha1:abc123...
  lastPollTime: "2026-06-25T17:04:00Z"   # surfaced for staleness detection
  atmosConfig: discovered    # atmos.yaml found and parsed
  conditions:
    - type: Ready
      status: "True"
      reason: ImportsResolved
    - type: SourceStale       # raised if now - lastPollTime exceeds the staleness threshold
      status: "False"
      reason: PolledRecently
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
  destroyResourcesOnDeletion: true   # finalizer runs `atmos <kind> destroy` (terraform shown; any kind)
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

Approval maps directly onto the runner-vs-governance line: the **act** of approving is a single
execution (flip the gate on one component now); the **system** of approvals (which are pending,
their history, who approved, presentation) is governance. So the two tiers are not "basic vs
fancy" — they are *mechanism* vs *governance*:

- **OSS / CLI tier = the mechanical act**, within a single execution, with no memory of pending
  or historical approvals.
- **Pro tier = governance** — the durable, presented record of pending approvals, history, audit,
  and cross-resource roll-up.

In detail:

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

One binary manages bootstrap *and* the resource lifecycle, using the **user's existing
kubeconfig** (standard client-go loading rules: `--kubeconfig` / `--context` / `KUBECONFIG` /
`~/.kube/config` / in-cluster). No hand-written `kubectl patch` incantations.

- `atmos operator install [--upgrade]` / `uninstall` — bootstrap the controller + CRDs onto a
  cluster (like `tfctl install`, `flux install`). `--upgrade` makes it a single idempotent
  install-or-upgrade verb (`helm upgrade --install` semantics): install if absent, upgrade in
  place if present.
- `atmos operator get | plan | reconcile | delete` — inspect and drive CRs.
- `atmos operator approve <stack>/<component> [--plan <id>]` — flip the `approvePlan` gate
  (wraps the API patch the user would otherwise do by hand).
- `atmos operator suspend | resume <stack|component>` — pause/resume reconciliation.

These verbs talk to the Kubernetes API directly with the caller's kubeconfig — they are the
ergonomic alternative to `kubectl patch atmoscomponent ... --type=merge -p '{"spec":{"approvePlan":"plan-..."}}'`.
This `atmos operator …` sub-command namespace exists to *operate the controller from the command
line*; performing an approval this way surfaces the manual mechanics, consistent with the
runner-vs-governance separation — the CLI does the single act; Pro governs the system of approvals.

---

## Kubernetes Data Source — reading secrets back (the read side of the loop)

`writeOutputsToSecret` is the write side. The symmetric read side is a new **`kubernetes` store
provider** in the existing store registry (`pkg/store/`, alongside SSM / Secrets Manager / Azure
Key Vault / GCP SM / Redis / Artifactory). Because Atmos has already abstracted *what it is to be a
store*, adding a read-capable provider is a natural extension of an existing interface, not a new
subsystem. It reads values from Kubernetes Secrets/ConfigMaps, exposed through the usual Atmos
surfaces (`!store`, `atmos.Store`, gomplate datasources):

```yaml
# Component B reads an output Component A wrote to a Secret — a closed in-cluster loop.
vars:
  vpc_id: !store kubernetes vpc-outputs vpc_id
```

This closes the loop (A writes → B reads) entirely in-cluster and interoperates cleanly with the
**external-secrets operator** (ESO): Atmos can read Secrets ESO syncs *in*, and ESO can sync
Atmos-written output Secrets *out*.

**Resolving the "traditional CLI/CI flow" concern (a k8s auth profile).** A `kubernetes` store
needs cluster credentials, exactly like the SSM store needs AWS creds — so it is the same
"this store requires credentials" pattern, not a new class of coupling, and it is **opt-in per
store config** (stacks that don't reference it are unaffected). Credentials are resolved via a
**Kubernetes auth profile** through Atmos Auth:

- **In the operator:** in-cluster config (the runner Pod's ServiceAccount token) — automatic.
- **In CLI/CI:** a configured kubeconfig/context resolved by Atmos Auth (e.g. an identity that
  runs `aws eks get-token` / `gcloud container clusters get-credentials`, or a stored kubeconfig),
  consistent with how Atmos Auth already brokers cloud identities ([Profile vs identity] —
  `--profile` selects the Atmos auth profile, not an AWS profile).

This keeps stack configuration that *doesn't* use the k8s store fully runtime-independent, so the
classic `atmos terraform plan` in CI is unaffected.

---

## Identity & Authentication

**Every operator capability authenticates through one model: Atmos Auth.** That includes stores,
the `kubernetes` store's auth profile, and the runner Pod's cloud identity. There is a single
identity model whether `atmos` runs as a human's CLI, in CI, or as a runner Pod.

### IRSA — already supported, no new mechanism required

A runner Pod gets AWS credentials from an **IRSA-annotated ServiceAccount** (`spec.serviceAccountName`).
Atmos Auth already supports this today:

- The **`aws/ambient` identity** resolves IRSA via the AWS SDK default credential chain — it
  deliberately preserves `AWS_WEB_IDENTITY_TOKEN_FILE` / `AWS_ROLE_ARN`
  (`pkg/auth/identities/aws/ambient.go`, see `docs/prd/ambient-identity.md`).
- The web-identity STS primitive `AssumeRoleWithWebIdentity` is already implemented
  (`pkg/auth/identities/aws/assume_role.go`) and is the **same call** the `github/oidc` →
  `atmos/pro` federation already uses. **IRSA is just a different OIDC issuer** (the EKS cluster's),
  so no new mechanism is needed. The runner identity is `aws/ambient` (IRSA), optionally chained
  to `aws/assume-role` for cross-account.

### Proposed: an explicit `aws/irsa` identity kind (multi-tenant hardening)

For a multi-tenant controller, `aws/ambient` has a sharp edge: the SDK chain can **silently fall
through to the node's IMDS instance-profile role** if a tenant's IRSA is misconfigured — a
classic privilege-escalation footgun. We therefore propose a thin explicit **`aws/irsa`** identity
kind:

- Reads the projected ServiceAccount token file directly + an explicit role ARN / audience /
  session name / duration from config.
- Reuses the existing `AssumeRoleWithWebIdentity` primitive; registered in
  `pkg/auth/factory/factory.go` (~100 LoC, modeled on `aws/ambient`).
- **Fail-fast:** errors if the web-identity token is absent — it **never** falls through to the
  node role.

`aws/ambient` remains the zero-config path; `aws/irsa` is the hardened, explicit, parameterizable
path recommended for runner Pods.

---

## Communication & Network Posture (egress-only)

A deliberate design constraint: **the cluster never needs inbound access from GitHub/GitLab or
Atmos Pro.** Everything is **pull / egress-only**, which makes the operator deployable in locked-
down and air-gapped-leaning environments.

- **Git:** the `AtmosRepository` controller **polls** the source every `spec.interval` (outbound
  443 to GitHub/GitLab) — the Flux source-controller model. **No inbound webhook is required.** An
  optional webhook receiver may be offered later purely as a low-latency optimization, never a
  requirement. v1 is polling-only.
  - **Rate-limit guards:** cheap change detection (`git ls-remote` / conditional request) before a
    full fetch + stack-processor compute; a per-provider concurrency cap and backoff on HTTP 429 /
    secondary rate limits; and **jittered intervals** so many tenant pollers don't synchronize into
    a thundering herd. This mirrors Argo CD, whose default poll is ~3m (120s reconciliation + up to
    60s jitter) with a repo-server concurrent-connection cap; webhooks (when present) bypass jitter.
  - **Staleness guards:** `status.lastPollTime` and `status.lastSyncedRevision` are surfaced, and a
    configurable staleness threshold raises a `SourceStale` / `Stalled` condition so a stuck or
    rate-limited poller is **visible** rather than silently serving stale desired state (Pro bubbles
    this up as governance).
- **Atmos Pro:** the operator **dials out** to Pro to register/report instances and **long-poll
  for approval decisions**. Pro never reaches into the cluster — same egress-only posture as Pro's
  existing push-based GitHub Actions integration. For fully disconnected / no-Pro environments,
  the **git-commit `approvePlan` fallback works with zero Pro connectivity**.

---

## Branch Planner — pre-merge plan previews ("Atmos branches")

The reconciliation loop above is the *post-merge* half of GitOps: desired state on `main` gets
applied. The **Branch Planner** is the *pre-merge* half — plan previews on PR branches — modeled
on tofu-controller's Branch Planner. There is a clean symmetry: `spec.ref.branch: main` →
reconcile/apply; **PR branches → plan-only preview**.

It splits precisely along the runner-vs-governance line:

- **Execution (runner — works without Pro):** the controller polls the git provider API for open
  PRs/MRs and, for each PR branch, spawns a runner Pod that runs `atmos <kind> plan` on that
  branch's in-cluster-computed config (a single execution). The plan result is surfaced on a
  PR-scoped resource status, events, and the `atmos operator` CLI.
- **Presentation (governance — Atmos Pro posts the comments):** Atmos Pro posts/updates the plan
  as a PR comment using its **existing GitHub PR integration** (`list_pull_requests`,
  `create_pull_request_comment`, `get_commit_files`, job summaries), and owns `!replan` handling,
  comment threading, and plan history across PR revisions. Without Pro you still get the plan
  (status / CLI / events); the **auto-PR-comment UX is a Pro feature** — consistent with "OSS works
  without Pro" for the reconcile core.

Egress-only (polls the provider API, posts via Pro's outbound integration) and subject to the same
polling guards below — PR-API polling is heavier than `git ls-remote`, so the rate-limit and jitter
guards matter *more* here.

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
| Branch Planner (PR plan → PR comment) | Branch Planner (runner runs `atmos plan`; Atmos Pro posts the comment) |
| `tfctl install/get/plan/reconcile/...` | `atmos operator install [--upgrade]` + `atmos operator get/plan/reconcile/...` |

---

## Phasing

- **Phase 1 (MVP):** the three CRDs (`AtmosRepository`, `AtmosStack`, `AtmosComponent`), a
  reconcile loop, the `terraform` kind only, ephemeral runner Pods, `approvePlan` git-commit
  gate, outputs-to-Secret, destroy finalizer. E2E on k3s/kind against a **mock component**
  (no cloud creds): apply CRs → runner Pods spawn → plan produced → approve → apply → outputs
  in Secret → deletion triggers destroy.
  Multi-tenancy (multiple namespaced `AtmosRepository` objects, no central repo required) is
  inherent in Phase 1 since the CRDs are namespaced. CLI `approve`/`suspend`/`resume` via the
  user's kubeconfig, `atmos operator install --upgrade`, the polling guards (jitter / rate-limit /
  staleness), and the `aws/irsa` runner identity also land here.
- **Phase 2:** remaining kinds (helmfile, packer, ansible, container); drift remediation; the
  `kubernetes` store provider (read secrets back, with the k8s auth profile) for the closed loop.
- **Phase 3:** Atmos Pro approval/control-plane integration (suspend-until-approved, transparent
  diff, audit) over an egress-only channel; the **Branch Planner** (PR-branch plan previews — the
  runner piece can land earlier, but PR-comment posting depends on Pro).
- **Future (out of scope):** `ApplicationSet`-style auto-generation of CRs from `AtmosRepository`;
  optional inbound webhook receiver for low-latency reconcile.

---

## Future / Exploratory CRDs

### `AtmosWorkflow` (exploratory — not committed)

A Kubernetes-native form of Atmos's existing workflow engine (`pkg/workflow` + the step registry):
**sequential or concurrent steps with user-defined inter-step dependencies (a DAG)**. It fits
runner-vs-governance — each step is a single-execution runner; the DAG state, history, and any
between-step approvals are governance. It is captured here as a direction, **not a committed
phase**, because of three honest caveats:

1. **Overlaps `AtmosStack`.** `AtmosStack` is already a DAG-ordered reconciliation of heterogeneous
   components. `AtmosWorkflow`'s differentiated value is *procedural* orchestration that isn't pure
   convergence — e.g. "run a migration → apply → smoke-test → notify."
2. **Overlaps Argo Workflows.** Generic K8s step/DAG orchestration is exactly what Argo Workflows
   does; we could lean on it rather than build a native engine.
3. **Engine extension, not just a CR wrapper.** Today's Atmos workflows are sequential step lists;
   adding concurrency + inter-step dependencies is a real extension to the workflow engine itself.

---

## Open Questions

1. **Desired-state computation site (recommend "embed"):** operator embeds the Atmos stack
   processor and computes in-cluster from `AtmosRepository` (recommended — keeps the DRY model)
   vs. CLI/CI renders and the operator only applies (thin operator).
2. **Management cluster:** same cluster as workloads vs. a dedicated control-plane cluster.
3. **State/backend:** reuse existing Atmos backends (S3/GCS/Azure, via the Backend Provisioner)
   vs. a k8s-secret backend like tofu-controller's default.
4. **API group/version:** confirm `atmos.tools/v1alpha1`.
5. **Runner identity — RESOLVED:** handled by Atmos Auth (see *Identity & Authentication*).
   Runner Pods use `aws/ambient` (IRSA, zero-config) or the proposed fail-fast `aws/irsa` kind;
   no new auth mechanism is required.

---

## Prior Art / References

- [`docs/prd/git-ops.md`](./git-ops.md) — Atmos as publisher; explicitly *not* a reconciler today
  (this PRD adds the reconciler mode for day-2).
- [`pkg/component/registry.go`](../../pkg/component/registry.go) — the component-kind registry
  the operator is polymorphic over.
- [`docs/prd/provisioner-system.md`](./provisioner-system.md) — self-registering lifecycle hooks
  (adjacent pattern).
- [tofu-controller](https://github.com/flux-iac/tofu-controller) /
  [docs](https://flux-iac.github.io/tofu-controller/) — the architectural precedent (incl. the
  Branch Planner).
- [`pkg/auth/identities/aws/`](../../pkg/auth/identities/aws/) — `aws/ambient` (IRSA today) and
  `AssumeRoleWithWebIdentity`; `docs/prd/ambient-identity.md`.
- [Argo CD reconciliation / jitter / rate limits](https://argo-cd.readthedocs.io/en/stable/operator-manual/high_availability/)
  — prior art for the polling guards.
