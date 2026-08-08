# PRD: Native Helm Release Lifecycle

**Status:** Draft

**Last Updated:** 2026-08-04

**Related:** [DAG-Based Concurrent Execution](./dag-concurrent-execution.md), [Component Dependencies](./component-dependencies.md), [PR #2667](https://github.com/cloudposse/atmos/pull/2667)

## Summary

Add configurable release lifecycle behavior to native Helm components. Atmos will expose Helm 4 wait strategies, release timeouts, failure-recovery actions, Job waiting, release history limits, chart-hook suppression, and CRD installation control through normal Helm component configuration and command-line overrides.

The feature makes native Helm suitable for ordered production rollouts in which a successful component must mean more than "resources were submitted to Kubernetes." A Helm component participating in a dependency DAG completes successfully only after its configured release lifecycle and readiness policy succeeds. Dependents remain blocked when the release fails, times out, or is rolled back.

This PRD includes the release-operation portion of timeout configuration because timeout semantics are inseparable from waiting and rollback. It also requires rendered hook visibility because omitting `release.Hooks` makes template and diff silently incomplete. Chart acquisition, external target delivery, and pre-rollback Kubernetes diagnostics remain separate work.

## Problem

Native Helm components were introduced by PR #2667 as an experimental component type. Atmos can render, diff, install, upgrade, delete, and deliver Helm releases, and Helm components already participate in dependency-ordered bulk execution. The release client, however, currently configures only the release name, namespace, chart version, namespace creation, dry-run mode, and a hard-coded Helm 4 `HookOnlyStrategy`.

The current behavior has four operational gaps:

1. **A successful apply does not imply resource readiness.** `HookOnlyStrategy` waits for chart hooks but does not wait for ordinary Deployments, StatefulSets, Services, or Jobs. A dependent DAG node may start while the prerequisite release is still rolling out.
2. **Users cannot bound or extend release waits.** Helm action timeouts are not configured, leaving native SDK behavior rather than an intentional Atmos contract. Long-running workloads cannot request a 60-minute readiness window, while stuck hooks may wait indefinitely.
3. **Failed releases cannot automatically recover.** Install and upgrade actions do not enable Helm 4 rollback-on-failure or upgrade cleanup.
4. **Common Helm lifecycle controls are missing.** Users cannot configure Job waiting, history retention, chart-hook suppression, or CRD installation behavior.

These gaps force users migrating from Helm or Helmfile to choose between dependency-aware Atmos execution and the release-safety controls they already use in CI.

## Current State

| Capability                   | Current native Helm behavior                                                               | Consequence                                                       |
| ---------------------------- | ------------------------------------------------------------------------------------------ | ----------------------------------------------------------------- |
| Install or upgrade selection | Atmos checks release history and calls separate Helm SDK install or upgrade actions        | Install-only and upgrade-only fields must be applied deliberately |
| Wait strategy                | Hard-coded `hookOnly`                                                                      | Ordinary release resources are not readiness gates                |
| Release timeout              | Helm SDK zero value                                                                        | No intentional Atmos release-timeout contract                     |
| Rollback on failure          | Disabled                                                                                   | Failed installs/upgrades can leave partial state                  |
| Wait for Jobs                | Disabled                                                                                   | Jobs in the ordinary manifest are not readiness gates             |
| Upgrade cleanup              | Disabled                                                                                   | Newly created resources can remain after a failed upgrade         |
| History limit                | Helm SDK zero value                                                                        | Release history is not bounded by Atmos                           |
| Chart hooks                  | Enabled                                                                                    | No component-level control; distinct from Atmos lifecycle hooks   |
| CRD installation             | Enabled for install                                                                        | No component-level control                                        |
| Apply dry-run                | Parsed by the command, but cluster delivery currently passes `false` to the release client | `atmos helm apply --dry-run` can mutate the cluster               |
| Cancellation                 | Several Helm paths replace the caller context with `context.Background()`                  | Signals and scheduler cancellation do not propagate consistently  |

## Goals

- Expose the Helm 4 release lifecycle controls required for production install and upgrade workflows.
- Allow one component to express different install, upgrade, and delete timeouts while inheriting a concise release-wide default policy.
- Preserve Helm 4 concepts and behavior instead of recreating a separate Atmos readiness model.
- Make the resolved lifecycle configuration available through stack type defaults, base-component inheritance, concrete components, and command-line overrides.
- Define an explicit completion contract for Helm nodes participating in `dependencies.components` execution.
- Fix apply dry-run propagation as a release-blocking safety prerequisite, then correctly propagate delete dry-run, cancellation, and deadlines through cluster operations.
- Validate configuration before chart download or cluster mutation.
- Keep template and diff complete by including Helm chart hook resources alongside the ordinary release manifest.
- Keep the design compatible with future pre-rollback diagnostics without requiring another public configuration rename.
- Follow Atmos schema, stack-processing, command parsing, provider, error, logging, and testing conventions.

## Non-Goals

- Collecting Pod status, Kubernetes Events, or container logs before rollback. This release uses Helm's built-in rollback behavior; failure diagnostics require separate orchestration.
- Configuring chart download, OCI registry, client-side render, or external provision-target delivery timeouts.
- Automatically running `helm dependency build` after chart provisioning. Atmos MUST report missing dependencies with the explicit build command. Dependency acquisition remains caller-controlled: Atmos only fetches missing dependencies when the user explicitly passes `--dependency-update`.
- Adding `--set-file`; Atmos `!include.raw` already covers byte-preserving file-backed value content, including a source file's terminal newline. Content normalization is separate from Helm lifecycle behavior.
- Supporting Helm CLI subcommand, getter, downloader, post-renderer, or Wasm plugins in native SDK operations.
- Adding force replacement, server-side apply selection, value reuse/reset, ownership takeover, validation bypass, or uninstall history/cascade options. In particular, Helm `Uninstall.KeepHistory`, the `--keep-history` flag, and a `keep_history` component field are deferred from R1.
- Changing dependency selection or scheduler algorithms.
- Treating a successful rollback as a successful DAG node.

## Design Principles

### Component-Owned Inputs Use a Release Tree

Lifecycle configuration belongs to the Helm component, not to generic `settings`. The design MUST NOT introduce `settings.helm`. Provider-owned inputs remain directly under `components.<type>.<name>`, with lifecycle policy grouped under one `release` field:

```yaml
components:
  helm:
    demo-api:
      chart: oci://registry.invalid/charts/demo-api
      version: 1.2.3
      namespace: demo

      release:
        timeout: 5m
        chart_hooks: true
        wait:
          strategy: watcher
          jobs: true
        history:
          max: 10
        install:
          timeout: 60m
          crds: create
          on_failure: uninstall
        upgrade:
          timeout: 10m
          on_failure: rollback
          cleanup_on_failure: true
```

The hierarchy makes operation applicability part of the schema instead of prose. `crds` exists only under `install`; `cleanup_on_failure` exists only under `upgrade`. Every `release` object rejects unknown keys so a misplaced field fails validation rather than being silently ignored.

### Helm Concepts Are Canonical; Atmos YAML Is Hierarchical

Helm 4 terminology is canonical for concepts and enum values, but its flat CLI flag surface is not the required shape of Atmos YAML. Atmos is a declarative configuration API and uses hierarchy to express release-wide defaults and operation-specific policy.

Flux is the relevant external precedent: [`HelmRelease`](https://fluxcd.io/flux/components/helm/helmreleases/) provides a global timeout and install, upgrade, rollback, and uninstall sections with per-action overrides. Atmos adopts the global-default plus operation-section model, but only exposes actions the R1 executor can control.

R1 intentionally omits `release.rollback`. Helm's `Upgrade.RollbackOnFailure` performs recovery inside the upgrade action using the effective upgrade timeout and wait configuration. Publishing a separate rollback timeout would imply control that Atmos does not have. A rollback section can be added only when Atmos owns rollback as an explicit action with its own context, timeout, reporting, and failure handling.

### Positive Names and Operation-Specific Failure Enums

YAML uses positive names: `chart_hooks` rather than `disable_chart_hooks`, and `install.crds: create | skip` rather than `skip_crds`. The `release` namespace distinguishes Helm chart hooks from the component's existing Atmos `hooks:` section.

`on_failure` is an enum scoped by its operation:

- `install.on_failure`: `uninstall | keep`.
- `upgrade.on_failure`: `rollback | keep`.

`upgrade.cleanup_on_failure` remains an independent Boolean because Helm exposes cleanup independently from rollback. R1 does not offer `upgrade.on_failure: uninstall`; that requires Atmos-managed release-history behavior beyond the current Helm action mapping.

Because native Helm lifecycle configuration remains experimental and none of the earlier proposed names shipped, R1 does not add YAML aliases for `atomic`, `wait`, the former flat fields, or Boolean failure flags. Helm-compatible names remain available where useful on the command line.

### Configuration Describes Policy; Flags Override an Invocation

Stack configuration defines the release's normal policy. Explicit command flags are the highest-priority one-run override for CI and incident response. A stack file MUST NOT outrank `--timeout`: operators must be able to extend or shorten an active deployment without editing and committing configuration.

### Rendering and Acquisition Are Orthogonal

The `release` tree changes only cluster-backed lifecycle resolution. Chart rendering, values precedence, chart-hook rendering, `!include.raw`, dependency diagnostics, and explicit dependency update behavior remain below this layer and are unchanged.

## User Experience

### Type Defaults and Per-Operation Overrides

Stack-level `helm.release` supplies defaults for native Helm components. Normal stack processing deep-merges the complete tree before an operation is selected:

```yaml
helm:
  release:
    timeout: 10m
    chart_hooks: true
    wait:
      strategy: watcher
      jobs: true
    history:
      max: 10
    upgrade:
      on_failure: rollback
      cleanup_on_failure: true

components:
  helm:
    foundation-release:
      chart: charts/foundation-release
      namespace: example-system

    model-release:
      chart: oci://registry.invalid/charts/model-release
      namespace: example-apps
      release:
        install:
          timeout: 60m
        upgrade:
          timeout: 10m
        delete:
          timeout: 5m
      dependencies:
        components:
          - name: foundation-release
```

`model-release` receives 60 minutes for a first install that allocates capacity and downloads artifacts, ten minutes for an upgrade that can reuse cached data, and five minutes for deletion. Other applicable lifecycle values inherit from `helm.release`.

Abstract components can define the same tree:

```yaml
components:
  helm:
    base-release-policy:
      metadata:
        type: abstract
      release:
        timeout: 10m
        wait:
          strategy: watcher
        upgrade:
          on_failure: rollback

    demo-release:
      metadata:
        inherits:
          - base-release-policy
      chart: charts/demo-release
      namespace: demo
      release:
        upgrade:
          timeout: 30m
```

Atmos first performs its normal deep merge across type defaults, inheritance, and the concrete component. It then overlays the selected operation section on the merged release-wide defaults. The effective upgrade timeout above is `30m`; install and delete inherit the release-wide `10m`.

### Command-Line Overrides

The cluster-backed `apply` and `deploy` commands support:

```shell
atmos helm apply demo-api -s example-prod \
  --on-failure=rollback \
  --cleanup-on-failure \
  --wait=watcher \
  --wait-for-jobs \
  --timeout=60m \
  --history-max=10
```

Atmos selects install or upgrade from release state, resolves the effective operation policy, and then applies explicitly supplied flags. `--on-failure` is validated against the selected operation: `uninstall | keep` for install and `rollback | keep` for upgrade.

For Helm 4 parity, `--wait` accepts an optional strategy:

```shell
--wait            # watcher
--wait=watcher
--wait=hookOnly
--wait=legacy
```

The deprecated Boolean forms `--wait=true` and `--wait=false` are accepted with a warning and normalize to `watcher` and `hookOnly`, respectively.

The `delete` command supports the lifecycle subset relevant to uninstall:

```shell
atmos helm delete demo-api -s example-prod \
  --wait=watcher \
  --timeout=10m \
  --no-hooks
```

`template`, `diff`, and `plan` do not register release-lifecycle flags because they do not perform a release operation.

## Configuration Contract

### Release-Wide Defaults

| Field                   | Type                 | Default                              | Helm 4 mapping                               | Applies to               |
| ----------------------- | -------------------- | ------------------------------------ | -------------------------------------------- | ------------------------ |
| `release.timeout`       | Duration string      | `0s` in v1.225.x; `5m` from v1.226.0 | Selected action `Timeout`                    | Install, upgrade, delete |
| `release.chart_hooks`   | Boolean              | `true`                               | Inverse of selected action `DisableHooks`    | Install, upgrade, delete |
| `release.wait.strategy` | Enum                 | `hookOnly`                           | `kube.WaitStrategy`                          | Install, upgrade, delete |
| `release.wait.jobs`     | Boolean              | `false`                              | `Install.WaitForJobs`, `Upgrade.WaitForJobs` | Install, upgrade         |
| `release.history.max`   | Non-negative integer | `10`                                 | `Upgrade.MaxHistory`                         | Upgrade                  |

### Operation Sections

| Field                                | Type                      | Inherits/default        | Helm 4 mapping                               |
| ------------------------------------ | ------------------------- | ----------------------- | -------------------------------------------- |
| `release.install.timeout`            | Duration string           | `release.timeout`       | `Install.Timeout`                            |
| `release.install.chart_hooks`        | Boolean                   | `release.chart_hooks`   | Inverse of `Install.DisableHooks`            |
| `release.install.wait`               | Object                    | `release.wait`          | Install wait strategy and Jobs               |
| `release.install.crds`               | Enum: `create`, `skip`    | `create`                | Inverse of `Install.SkipCRDs`                |
| `release.install.on_failure`         | Enum: `uninstall`, `keep` | `keep`                  | `uninstall` sets `Install.RollbackOnFailure` |
| `release.upgrade.timeout`            | Duration string           | `release.timeout`       | `Upgrade.Timeout`                            |
| `release.upgrade.chart_hooks`        | Boolean                   | `release.chart_hooks`   | Inverse of `Upgrade.DisableHooks`            |
| `release.upgrade.wait`               | Object                    | `release.wait`          | Upgrade wait strategy and Jobs               |
| `release.upgrade.on_failure`         | Enum: `rollback`, `keep`  | `keep`                  | `rollback` sets `Upgrade.RollbackOnFailure`  |
| `release.upgrade.cleanup_on_failure` | Boolean                   | `false`                 | `Upgrade.CleanupOnFail`                      |
| `release.delete.timeout`             | Duration string           | `release.timeout`       | `Uninstall.Timeout`                          |
| `release.delete.chart_hooks`         | Boolean                   | `release.chart_hooks`   | Inverse of `Uninstall.DisableHooks`          |
| `release.delete.wait.strategy`       | Enum                      | `release.wait.strategy` | `Uninstall.WaitStrategy`                     |

Operation objects use `additionalProperties: false`. Fields not listed for an operation are invalid there. Flux supports additional CRD and remediation modes through controller-owned behavior; R1 exposes only native Helm SDK semantics it can guarantee. In particular, `install.crds: replace` is not accepted.

The `install.wait` and `upgrade.wait` objects accept `strategy` and `jobs`. The `delete.wait` object accepts only `strategy`. These nested objects also reject unknown keys.

The target timeout default is five minutes, matching the Helm CLI. R1 introduces that default through one explicit minor-release migration window instead of silently changing existing executions:

1. Atmos v1.225.x is the migration release line. When both the selected operation timeout and `release.timeout` are omitted, Atmos preserves the current SDK zero value and emits one warning per selected Helm component per invocation that the default will become `5m` in v1.226.0.
2. An explicit timeout at either level, including `0s`, suppresses the migration warning.
3. Starting with Atmos v1.226.0, an omitted effective timeout resolves to `5m` and the migration warning is removed.

An explicit effective timeout of `0s` disables the Helm action timeout. Negative durations are invalid. Documentation MUST warn that a zero timeout can leave hook or resource waits unbounded. This is especially important for delete: Helm 4's uninstall request does not accept the caller context, so a zero delete timeout can leave an uninstall wait unbounded after the caller stops waiting. Timeout errors MUST include the effective duration and identify the selected configuration path, such as `release.install.timeout` or the inherited `release.timeout`.

The `release.history.max` default is `10`, matching the Helm CLI rather than the Helm SDK zero value. An explicit `0` disables history pruning. Negative values are invalid. Release notes MUST call out that an omitted value now bounds upgrade history and that users who require unlimited history must explicitly configure `0`.

### Wait Strategies

Atmos exposes the exact Helm 4 strategy values:

| Strategy   | Behavior                                                                    | Appropriate use                                                                  |
| ---------- | --------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| `hookOnly` | Waits for hook Pods and Jobs but not ordinary chart resources               | Backward-compatible default and fire-and-observe workflows                       |
| `watcher`  | Uses Helm 4's event-driven Kubernetes status watcher for ordinary resources | Recommended production readiness policy                                          |
| `legacy`   | Uses Helm 3-style readiness polling                                         | Compatibility for charts or clusters that do not work correctly with the watcher |

`wait.jobs` controls Jobs in the ordinary release manifest. Helm chart hook Jobs are already waited on when hooks are enabled. `jobs: true` requires an effective `watcher` or `legacy` strategy.

Helm 4 automatically promotes `hookOnly` to `watcher` when install uninstall-on-failure or upgrade rollback-on-failure is enabled. Atmos resolves and reports the effective strategy before invoking Helm rather than relying on an invisible SDK mutation.

## Precedence

Lifecycle resolution occurs in two configuration stages followed by invocation overrides:

1. Deep-merge the complete `release` tree using normal Atmos precedence, from lowest to highest: built-in Atmos defaults, stack-level native Helm defaults under `helm.release`, inherited abstract/base Helm components in inheritance order, and the concrete `components.helm.<name>.release` instance.
2. Select install, upgrade, or delete from release state and overlay that operation section on the merged release-wide defaults.
3. Apply explicitly supplied command-line flags last.

Only flags present on the command line override configuration. A Cobra Boolean default MUST NOT overwrite an explicit stack value. Boolean flags accept `=false` so incident-time input can enable or disable an inherited policy. `--on-failure` replaces the effective operation enum for that invocation and is validated after action selection.

Lifecycle fields are not added to `atmos.yaml` global `components.helm` configuration. That structure remains responsible for process-wide native Helm settings such as `base_path`, source behavior, and repositories. Release policy belongs to processed stack configuration where inheritance and per-environment overrides are available.

## Operation Applicability Is Schema-Enforced

The hierarchy replaces the former operation-applicability table. Install-only settings exist only in `release.install`; upgrade-only settings exist only in `release.upgrade`; delete settings exist only in `release.delete`. Atmos rejects misplaced and unknown keys instead of accepting and ignoring them.

Release-wide defaults intentionally contain only settings that can feed more than one operation. `release.wait.jobs` applies only when the selected install or upgrade action supports ordinary Job waiting; delete consumes only `release.wait.strategy`.

Lifecycle configuration is allowed on a component that also defines an external provision target, but it is applied only when the selected target kind is `kubernetes`. Explicit lifecycle flags combined with a non-Kubernetes target are an invocation error. Stored component defaults are ignored for the external target so one component can support both cluster and GitOps delivery. This bypass is reported in the normal execution summary.

## Effective Lifecycle Resolution

Atmos validates the merged tree before chart acquisition, then selects and resolves the operation before cluster mutation:

```text
processed component
        │
        ▼
deep-merge and validate release tree
        │
        ▼
inspect release state and select install / upgrade / delete
        │
        ├── overlay selected operation on release defaults
        ├── apply explicit CLI overrides
        ├── validate operation enum and cross-field constraints
        └── derive watcher when recovery requires it
        │
        ▼
canonical effectiveReleasePolicy
        │
        └── selected Helm action mapping
```

Structural validation failure occurs before repository setup or chart download. Validation that depends on the selected action, such as the `--on-failure` enum, occurs immediately after release-history lookup and before Kubernetes mutation.

## DAG Completion Contract

For a cluster-backed Helm node, scheduler completion means the selected Helm action has returned successfully under the effective lifecycle policy.

```text
terraform/base-infrastructure
            │
            ▼
helm/foundation-release
  install/upgrade + configured wait
            │
            ├── failure, timeout, or rollback ──▶ node failed; dependents blocked
            │
            └── success ───────────────────────▶ dependent nodes become ready
                                                  │
                                                  ▼
                                          helm/dependent-release
```

The following rules apply:

- A Helm node using `hookOnly` may complete before ordinary resources are ready. This is intentional and visible in its resolved policy.
- A Helm node using `watcher` or `legacy` completes only after Helm reports the selected resources ready.
- A release that fails and is successfully rolled back still returns failure to the scheduler.
- A rollback or uninstall failure preserves the original release failure and adds the recovery failure.
- Dependents never run after timeout, failed readiness, failed hooks, failed rollback, or cancellation.
- A future mixed-kind scheduler consumes the same provider result; it must not reinterpret Helm readiness.

## Timeout Semantics

The effective timeout is selected from `release.<operation>.timeout`, then `release.timeout`, then the built-in default. It is passed to the selected install, upgrade, hook, readiness, or delete action according to Helm 4 behavior. Helm's internal upgrade rollback inherits the effective upgrade timeout. This timeout is not a total wall-clock deadline for every step that precedes the action.

Specifically, `timeout` does not govern:

- Resolving or downloading an HTTP/OCI chart.
- Loading a chart from disk.
- Installing CRDs from a chart's `crds/` directory or waiting for the Kubernetes API to recognize them. Helm uses its own fixed 60-second CRD recognition wait before the release action timeout applies.
- Client-side template/diff rendering.
- Cloning, committing, or pushing an external delivery target.

Those operations require separate timeout fields and downloader/registry plumbing. The existing render timeout must not be described as reliably cancelling `LocateChart`, because Helm's chart location and registry paths do not consistently consume the `RunWithContext` context.

Cluster apply MUST stop wrapping the entire operation in the existing client-side render timeout. The caller context controls cancellation; Helm's action timeout controls Kubernetes operations.

## Context and Cancellation

Implementation MUST propagate one caller-owned context from the Cobra command or scheduler into component execution and then into Helm `RunWithContext` calls.

The current `component.ExecutionContext` does not carry a Go context, and Helm bulk execution and delivery create new background contexts. The implementation must add an equivalent context channel, preferably a `Context context.Context` field with a non-nil accessor that falls back to `context.Background()` for compatibility with existing providers and tests.

Required propagation includes:

- Direct command execution.
- Graph-backed bulk execution.
- Release-history lookup where supported by the SDK.
- Install and upgrade `RunWithContext`.
- Delete wait and hook phases through Helm uninstall wait options. Helm 4 does not accept a context for the uninstall request itself, so cancellation is checked before and after the action. A positive effective delete timeout bounds the wait; `0s` does not.
- External delivery and rendering call sites, without changing their timeout configuration in this PRD.

Scheduler cancellation or an operating-system signal must prevent new dependent nodes from starting and stop the caller from waiting for the active Helm action. Because Helm install and upgrade actions may continue work after `RunWithContext` returns, this PRD does not promise that caller cancellation terminates already-running SDK work. Atmos must prevent a new Atmos-managed rollback attempt from starting with a fresh background context after cancellation. Any stronger guarantee requires Atmos to own the action worker goroutine and wait for it to terminate before returning. Helm's built-in rollback-on-failure behavior remains responsible for its documented interrupted-release semantics.

## Dry-Run Semantics

Correct apply dry-run propagation is a release-blocking Phase 0 safety prerequisite. Lifecycle controls MUST NOT ship while a command presented as a dry run can mutate a cluster.

`atmos helm apply --dry-run` MUST reach `applyRelease` as a server-side Helm dry run and MUST NOT persist a release or mutate Kubernetes resources. The same resolved lifecycle is validated and reported, but rollback and cleanup cannot execute because no release mutation occurs.

`atmos helm delete --dry-run` MUST map to Helm 4 uninstall dry-run before R1 is complete so the release lifecycle is consistent across mutating commands.

Tests MUST exercise the complete command-to-provider path and prove that `ConfigAndStacksInfo.DryRun` reaches the selected cluster action. Action-helper tests alone are insufficient because the existing defect occurs between command parsing and provider invocation. Dry-run behavior is part of the acceptance criteria because the command already parses the flag; correcting its propagation is not a new user-facing feature.

## Failure and Recovery Semantics

R1 uses Helm 4's built-in `RollbackOnFailure` behavior for the two supported recovery enums:

- `release.install.on_failure: uninstall`: Helm uninstalls the newly created release after a failed install.
- `release.upgrade.on_failure: rollback`: Helm rolls back to the most recent successful release after a failed upgrade.
- `release.upgrade.cleanup_on_failure: true`: Helm removes resources newly created during a failed upgrade, whether or not rollback is enabled. When both are configured, Helm owns cleanup and recovery sequencing.
- Failure after successful recovery: Atmos returns a failed node with an error explaining that recovery completed.
- Failed recovery: Atmos returns both the original action failure and the rollback/uninstall failure using error wrapping or `errors.Join` without losing either cause.

With `on_failure: keep`, Atmos returns the install or upgrade failure without requesting Helm rollback or uninstall. Partial release state remains for the operator unless the independent upgrade cleanup Boolean applies.

Atmos does not attempt to collect Kubernetes diagnostics before recovery in this phase. Helm performs built-in recovery before returning control to Atmos, so implementing diagnostics or a separately configurable rollback later requires Atmos-managed recovery or an upstream Helm callback.

## Architecture

### Canonical Internal Type

The processed map first decodes into a hierarchical input. After action selection, Atmos resolves one flat effective policy for the selected Helm action:

```go
type releasePolicyInput struct {
  Timeout    optionalDuration
  ChartHooks optionalBool
  Wait       waitPolicyInput
  History    historyPolicyInput
  Install    installPolicyInput
  Upgrade    upgradePolicyInput
  Delete     deletePolicyInput
}

type effectiveReleasePolicy struct {
  Operation       releaseOperation
  Timeout         time.Duration
  ChartHooks      bool
  WaitStrategy    kube.WaitStrategy
  WaitForJobs     bool
  MaxHistory      int
  OnFailure       failurePolicy
  CleanupOnFailure bool
  CRDs            crdPolicy
  TimeoutExplicit bool
}
```

The operation-specific failure enums are closed strings. Input decoding MUST preserve omitted versus explicit values, including `false` and `timeout: 0s`. Resolution metadata supports migration warnings and observability but is not passed into Helm actions. `chartSpec` contains the resolved `effectiveReleasePolicy`; install, upgrade, and delete functions do not re-read raw component maps or flags.

### Stack Processing

The implementation must extend the existing native Helm field bag used by stack processing. Adding fields only to `chartSpec` or the JSON schema is insufficient: unrecognized Helm fields are currently omitted from base-component inheritance and the final component map.

Required processing changes include:

- Add the `release` tree to the recognized native Helm component field set.
- Deep-merge stack-level `helm.release` defaults into every native Helm component at the lowest component-specific precedence.
- Preserve nested base-component and concrete-component overrides without flattening operation sections.
- Keep lifecycle fields out of `settings`.
- Add processing tests covering type defaults, multi-level inheritance, partial nested overrides, explicit `false`, and concrete overrides.

### Schema Surfaces

Update all generated and source schema surfaces used by Atmos:

- The stack-level native `helm` defaults schema.
- `helm_component_manifest` in the stack manifest schema.
- Go schema or decoding types used to generate published schemas, where applicable.
- Website configuration reference and examples.

Every object under `release` uses `additionalProperties: false`. Enums and constraints must be schema-visible:

- `wait.strategy`: `watcher`, `hookOnly`, or `legacy`.
- `install.crds`: `create` or `skip`.
- `install.on_failure`: `uninstall` or `keep`.
- `upgrade.on_failure`: `rollback` or `keep`.
- Release-wide and operation-specific timeouts are duration strings; runtime validation supplies the authoritative parser.
- `history.max` is an integer greater than or equal to zero.

During the timeout migration release, schema generation and stack processing MUST NOT materialize an omitted release-wide or operation timeout as an explicit `0s`; omission must remain observable.

### Helm Action Mapping

| Effective policy         | Install action                | Upgrade action                | Uninstall action              |
| ------------------------ | ----------------------------- | ----------------------------- | ----------------------------- |
| `OnFailure == uninstall` | Set `RollbackOnFailure`       | —                             | —                             |
| `OnFailure == rollback`  | —                             | Set `RollbackOnFailure`       | —                             |
| `CleanupOnFailure`       | —                             | Set `CleanupOnFail`           | —                             |
| `WaitStrategy`           | Set                           | Set                           | Set                           |
| `WaitForJobs`            | Set                           | Set                           | —                             |
| `Timeout`                | Set                           | Set                           | Set                           |
| `MaxHistory`             | —                             | Set                           | —                             |
| `ChartHooks`             | Set inverse on `DisableHooks` | Set inverse on `DisableHooks` | Set inverse on `DisableHooks` |
| `CRDs == skip`           | Set `SkipCRDs`                | —                             | —                             |

Dry-run is execution intent, not release policy, and therefore remains outside `effectiveReleasePolicy`. The command-to-provider path MUST propagate it independently to `Install.DryRunStrategy` or `Upgrade.DryRunStrategy` for apply/deploy and `Uninstall.DryRun` for delete. Apply and deploy use Helm's server-side dry-run strategy so validation reaches the cluster without persisting a release.

| Atmos operation                  | Provider operation | Helm timeout and recovery behavior                                                                                                                                        |
| -------------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| apply/deploy, no release history | Install            | `Install.Timeout`; on failure, `RollbackOnFailure` performs Helm's internal uninstall recovery using the same action configuration.                                       |
| apply/deploy, existing release   | Upgrade            | `Upgrade.Timeout`; on failure, `RollbackOnFailure` performs Helm's internal rollback, with `CleanupOnFail` applied when configured.                                       |
| Helm internal upgrade recovery   | Rollback           | Helm propagates the upgrade timeout and wait configuration into its internal rollback action; Atmos returns the original operation as failed even when recovery succeeds. |
| delete                           | Uninstall          | `Uninstall.Timeout`; no automatic recovery action follows an uninstall failure.                                                                                           |

Action mapping should live in small, unit-testable helpers. The scheduler and command packages must not import Helm SDK action types merely to configure lifecycle policy.

### Command Flags

Use the standard Atmos parser and command registry. Mirror Helm names when the meaning is identical:

| Atmos flag                    | Configuration field                | Commands              |
| ----------------------------- | ---------------------------------- | --------------------- |
| `--on-failure=<action>`       | Selected operation `on_failure`    | apply, deploy         |
| `--cleanup-on-failure[=bool]` | `upgrade.cleanup_on_failure`       | apply, deploy         |
| `--wait[=strategy]`           | Effective `wait.strategy`          | apply, deploy, delete |
| `--wait-for-jobs[=bool]`      | Effective `wait.jobs`              | apply, deploy         |
| `--timeout`                   | Effective operation timeout        | apply, deploy, delete |
| `--history-max`               | `history.max`                      | apply, deploy         |
| `--no-hooks[=bool]`           | Inverse of effective `chart_hooks` | apply, deploy, delete |
| `--skip-crds[=bool]`          | `install.crds`                     | apply, deploy         |

`--on-failure` is a single enum whose valid values depend on the selected action. An explicit CLI value replaces configuration for that invocation. Operation-specific flags are validated after action selection and fail if the selected action cannot honor them: `--cleanup-on-failure` requires upgrade, `--skip-crds` requires install, and `--history-max` requires upgrade. Flags are represented in execution summaries using canonical positive field names. Alias warnings use Atmos UI/logging primitives, not direct standard-output writes.

## Observability

The Helm CI/job summary should include a non-secret lifecycle block for cluster-backed operations:

```yaml
release:
  operation: upgrade
  timeout: 1h0m0s
  chart_hooks: true
  wait:
    strategy: watcher
    jobs: true
  history:
    max: 10
  on_failure: rollback
  cleanup_on_failure: true
```

The summary reports only the effective selected-operation values after hierarchy resolution and CLI overrides.

For an external provision target, the normal execution summary reports the bypass without presenting stored lifecycle values as active policy:

```yaml
release:
  applied: false
  target_kind: git
  reason: external_target
```

This summary is emitted at the normal CI/job-summary level. A warning is unnecessary for stored defaults because dual cluster/external components are supported intentionally; explicit lifecycle flags on an external target remain an error.

At debug level, Atmos logs:

- Whether the action selected install or upgrade.
- The effective wait strategy and why it changed.
- Which release-wide and operation-specific paths supplied each effective override.
- Which failure policy was requested.
- Additional diagnostic detail when the selected target bypasses release lifecycle behavior.

Errors must use static Atmos sentinel errors where callers need classification and wrap the underlying Helm or Kubernetes cause.

## Backward Compatibility

Native Helm remains experimental. Existing manifests that omit `release` remain structurally valid, while earlier flat lifecycle proposals are not retained as aliases because they never shipped.

- An omitted `release` tree retains `hookOnly`, `on_failure: keep`, no ordinary Job waiting, chart hooks enabled, and install CRD creation.
- During v1.225.x, an omitted effective timeout preserves the current SDK zero value and emits a migration warning once per selected component per invocation. Starting with v1.226.0, the default becomes five minutes and the warning is removed.
- An explicit `release.timeout: 0s` or operation-specific `timeout: 0s` preserves unbounded behavior without a migration warning.
- An omitted `release.history.max` changes from the SDK zero value to `10`; an explicit `0` preserves unlimited history.
- Direct single-component commands and bulk commands resolve the same lifecycle configuration.
- External GitOps delivery remains render-and-deliver and does not acquire release semantics.
- Existing Atmos `hooks:` and `--skip-hooks` behavior is unchanged.

The release notes must call out the timeout migration schedule and the new ten-revision history limit. A previously hanging hook may fail after the timeout transition, a workload that legitimately needs more time must configure a larger release-wide or operation timeout, and a release requiring unlimited history must configure `release.history.max: 0`.

## Security and Safety

- Lifecycle summaries contain no rendered values, credentials, Kubernetes Secrets, or logs.
- Rollback does not change secret masking behavior.
- `release.chart_hooks: false` must not disable Atmos policy or security hooks.
- Validation runs before chart retrieval and cluster mutation.
- Explicit lifecycle flags on an external target fail rather than creating a false impression that rollback or waiting occurred.
- Timeout and cancellation errors preserve enough context to identify the component, stack, release, namespace, operation, and effective wait strategy.

## Testing Strategy

### Unit Tests

- Decode every release-wide and operation-specific field from a processed Helm component.
- Verify built-in defaults, including `release.history.max: 10` and the staged timeout default.
- Verify the v1.225.x phase: an omitted effective timeout remains unbounded and warns once per selected component per invocation, while explicit release-wide and operation-specific `0s` values remain unbounded without warning.
- Verify the v1.226.0 phase: an omitted timeout resolves to `5m` without a migration warning.
- Verify stack-level `helm.release` defaults, abstract inheritance, concrete partial-tree overrides, and explicit `false` values.
- Verify the full release tree merges before the selected operation overlays release-wide defaults.
- Verify `release.install.timeout: 60m`, `release.upgrade.timeout: 10m`, and `release.delete.timeout: 5m` resolve independently for the same component.
- Verify explicit CLI flags override both release-wide and selected-operation configuration.
- Verify `--on-failure` accepts only `uninstall | keep` for install and `rollback | keep` for upgrade.
- Reject unknown fields at every release object and reject `release.rollback` in R1.
- Validate all wait strategies and reject unknown values.
- Validate negative timeout and history values.
- Validate effective `wait.jobs` against the effective strategy.
- Verify install uninstall-on-failure and upgrade rollback-on-failure promote `hookOnly` to `watcher` before action mapping.
- Verify every install, upgrade, and delete action field mapping.
- Verify only explicitly changed flags override stack configuration.
- Verify explicit `=false` command-line values override stack-level `true` for every lifecycle Boolean flag.
- Verify explicit operation-specific flags fail when the selected action cannot honor them.
- Verify explicit lifecycle flags fail for external provision targets.
- Verify apply and delete dry-run propagation through the complete command-to-provider path.
- Verify caller-context propagation and cancellation.
- Verify canonical lifecycle fields in summaries.

### Stack Processing Tests

- Stack-level `helm.release` defaults reach every concrete Helm component.
- Base-component release trees inherit through multiple levels.
- Concrete nested values override inherited leaves without replacing sibling operation sections.
- Operation sections overlay release-wide defaults only after normal stack merging.
- Timeout presence survives stack processing at both levels so omitted and explicit `0s` remain distinguishable.
- Lifecycle fields survive `describe component` and `describe stacks` processing.
- JSON schema accepts valid trees and rejects misplaced fields, unknown keys, invalid operation failure enums, unknown wait strategies, or negative history values.

### Helm SDK Tests

Use the existing in-memory release lifecycle tests to cover:

- First install and subsequent upgrade with lifecycle mapping.
- Dry-run install and upgrade without persisted history.
- Default history pruning at ten revisions, a positive `release.history.max` override, and explicit unlimited `0`.
- Chart-hook suppression on install and upgrade.
- CRD `create` and `skip` policies on install.
- Upgrade cleanup with rollback both enabled and disabled.
- Separate install, upgrade, and delete timeout mappings for one component.
- Delete timeout, wait strategy, chart-hook suppression, and dry run.

### k3s Integration Tests

Add a small deterministic chart fixture with ordinary resources and weighted hook Jobs:

1. A pre-install/pre-upgrade hook at weight `-2` that creates a ConfigMap marked `helm.sh/resource-policy: keep`.
2. A hook Job at weight `-1` whose success requires the ConfigMap, proving lower-weight hooks complete first.
3. A failing hook Job.
4. A slow ordinary Job used to verify `wait.jobs`.
5. A Deployment whose readiness can be delayed without pulling a large image.
6. A CRD used to verify `install.crds`.

Required scenarios:

- `hookOnly` returns without waiting for the delayed Deployment.
- `watcher` waits for Deployment readiness.
- `wait.jobs` waits for the ordinary Job.
- Install, upgrade, and delete timeouts fail within bounded tolerances and use their independent effective values.
- Failed first install with `on_failure: uninstall` leaves no deployed release.
- Failed upgrade with rollback restores the previous successful release.
- Failed upgrade cleanup removes newly created resources.
- CRD installation uses Helm's separate recognition wait and is not shortened by the release `timeout` value.
- Weighted hooks execute in ascending weight order on first install and upgrade.
- Rollback and failed-install cleanup preserve resources marked `helm.sh/resource-policy: keep`.
- `chart_hooks: false` prevents the fixture hooks from executing.
- `install.crds: skip` does not install the fixture CRD.
- A two-component Helm DAG does not start the dependent until the prerequisite's configured readiness succeeds.
- A failed or rolled-back prerequisite blocks the dependent.

Integration tests must use short durations, polling with bounded retries, unique namespaces, and cleanup registered with the test framework. They must not use fixed sleeps as the primary assertion mechanism.

## Implementation Plan

The phases below are independently mergeable implementation slices, not a requirement to serialize every pull request. The context/cancellation slice has the largest cross-provider regression surface and MUST NOT block the canonical model, stack processing, command flags, dry-run safety fix, or Helm action mapping. It remains required before R1 is declared complete.

### Phase 0: Apply Dry-Run Safety Prerequisite

1. Propagate `ConfigAndStacksInfo.DryRun` through cluster delivery into `applyRelease` instead of passing a literal `false`.
2. Add a command-to-provider regression test proving that apply dry-run does not persist release history or mutate Kubernetes resources.
3. Land this fix before enabling or releasing any lifecycle configuration or flags.

### Phase 1: Canonical Model and Validation

1. Add static errors for lifecycle decoding and validation.
2. Add presence-aware release-tree decoding and selected-operation `effectiveReleasePolicy` resolution.
3. Add operation failure enums, wait strategy, staged timeout, history default, deep-merge, and cross-field tests.

### Phase 2: Stack Processing and Schemas

1. Add the nested release tree to native Helm stack processing.
2. Add stack-level `helm.release` defaults, deep inheritance, and operation-overlay tests.
3. Update stack schemas and generated schema artifacts.

### Phase 3: Command and Helm Action Mapping

1. Register lifecycle flags on applicable commands.
2. Configure install, upgrade, and uninstall actions from the canonical effective release policy.
3. Propagate delete dry-run and add its command-to-provider regression test.
4. Reject explicit lifecycle flags for external targets and report stored-policy bypass in normal summaries.
5. Preserve Helm failure and recovery errors.
6. Add effective lifecycle data to debug logs and CI summaries.

### Phase 4: Context and Cancellation Workstream

1. Add a backward-compatible caller context accessor to component execution.
2. Remove Helm background-context substitutions from direct and graph-backed execution.
3. Verify cancellation in Helm and add provider-level regression tests for other registered component types.
4. Merge this workstream independently when ready; do not make it a prerequisite for Phases 1 through 3.

### Phase 5: Integration and Documentation

1. Add deterministic k3s lifecycle fixtures and tests.
2. Update native Helm component and command documentation.
3. Add migration notes for the staged five-minute timeout and ten-revision history default.
4. Document the DAG completion contract.

## Risks and Mitigations

| Risk                                                                     | Impact                                                                 | Mitigation                                                                                                                      |
| ------------------------------------------------------------------------ | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Apply dry-run remains disconnected from cluster delivery                 | A command presented as non-mutating changes release or cluster state   | Phase 0 prerequisite and command-to-provider mutation regression test                                                           |
| Five-minute default breaks long-running existing releases                | Apply begins failing where it previously waited indefinitely           | One-minor warning window; explicit effective `timeout: 0s`; actionable timeout errors; per-operation examples for slow installs |
| Ten-revision history default prunes older release records                | Users lose rollback history they expected Atmos to retain indefinitely | Prominent release note; explicit `release.history.max: 0`; pruning tests against the Helm CLI-compatible default                |
| `wait.jobs` appears effective under `hookOnly`                           | Users believe ordinary Jobs are gated when they are not                | Validate against the effective strategy and explain hook Jobs separately                                                        |
| Rollback is mistaken for success                                         | Dependents run after the desired version failed                        | Always return node failure after recovery                                                                                       |
| `release.chart_hooks` is confused with Atmos hooks                       | Policy hooks are accidentally assumed disabled                         | Keep the field inside `release`; retain `--skip-hooks` for Atmos hooks                                                          |
| A published rollback section appears to control Helm's internal rollback | Operators trust a timeout that the SDK path ignores                    | Omit `release.rollback` until Atmos owns rollback as an explicit action                                                         |
| Lifecycle flags appear to affect GitOps delivery                         | Users assume an external controller waited or rolled back              | Reject explicit flags for external targets and report `release.applied: false` in the normal summary                            |
| Context refactor affects other component providers                       | Cancellation regression outside Helm                                   | Add a backward-compatible context accessor and provider-level tests                                                             |
| Helm SDK behavior changes in later 4.x releases                          | Semantics drift from documentation                                     | Keep canonical mapping isolated and run lifecycle tests against dependency upgrades                                             |
| R4 diagnostics later require custom rollback orchestration               | Duplicate lifecycle configuration or incompatible behavior             | Keep public contract independent of whether Helm or Atmos performs recovery                                                     |

## Success Criteria

- The release tree resolves through type defaults, inheritance, concrete components, selected-operation overlays, and highest-priority explicit CLI overrides.
- `atmos helm apply --dry-run` and `delete --dry-run` do not mutate release state.
- An omitted timeout remains unbounded with one warning per selected component per invocation in v1.225.x and resolves to `5m` without that warning from v1.226.0, while explicit `timeout: 0s` remains unbounded without a warning in both phases.
- An omitted `release.history.max` prunes to ten revisions and explicit `0` remains unlimited.
- One component can use independent install, upgrade, and delete timeouts without changing chart rendering or values.
- Watcher readiness prevents dependent DAG nodes from starting before the prerequisite release is ready.
- Failed or rolled-back releases block dependents and return actionable errors.
- Install, upgrade, and delete actions receive only their applicable Helm SDK fields.
- Cancellation propagates from direct and bulk commands into active Helm actions.
- k3s tests prove successful waits, timeouts, rollback, cleanup, weighted hook ordering, retained-resource behavior, hook suppression, CRD skipping, and DAG gating.
- Native Helm documentation distinguishes Helm chart hooks, Atmos lifecycle hooks, native SDK capabilities, and external GitOps delivery.

## Future Work

1. Configurable chart acquisition, client-side render, and external delivery timeouts.
2. Deployed-baseline and external-artifact lifecycle semantics for Helm chart hooks beyond the template and diff visibility required by this PRD.
3. Pre-rollback Pod state, Warning Event, and bounded container-log diagnostics.
4. Atmos-managed rollback with its own `release.rollback` section, context, timeout, wait, reporting, and failure behavior.
5. Additional Helm 4 release controls after production feedback, including uninstall `keep_history` and cascade behavior.
6. Mixed-kind DAG execution using the same Helm completion contract.
