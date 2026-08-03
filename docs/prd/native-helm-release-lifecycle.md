# PRD: Native Helm Release Lifecycle

**Status:** Draft

**Last Updated:** 2026-08-03

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

| Capability | Current native Helm behavior | Consequence |
| --- | --- | --- |
| Install or upgrade selection | Atmos checks release history and calls separate Helm SDK install or upgrade actions | Install-only and upgrade-only fields must be applied deliberately |
| Wait strategy | Hard-coded `hookOnly` | Ordinary release resources are not readiness gates |
| Release timeout | Helm SDK zero value | No intentional Atmos release-timeout contract |
| Rollback on failure | Disabled | Failed installs/upgrades can leave partial state |
| Wait for Jobs | Disabled | Jobs in the ordinary manifest are not readiness gates |
| Upgrade cleanup | Disabled | Newly created resources can remain after a failed upgrade |
| History limit | Helm SDK zero value | Release history is not bounded by Atmos |
| Chart hooks | Enabled | No component-level control; distinct from Atmos lifecycle hooks |
| CRD installation | Enabled for install | No component-level control |
| Apply dry-run | Parsed by the command, but cluster delivery currently passes `false` to the release client | `atmos helm apply --dry-run` can mutate the cluster |
| Cancellation | Several Helm paths replace the caller context with `context.Background()` | Signals and scheduler cancellation do not propagate consistently |

## Goals

- Expose the Helm 4 release lifecycle controls required for production install and upgrade workflows.
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
- Automatically running `helm dependency build` after chart provisioning. Atmos MUST report missing dependencies with the explicit build command, while dependency acquisition remains a caller-controlled step to avoid hidden network access and chart-directory mutation.
- Adding `--set-file`; Atmos `!include.raw` already covers byte-preserving file-backed value content, including a source file's terminal newline. Content normalization is separate from Helm lifecycle behavior.
- Supporting Helm CLI subcommand, getter, downloader, post-renderer, or Wasm plugins in native SDK operations.
- Adding force replacement, server-side apply selection, dependency update, value reuse/reset, ownership takeover, validation bypass, or uninstall history/cascade options. In particular, Helm `Uninstall.KeepHistory`, the `--keep-history` flag, and a `keep_history` component field are deferred from R1.
- Changing dependency selection or scheduler algorithms.
- Treating a successful rollback as a successful DAG node.

## Design Principles

### Component-Owned Inputs Are Direct Component Fields

Lifecycle configuration belongs to the Helm component, not to generic `settings`. The design MUST NOT introduce `settings.helm`. This follows the Atmos component convention that provider-owned inputs live directly under `components.<type>.<name>`.

```yaml
components:
  helm:
    demo-api:
      chart: oci://registry.invalid/charts/demo-api
      version: 1.2.3
      namespace: demo

      on_failure: [rollback, cleanup]
      wait_strategy: watcher
      wait_for_jobs: true
      timeout: 60m
      max_history: 10
      disable_chart_hooks: false
      skip_crds: false
```

`disable_chart_hooks` deliberately names Helm chart hooks. Atmos components already have a `hooks:` section and a global `--skip-hooks` behavior for Atmos lifecycle hooks. Calling the new field only `disable_hooks` would make these unrelated mechanisms ambiguous.

### Failure Policy Is Extensible

Atmos exposes failure behavior as one ordered-set field, `on_failure`, rather than separate Booleans. The initial actions are `rollback` and `cleanup`; adding a future action does not require another top-level component field or flag. The list order does not control execution order because Helm owns recovery sequencing. Atmos removes duplicates and reports actions in canonical order.

The equivalent invocation flag is `--on-failure=rollback,cleanup`. Because native Helm lifecycle configuration is still experimental and none of the earlier proposed names shipped, R1 does not introduce `rollback_on_failure`, `cleanup_on_fail`, `atomic`, or their CLI forms as compatibility aliases.

Canonical terminology applies where it does not collide with an existing Atmos concept. The `disable_chart_hooks` field intentionally remains more explicit than Helm's `DisableHooks` SDK field and `--no-hooks` flag because Atmos lifecycle hooks are a separate mechanism. R1 does not add a `disable_hooks` alias.

### Configuration Describes Policy; Flags Override an Invocation

Stack configuration defines the release's normal lifecycle policy. Command flags provide an explicit one-run override for CI, emergency operation, or migration from Helm CLI scripts.

## User Experience

### Type Defaults and Component Overrides

Stack-level `helm` fields provide defaults for every native Helm component in the processed stack:

```yaml
helm:
  wait_strategy: watcher
  timeout: 10m
  on_failure: [rollback]
  max_history: 10

components:
  helm:
    foundation-release:
      chart: charts/foundation-release
      namespace: example-system
      timeout: 20m

    dependent-release:
      chart: oci://registry.invalid/charts/dependent-release
      namespace: example-apps
      timeout: 60m
      wait_for_jobs: true
      dependencies:
        components:
          - name: foundation-release
```

After normal stack processing, `foundation-release` uses a 20-minute timeout and `dependent-release` uses a 60-minute timeout. Both inherit watcher readiness, the `rollback` failure action, and a ten-revision history limit.

Abstract Helm components can also define lifecycle fields:

```yaml
components:
  helm:
    base-release-policy:
      metadata:
        type: abstract
      wait_strategy: watcher
      on_failure: [rollback, cleanup]
      max_history: 10

    demo-release:
      metadata:
        inherits:
          - base-release-policy
      chart: charts/demo-release
      namespace: demo
      timeout: 30m
```

Lifecycle fields follow normal Atmos override precedence. `on_failure` follows the configured Atmos list merge strategy: the default `replace` strategy lets a concrete component replace or clear an inherited policy with `[]`, while `append` can add actions across inheritance layers. Atmos normalizes duplicates after stack processing.

### Command-Line Overrides

The cluster-backed `apply` and `deploy` commands support:

```shell
atmos helm apply demo-api -s example-prod \
  --on-failure=rollback,cleanup \
  --wait=watcher \
  --wait-for-jobs \
  --timeout=60m \
  --history-max=10
```

For Helm 4 parity, `--wait` accepts an optional strategy:

```shell
--wait            # watcher
--wait=watcher
--wait=hookOnly
--wait=legacy
```

The deprecated boolean forms `--wait=true` and `--wait=false` are accepted with a warning and normalize to `watcher` and `hookOnly`, respectively.

The `delete` command supports the lifecycle subset relevant to uninstall:

```shell
atmos helm delete demo-api -s example-prod \
  --wait=watcher \
  --timeout=10m \
  --no-hooks
```

`template`, `diff`, and `plan` do not register release-lifecycle flags because they do not perform a release operation.

## Configuration Contract

| Field | Type | Default | Default status | Helm 4 mapping | Applies to |
| --- | --- | --- | --- | --- | --- |
| `on_failure` | List of enums: `rollback`, `cleanup` | `[]` | Preserves current Atmos behavior | `rollback` sets `Install.RollbackOnFailure` and `Upgrade.RollbackOnFailure`; `cleanup` sets `Upgrade.CleanupOnFail` | Install, upgrade |
| `wait_strategy` | Enum | `hookOnly` | Preserves current Atmos behavior | `kube.WaitStrategy` | Install, upgrade, delete |
| `wait` | Boolean | Unset | Convenience alias | `true` = `watcher`, `false` = `hookOnly` | Install, upgrade, delete |
| `wait_for_jobs` | Boolean | `false` | Preserves current Atmos behavior | `Install.WaitForJobs`, `Upgrade.WaitForJobs` | Install, upgrade |
| `timeout` | Duration string | `0s` in v1.225.x; `5m` from v1.226.0 | Staged change to the Helm CLI default | Helm action `Timeout` | Install, upgrade, delete |
| `max_history` | Non-negative integer | `10` | New Helm CLI-compatible limit | `Upgrade.MaxHistory` | Upgrade only |
| `disable_chart_hooks` | Boolean | `false` | Preserves current Atmos behavior | Helm action `DisableHooks` | Install, upgrade, delete |
| `skip_crds` | Boolean | `false` | Preserves current Atmos behavior | `Install.SkipCRDs` | Install only |

The target timeout default is five minutes, matching the Helm CLI. R1 introduces that default through one explicit minor-release migration window instead of silently changing existing executions:

1. Atmos v1.225.x is the migration release line. An omitted `timeout` preserves the current SDK zero value and emits one warning per selected Helm component per invocation, including direct and bulk operations, that the default will become `5m` in v1.226.0.
2. An explicit `timeout`, including `timeout: 0s`, suppresses the migration warning.
3. Starting with Atmos v1.226.0, an omitted `timeout` resolves to `5m` and the migration warning is removed.

An explicit `timeout: 0s` disables the Helm action timeout. Negative durations are invalid. Documentation MUST warn that a zero timeout can leave hook or resource waits unbounded. This is especially important for delete: Helm 4's uninstall request does not accept the caller context, so `timeout: 0s` can leave an uninstall wait unbounded after the caller stops waiting. Timeout errors MUST include the effective duration and identify the `timeout` component field so users can correct long-running releases without inspecting debug logs.

The `max_history` default is `10`, matching the Helm CLI rather than the Helm SDK zero value. An explicit `max_history: 0` disables history pruning. Negative values are invalid. Release notes MUST call out that an omitted value now bounds upgrade history and that users who require unlimited history must explicitly configure `0`.

### Wait Strategies

Atmos exposes the exact Helm 4 strategy values:

| Strategy | Behavior | Appropriate use |
| --- | --- | --- |
| `hookOnly` | Waits for hook Pods and Jobs but not ordinary chart resources | Backward-compatible default and fire-and-observe workflows |
| `watcher` | Uses Helm 4's event-driven Kubernetes status watcher for ordinary resources | Recommended production readiness policy |
| `legacy` | Uses Helm 3-style readiness polling | Compatibility for charts or clusters that do not work correctly with the watcher |

`wait_for_jobs` controls Jobs in the ordinary release manifest. Helm chart hook Jobs are already waited on when hooks are enabled. `wait_for_jobs: true` requires an effective `watcher` or `legacy` strategy.

Helm 4 automatically promotes `hookOnly` to `watcher` when `on_failure` contains `rollback`. Atmos resolves and reports the effective strategy before invoking Helm, rather than relying on an invisible SDK mutation. Therefore this is valid:

```yaml
on_failure: [rollback]
wait_for_jobs: true
```

Its effective wait strategy is `watcher`.

### List and Alias Resolution

`on_failure` is decoded after normal stack merging. Values must be strings, unknown values are rejected, and duplicates are removed. Canonical output order is `rollback`, then `cleanup`, regardless of input or inheritance order. An empty list is valid and requests no recovery action.

For example, the default `replace` list strategy lets a concrete component disable an inherited safety policy:

```yaml
components:
  helm:
    base-release-policy:
      metadata:
        type: abstract
      on_failure: [rollback, cleanup]

    demo-release:
      metadata:
        inherits:
          - base-release-policy
      chart: charts/demo-release
      namespace: demo
      on_failure: []
```

The effective `on_failure` value for `demo-release` is `[]`. Stack-processing tests MUST cover replacement, clearing, append-mode accumulation, and duplicate normalization.

`wait_strategy` remains canonical over the existing `wait` convenience alias. If both are present, Atmos uses `wait_strategy` and emits one warning that `wait` was ignored.

Resolved summaries and logs use only canonical names.

## Precedence

Lifecycle values resolve from lowest to highest priority:

1. Built-in Atmos defaults.
2. Stack-level native Helm defaults under `helm`.
3. Inherited abstract/base Helm components in inheritance order.
4. The concrete `components.helm.<name>` instance.
5. Explicit command-line flags.

Only flags explicitly present on the command line override component configuration. `--on-failure` replaces the resolved component list for that invocation; `--on-failure=` supplies an empty list and disables inherited actions. A Cobra Boolean default of `false` MUST NOT overwrite `true` from the stack. Every lifecycle Boolean flag accepts an explicit `=false` value, such as `--wait-for-jobs=false`, `--no-hooks=false`, and `--skip-crds=false`, so command-line input can disable a stack-level `true` value. Bare Boolean flags mean `true`.

Lifecycle fields are not added to `atmos.yaml` global `components.helm` configuration. That structure remains responsible for process-wide native Helm settings such as `base_path`, source behavior, repositories, and plugin management. Release policy belongs to processed stack configuration where inheritance and per-environment overrides are available.

## Operation Applicability

| Setting | New install | Existing release upgrade | Delete | Template/diff | External provision target |
| --- | --- | --- | --- | --- | --- |
| `on_failure: [rollback]` | Uninstall failed release | Roll back to last successful release | Not applicable | Not applicable | Not applicable |
| `on_failure: [cleanup]` | Ignored | Remove resources newly created by the failed upgrade | Not applicable | Not applicable | Not applicable |
| `wait_strategy` | Applied | Applied | Applied | Not applicable | Not applicable |
| `wait_for_jobs` | Applied | Applied | Not applicable | Not applicable | Not applicable |
| `timeout` | Applied | Applied | Applied | Not applicable | Not applicable |
| `max_history` | Ignored | Applied | Not applicable | Not applicable | Not applicable |
| `disable_chart_hooks` | Applied | Applied | Applied | Not part of this PRD | Not applicable |
| `skip_crds` | Applied | Ignored | Not applicable | Not applicable | Not applicable |

Install-only and upgrade-only fields are valid in persistent component configuration because the same `apply` command transitions from first install to later upgrades. Atmos MUST NOT fail a first install because `on_failure` contains `cleanup` or `max_history` is configured, and MUST NOT fail an upgrade because `skip_crds` remains configured. Inapplicable fields are omitted from the action and visible only at debug level.

Lifecycle configuration is allowed on a component that also defines an external provision target, but it is applied only when the selected target kind is `kubernetes`. Explicit lifecycle flags combined with a non-Kubernetes target are an invocation error because the user requested behavior that cannot occur. Stored component defaults are ignored for the external target so one component can support both cluster and GitOps delivery. This bypass is reported in the normal execution summary, not only in debug logs.

## Effective Lifecycle Resolution

Atmos resolves the lifecycle before chart acquisition or cluster access:

```text
processed component
        │
        ▼
extract lifecycle input
        │
        ├── normalize on_failure actions and the wait alias
        ├── apply explicit CLI overrides
        ├── validate enum, duration, and integer values
        ├── derive watcher when rollback requires it
        └── validate cross-field constraints
        │
        ▼
canonical releaseLifecycle
        │
        ├── install mapping
        ├── upgrade mapping
        └── delete mapping
```

Validation failure occurs before repository setup, chart download, release history access, or Kubernetes mutation.

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

`timeout` is a Helm release-operation timeout. It is passed to install, upgrade, rollback, hook execution, readiness waiting, and delete according to Helm 4 behavior. It is not a total wall-clock deadline for every step that precedes the action.

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
- Delete wait and hook phases through Helm uninstall wait options. Helm 4 does not accept a context for the uninstall request itself, so cancellation is checked before and after the action. A positive configured timeout bounds the wait; `timeout: 0s` does not.
- External delivery and rendering call sites, without changing their timeout configuration in this PRD.

Scheduler cancellation or an operating-system signal must prevent new dependent nodes from starting and stop the caller from waiting for the active Helm action. Because Helm install and upgrade actions may continue work after `RunWithContext` returns, this PRD does not promise that caller cancellation terminates already-running SDK work. Atmos must prevent a new Atmos-managed rollback attempt from starting with a fresh background context after cancellation. Any stronger guarantee requires Atmos to own the action worker goroutine and wait for it to terminate before returning. Helm's built-in rollback-on-failure behavior remains responsible for its documented interrupted-release semantics.

## Dry-Run Semantics

Correct apply dry-run propagation is a release-blocking Phase 0 safety prerequisite. Lifecycle controls MUST NOT ship while a command presented as a dry run can mutate a cluster.

`atmos helm apply --dry-run` MUST reach `applyRelease` as a server-side Helm dry run and MUST NOT persist a release or mutate Kubernetes resources. The same resolved lifecycle is validated and reported, but rollback and cleanup cannot execute because no release mutation occurs.

`atmos helm delete --dry-run` MUST map to Helm 4 uninstall dry-run before R1 is complete so the release lifecycle is consistent across mutating commands.

Tests MUST exercise the complete command-to-provider path and prove that `ConfigAndStacksInfo.DryRun` reaches the selected cluster action. Action-helper tests alone are insufficient because the existing defect occurs between command parsing and provider invocation. Dry-run behavior is part of the acceptance criteria because the command already parses the flag; correcting its propagation is not a new user-facing feature.

## Failure and Recovery Semantics

When `on_failure` contains `rollback`, R1 uses Helm 4's built-in `RollbackOnFailure` behavior:

- Failed install with rollback enabled: Helm uninstalls the newly created release.
- Failed upgrade with rollback enabled: Helm rolls back to the most recent successful release.
- `cleanup`: Helm removes resources newly created during a failed upgrade whenever the action is present, whether or not `rollback` is also present. When both are present, Helm owns the cleanup and recovery sequencing.
- Failure after successful recovery: Atmos returns a failed node with an error explaining that recovery completed.
- Failed recovery: Atmos returns both the original action failure and the rollback/uninstall failure using error wrapping or `errors.Join` without losing either cause.

When `on_failure` does not contain `rollback`, Atmos returns the install or upgrade failure without requesting Helm rollback or uninstall. Any partial release state is left for the operator unless the independent upgrade-only `cleanup` action applies.

Atmos does not attempt to collect Kubernetes diagnostics before recovery in this phase. Helm performs built-in recovery before returning control to Atmos, so implementing diagnostics later may require Atmos-managed recovery or an upstream Helm callback. The public `on_failure` contract can add a future diagnostics action without another top-level field.

## Architecture

### Canonical Internal Type

The processed map and flag inputs normalize into one internal value before action selection:

```go
type releaseLifecycle struct {
  OnFailure        []failureAction
  WaitStrategy      kube.WaitStrategy
  WaitForJobs       bool
  Timeout           time.Duration
  MaxHistory        int
  DisableChartHooks bool
  SkipCRDs          bool
}

type releaseLifecycleResolution struct {
  Policy          releaseLifecycle
  TimeoutExplicit bool
}
```

`failureAction` is a closed string enum in R1 with `rollback` and `cleanup`. Input decoding MUST distinguish an omitted list from an explicitly empty list when applying command-line overrides, distinguish omitted Boolean flags from explicit `false`, and distinguish an omitted timeout from explicit `timeout: 0s` during the migration window. Resolution metadata is used for migration warnings and observability but is not passed into Helm actions. `chartSpec` contains the canonical `releaseLifecycle`; install, upgrade, and delete functions do not re-read raw component maps or flags.

### Stack Processing

The implementation must extend the existing native Helm field bag used by stack processing. Adding fields only to `chartSpec` or the JSON schema is insufficient: unrecognized Helm fields are currently omitted from base-component inheritance and the final component map.

Required processing changes include:

- Add lifecycle keys to the recognized native Helm component field set.
- Merge stack-level `helm` lifecycle defaults into every native Helm component at the lowest component-specific precedence.
- Preserve base-component and concrete-component overrides, including the configured list merge strategy for `on_failure`.
- Keep lifecycle fields out of `settings`.
- Add processing tests covering type defaults, multi-level inheritance, explicit `false`, list replacement and clearing, append-mode accumulation, duplicate normalization, and concrete overrides.

### Schema Surfaces

Update all generated and source schema surfaces used by Atmos:

- The stack-level native `helm` defaults schema.
- `helm_component_manifest` in the stack manifest schema.
- Go schema or decoding types used to generate published schemas, where applicable.
- Website configuration reference and examples.

Enums and constraints must be schema-visible:

- `on_failure`: an array whose items are `rollback` or `cleanup`.
- `wait_strategy`: `watcher`, `hookOnly`, or `legacy`.
- `timeout`: duration string; runtime validation supplies the authoritative parser.
- `max_history`: integer greater than or equal to zero.

During the timeout migration release, schema generation and stack processing MUST NOT materialize an omitted timeout as an explicit `0s`; omission must remain observable so Atmos can emit the migration warning while allowing explicit `timeout: 0s` without warning.

### Helm Action Mapping

| Canonical policy | Install action | Upgrade action | Uninstall action |
| --- | --- | --- | --- |
| `on_failure` contains `rollback` | Set `RollbackOnFailure` | Set `RollbackOnFailure` | — |
| `on_failure` contains `cleanup` | — | Set `CleanupOnFail` | — |
| `WaitStrategy` | Set | Set | Set |
| `WaitForJobs` | Set | Set | — |
| `Timeout` | Set | Set | Set |
| `MaxHistory` | — | Set | — |
| `DisableChartHooks` | `DisableHooks` | `DisableHooks` | `DisableHooks` |
| `SkipCRDs` | Set | — | — |

Dry-run is execution intent, not release lifecycle policy, and therefore remains outside `releaseLifecycle`. The command-to-provider path MUST propagate it independently to `Install.DryRunStrategy` or `Upgrade.DryRunStrategy` for apply/deploy and `Uninstall.DryRun` for delete. Apply and deploy use Helm's server-side dry-run strategy so validation reaches the cluster without persisting a release.

| Atmos operation | Provider operation | Helm timeout and recovery behavior |
| --- | --- | --- |
| apply/deploy, no release history | Install | `Install.Timeout`; on failure, `RollbackOnFailure` performs Helm's internal uninstall recovery using the same action configuration. |
| apply/deploy, existing release | Upgrade | `Upgrade.Timeout`; on failure, `RollbackOnFailure` performs Helm's internal rollback, with `CleanupOnFail` applied when configured. |
| Helm internal upgrade recovery | Rollback | Helm propagates the upgrade timeout and wait configuration into its internal rollback action; Atmos returns the original operation as failed even when recovery succeeds. |
| delete | Uninstall | `Uninstall.Timeout`; no automatic recovery action follows an uninstall failure. |

Action mapping should live in small, unit-testable helpers. The scheduler and command packages must not import Helm SDK action types merely to configure lifecycle policy.

### Command Flags

Use the standard Atmos parser and command registry. Mirror Helm names when the meaning is identical:

| Atmos flag | Configuration field | Commands |
| --- | --- | --- |
| `--on-failure=<actions>` | `on_failure` | apply, deploy |
| `--wait[=strategy]` | `wait_strategy` | apply, deploy, delete |
| `--wait-for-jobs[=bool]` | `wait_for_jobs` | apply, deploy |
| `--timeout` | `timeout` | apply, deploy, delete |
| `--history-max` | `max_history` | apply, deploy |
| `--no-hooks[=bool]` | `disable_chart_hooks` | apply, deploy, delete |
| `--skip-crds[=bool]` | `skip_crds` | apply, deploy |

`--on-failure` is a comma-separated string-slice flag. Repeating the flag accumulates values within that invocation, while the resulting CLI list replaces stack configuration. An explicit empty value clears the policy. Unknown actions fail before chart acquisition or cluster access. Flags must be represented in execution summaries using canonical field names. Alias warnings use Atmos UI/logging primitives, not direct standard-output writes.

## Observability

The Helm CI/job summary should include a non-secret lifecycle block for cluster-backed operations:

```yaml
lifecycle:
  operation: upgrade
  wait_strategy: watcher
  wait_for_jobs: true
  timeout: 1h0m0s
  on_failure:
    - rollback
    - cleanup
  max_history: 10
  disable_chart_hooks: false
```

The summary reports effective values after list normalization, alias normalization, and CLI overrides. It does not report inapplicable settings as if Helm used them.

For an external provision target, the normal execution summary reports the bypass without presenting stored lifecycle values as active policy:

```yaml
lifecycle:
  applied: false
  target_kind: git
  reason: external_target
```

This summary is emitted at the normal CI/job-summary level. A warning is unnecessary for stored defaults because dual cluster/external components are supported intentionally; explicit lifecycle flags on an external target remain an error.

At debug level, Atmos logs:

- Whether the action selected install or upgrade.
- The effective wait strategy and why it changed.
- Which operation-specific fields were ignored.
- Which failure actions were requested.
- Additional diagnostic detail when the selected target bypasses release lifecycle behavior.

Errors must use static Atmos sentinel errors where callers need classification and wrap the underlying Helm or Kubernetes cause.

## Backward Compatibility

Native Helm remains experimental, but existing manifests must remain structurally valid.

- Omitted lifecycle fields retain `hookOnly`, no failure actions, no Job waiting, hooks enabled, and CRDs enabled.
- During v1.225.x, an omitted timeout preserves the current SDK zero value and emits a migration warning once per selected component per invocation. Starting with v1.226.0, the default becomes the documented Helm CLI value of five minutes and the warning is removed.
- `timeout: 0s` explicitly preserves unbounded timeout behavior without a migration warning.
- An omitted `max_history` changes from the SDK zero value to the Helm CLI default of `10`; `max_history: 0` explicitly preserves unlimited history.
- Direct single-component commands and bulk commands resolve the same lifecycle configuration.
- External GitOps delivery remains render-and-deliver and does not acquire release semantics.
- Existing Atmos `hooks:` and `--skip-hooks` behavior is unchanged.

The release notes must call out the timeout migration schedule and the new ten-revision history limit. A previously hanging hook may fail after the timeout transition, a workload that legitimately needs more time must configure a larger value, and an installation that requires unlimited release history must configure `max_history: 0`.

## Security and Safety

- Lifecycle summaries contain no rendered values, credentials, Kubernetes Secrets, or logs.
- Rollback does not change secret masking behavior.
- `disable_chart_hooks` must not disable Atmos policy or security hooks.
- Validation runs before chart retrieval and cluster mutation.
- Explicit lifecycle flags on an external target fail rather than creating a false impression that rollback or waiting occurred.
- Timeout and cancellation errors preserve enough context to identify the component, stack, release, namespace, operation, and effective wait strategy.

## Testing Strategy

### Unit Tests

- Decode every lifecycle field from a processed Helm component.
- Verify built-in defaults, including `max_history: 10` and the staged timeout default.
- Verify the v1.225.x phase: an omitted timeout remains unbounded and warns once per selected component per invocation, while explicit `timeout: 0s` remains unbounded without warning.
- Verify the v1.226.0 phase: an omitted timeout resolves to `5m` without a migration warning.
- Verify stack-level Helm defaults, abstract inheritance, concrete overrides, explicit `false` values, and an explicitly empty `on_failure` list.
- Verify `on_failure` replacement, clearing, append-mode accumulation, duplicate normalization, and canonical summary order.
- Verify `--on-failure` replaces stack configuration, repeated flags accumulate invocation values, and `--on-failure=` clears inherited actions.
- Reject non-list configuration, non-string list items, and unknown failure actions before chart acquisition.
- Validate all wait strategies and reject unknown values.
- Validate negative timeout and history values.
- Validate `wait_for_jobs` against the effective strategy.
- Verify a `rollback` action promotes `hookOnly` to `watcher` before action mapping.
- Verify every install, upgrade, and delete action field mapping.
- Verify operation-inapplicable fields are omitted.
- Verify only explicitly changed flags override stack configuration.
- Verify explicit `=false` command-line values override stack-level `true` for every lifecycle Boolean flag.
- Verify explicit lifecycle flags fail for external provision targets.
- Verify apply and delete dry-run propagation through the complete command-to-provider path.
- Verify caller-context propagation and cancellation.
- Verify canonical lifecycle fields in summaries.

### Stack Processing Tests

- Stack-level `helm` lifecycle defaults reach every concrete Helm component.
- Base-component lifecycle fields inherit through multiple levels.
- Concrete values override inherited values under the default `replace` list strategy.
- `on_failure` honors `replace` and `append` list merge strategies and normalizes duplicates.
- Timeout presence survives stack processing so omitted and explicit `0s` remain distinguishable.
- Lifecycle fields survive `describe component` and `describe stacks` processing.
- JSON schema accepts valid fields and rejects unknown failure actions, unknown wait strategies, or negative history values.

### Helm SDK Tests

Use the existing in-memory release lifecycle tests to cover:

- First install and subsequent upgrade with lifecycle mapping.
- Dry-run install and upgrade without persisted history.
- Default history pruning at ten revisions, a positive `max_history` override, and explicit unlimited `max_history: 0`.
- Chart-hook suppression on install and upgrade.
- CRD skipping on install.
- Upgrade cleanup with rollback both enabled and disabled.
- Delete timeout, wait strategy, chart-hook suppression, and dry run.

### k3s Integration Tests

Add a small deterministic chart fixture with ordinary resources and weighted hook Jobs:

1. A pre-install/pre-upgrade hook at weight `-2` that creates a ConfigMap marked `helm.sh/resource-policy: keep`.
2. A hook Job at weight `-1` whose success requires the ConfigMap, proving lower-weight hooks complete first.
3. A failing hook Job.
4. A slow ordinary Job used to verify `wait_for_jobs`.
5. A Deployment whose readiness can be delayed without pulling a large image.
6. A CRD used to verify `skip_crds`.

Required scenarios:

- `hookOnly` returns without waiting for the delayed Deployment.
- `watcher` waits for Deployment readiness.
- `wait_for_jobs` waits for the ordinary Job.
- Timeout fails within a bounded tolerance.
- Failed first install with rollback leaves no deployed release.
- Failed upgrade with rollback restores the previous successful release.
- Failed upgrade cleanup removes newly created resources.
- CRD installation uses Helm's separate recognition wait and is not shortened by the release `timeout` value.
- Weighted hooks execute in ascending weight order on first install and upgrade.
- Rollback and failed-install cleanup preserve resources marked `helm.sh/resource-policy: keep`.
- `disable_chart_hooks` prevents the fixture hooks from executing.
- `skip_crds` does not install the fixture CRD.
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
2. Add presence-aware input decoding and canonical `releaseLifecycle` resolution.
3. Add failure-action, wait strategy, staged timeout, history default, wait-alias, and cross-field tests.

### Phase 2: Stack Processing and Schemas

1. Add lifecycle fields to native Helm stack processing.
2. Add stack-level Helm defaults and inheritance tests.
3. Update stack schemas and generated schema artifacts.

### Phase 3: Command and Helm Action Mapping

1. Register lifecycle flags on applicable commands.
2. Configure install, upgrade, and uninstall actions from the canonical lifecycle.
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

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Apply dry-run remains disconnected from cluster delivery | A command presented as non-mutating changes release or cluster state | Phase 0 prerequisite and command-to-provider mutation regression test |
| Five-minute default breaks long-running existing releases | Apply begins failing where it previously waited indefinitely | One-minor warning window; explicit `timeout: 0s`; actionable timeout errors; examples for `60m` |
| Ten-revision history default prunes older release records | Users lose rollback history they expected Atmos to retain indefinitely | Prominent release note; explicit `max_history: 0`; pruning tests against the Helm CLI-compatible default |
| `wait_for_jobs` appears effective under `hookOnly` | Users believe ordinary Jobs are gated when they are not | Validate against the effective strategy and explain hook Jobs separately |
| Rollback is mistaken for success | Dependents run after the desired version failed | Always return node failure after recovery |
| `disable_chart_hooks` is confused with Atmos hooks | Policy hooks are accidentally assumed disabled | Distinct configuration name; retain `--skip-hooks` for Atmos hooks |
| Lifecycle flags appear to affect GitOps delivery | Users assume an external controller waited or rolled back | Reject explicit flags for external targets and report `lifecycle.applied: false` in the normal summary |
| Context refactor affects other component providers | Cancellation regression outside Helm | Add a backward-compatible context accessor and provider-level tests |
| Helm SDK behavior changes in later 4.x releases | Semantics drift from documentation | Keep canonical mapping isolated and run lifecycle tests against dependency upgrades |
| R4 diagnostics later require custom rollback orchestration | Duplicate lifecycle configuration or incompatible behavior | Keep public contract independent of whether Helm or Atmos performs recovery |

## Success Criteria

- All lifecycle fields resolve through type defaults, inheritance, concrete components, and explicit CLI overrides.
- `atmos helm apply --dry-run` and `delete --dry-run` do not mutate release state.
- An omitted timeout remains unbounded with one warning per selected component per invocation in v1.225.x and resolves to `5m` without that warning from v1.226.0, while explicit `timeout: 0s` remains unbounded without a warning in both phases.
- An omitted `max_history` prunes to ten revisions and explicit `max_history: 0` remains unlimited.
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
4. Additional Helm 4 release controls after production feedback, including uninstall `keep_history` and cascade behavior.
5. Mixed-kind DAG execution using the same Helm completion contract.
