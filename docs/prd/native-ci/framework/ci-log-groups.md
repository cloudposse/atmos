# PRD: Collapsible CI log groups

## Status

Implemented (single PR), retroactive PRD. All three grouping dimensions ship: step-level (workflows + custom commands), phase-level (terraform/tofu), and invocation-level (any `atmos` command). Phase-level grouping for helmfile/packer and for registry-based/custom component types, plus GitLab/Azure DevOps providers, are documented follow-ups.

## Problem

When Atmos runs inside CI, its output lands in one flat, undifferentiated log. A workflow with `vendor → plan → apply → smoke-test` steps, or a single `atmos terraform apply` (which internally runs `init` then `apply`), produces thousands of interleaved lines with no marker for where one unit of work ends and the next begins. When a run fails, finding the part that broke means scrolling and squinting.

CI providers already solve this: GitHub Actions folds a region of the log into a named, collapsible group via the `::group::<name>` / `::endgroup::` workflow commands (GitLab and Azure DevOps have equivalents). The usual way to get this is to hand-write `echo "::group::..."` around a `run:` step in the CI YAML. But Atmos output isn't `run:` lines the author controls one at a time, so there was no clean place to emit those markers — and doing it by hand doesn't scale across every workflow, command, and tool phase.

## The three dimensions of grouping

A key insight is that "group Atmos output in CI" is not one feature — there are **three independent boundaries** at which output can be folded:

| # | Dimension | Group boundary | Example | Status |
|---|-----------|----------------|---------|--------|
| 1 | **Step-level** | one group per workflow / custom-command **step** | a workflow step `deploy cluster` running `atmos terraform apply eks` → one group | **Implemented** |
| 2 | **Invocation-level** | one group per whole top-level `atmos <command>` run | `atmos terraform plan vpc -s plat-ue1-prod` run from CI YAML → one group | **Implemented** |
| 3 | **Phase-level** | groups for the internal **phases** of one command invocation | within one `atmos terraform apply`: `terraform init`, then `terraform apply` | **Implemented** (terraform/tofu) |

The original inspiration — `echo "::group::terraform init (bounded)"` — is Dimension 3.

### Why a `mode`, not toggles

CI providers **do not support nested groups**. If two dimensions both opened a group around the same output, the result is broken rendering. So the dimensions are not independently-toggled booleans — they are mutually exclusive, and a single `mode` selects the granularity:

```yaml
ci:
  enabled: true     # master switch
  groups:
    mode: auto      # auto | invocation | off  (default: auto)
```

- `auto` (default): the finest dimension that applies to each command — **steps** for workflows/custom commands, **phases** for terraform/tofu. (D1 + D3, which never overlap in one process: a workflow step that runs `atmos terraform apply` is one step group, and the child terraform's phases stay flat inside it — see Nesting.)
- `invocation`: one group per whole `atmos` command (D2); finer step/phase grouping suppressed.
- `off`: no grouping.

A mode is mutually exclusive by construction, so there is no way to misconfigure conflicting dimensions.

## Design

### Provider capability (reuse the CI provider registry)

`pkg/ci` already has a CI provider registry with optional capability interfaces (`DebugModeDetector`, `CacheProvider`). Log grouping is another optional capability:

```go
// pkg/ci/internal/provider/types.go
type LogGrouper interface {
    StartGroup(w io.Writer, name string)
    EndGroup(w io.Writer)
}
```

The GitHub provider implements it (`pkg/ci/providers/github/loggroup.go`), emitting `::group::<escaped>` / `::endgroup::` and reusing the existing `escapeData` helper from `annotations.go`. Providers without log grouping degrade to a no-op.

### Public seam

`pkg/ci/loggroup.go` exposes the provider-agnostic entry points:

```go
type Dimension int // DimensionStep, DimensionPhase, DimensionInvocation

func GroupingEnabled(cfg *schema.AtmosConfiguration) bool
func Group(cfg *schema.AtmosConfiguration, dim Dimension, name string, fn func() error) error
func LogGroupSentinelEnv() string // "ATMOS_CI_LOG_GROUP_ACTIVE=1"
```

`Group` emits markers around `fn` only when the resolved mode selects `dim` (`dimensionActive`): step/phase under `auto`, invocation under `invocation`. The end marker is always emitted (deferred), so a failing operation never leaves a group open. Markers are written through `io.MaskWriter`, so a secret in a label cannot leak. `resolveGroupMode` maps config → `off`/`auto`/`invocation` (unknown values degrade to `auto`).

### Call sites (one per dimension)

- **D1 (steps)** — the step abstraction owns it: `pkg/runner/step/loggroup.go` `RunGrouped(cfg, name, command, fn)` calls `Group(…, DimensionStep, …)`. Both orchestrators wrap their per-step dispatch through it: `internal/exec/workflow_utils.go` `ExecuteWorkflow` (the live workflow path) and `cmd/cmd_utils.go` `executeCustomCommand`. (The unused parallel `pkg/workflow/executor.go` is also wired for forward-consistency.)
- **D2 (invocation)** — `cmd/root.go` `Execute` wraps `internal.Execute(RootCmd)` in `Group(…, DimensionInvocation, "atmos <command path> [first positional]", …)`. The label is derived from the effective Cobra args after Atmos preprocessing, omits flags and anything after `--`, and is a no-op unless `mode: invocation`.
- **D3 (phases)** — `internal/exec/terraform_execute_helpers_exec.go` `executeCommandPipeline` wraps the `init` phase and the main subcommand in `Group(…, DimensionPhase, "terraform <phase>", …)`. The label uses the resolved tool's base name (`terraform`/`tofu`).

### Component types and the registry

The intent is that phase grouping is **built into the component types**, ideally via the component registry (`pkg/component`). Reality check: the registry holds the *extensible* types — `container`, `ansible`, `custom`, `emulator`, `mock` — while the built-in IaC tools (`terraform`/`helmfile`/`packer`) execute through `internal/exec`, not the registry. So:

- For **terraform/tofu**, phase grouping wraps the live `executeCommandPipeline` phase boundaries (above). This is where the phases actually run.
- For **registry-based and custom component types**, the same pattern applies: a provider's `Execute` wraps its own phases with `ci.Group(cfg, ci.DimensionPhase, label, fn)` when it has phases worth grouping. (No phased registry provider exists yet, so nothing is wired there today — adding it is mechanical and avoids a stub interface no one calls.)

This keeps grouping a property of each component type's execution rather than a central hardcoded list.

### Nesting

CI providers do not support nested groups. The unifying rule is **outermost wins**, enforced two ways:

- **Cross-process:** when a parent group is already open, or when the current subprocess boundary is about to open a group for the selected mode/dimension, the step/command orchestrator appends `LogGroupSentinelEnv()` to the child subprocess's environment. A child `atmos` process sees `ATMOS_CI_LOG_GROUP_ACTIVE` set, so its own grouping is suppressed. This makes a workflow step's group the outer boundary and keeps a nested `atmos terraform apply` (and its phases) flat inside it — and keeps an `invocation`-mode group flat over everything beneath it.
- **In-process:** a process-local depth counter ensures only the outermost `Group` emits.

A terraform phase spawns the `terraform` binary (not `atmos`), so phases need no sentinel of their own; they're suppressed only when an ancestor `atmos` already grouped.

## Known limitations

- In `invocation` mode, Atmos cannot detect user-written `::group::` wrappers in CI YAML. The sentinel suppresses nested *Atmos* grouping, but it cannot suppress a manual outer wrapper already emitted by the workflow. Users should remove manual outer wrappers when enabling `ci.groups.mode: invocation` to avoid double-grouped logs.

### Configuration

`schema.CIGroupsConfig{ Mode string }` on `schema.CIConfig.Groups`, gated by the `ci.enabled` master switch. Env override `ATMOS_CI_GROUPS_MODE`, bound in `pkg/config/load.go` next to the `ci.cache.*` bindings.

## Out of scope (follow-ups)

- **Phase-level grouping for helmfile and packer** — same `ci.Group(DimensionPhase, …)` pattern at their exec pipelines.
- **Phase-level grouping for registry-based / custom component types** — adopt the same call in their `Execute` when they gain phased execution.
- **GitLab / Azure DevOps `LogGrouper` implementations** — section markers (GitLab uses timestamped `section_start`/`section_end`).

## Testing

- `pkg/ci/providers/github`: `StartGroup`/`EndGroup` emit exact markers; escaping of `%`, `\n`, `\r`.
- `pkg/ci`: `resolveGroupMode` mapping; `dimensionActive` mode×dimension matrix; `Group` brackets `fn`, end-on-error, outermost-only, no-op when off/disabled/no-provider/sentinel-set; `GroupingEnabled`.
- `pkg/runner/step`: `RunGrouped` label selection and inactive passthrough.
- End-to-end (verified, gated on `GITHUB_ACTIONS` + `ci.enabled`):
  - D1 — workflow and custom-command steps emit one group per step in `auto`.
  - D2 — `invocation` mode wraps a whole command (custom command, `version`) in one group.
  - D3 — a direct `atmos terraform plan` emits `terraform init` and `terraform plan` groups in `auto` (real terraform run); `invocation` mode collapses them into one invocation group; no-CI emits nothing.
