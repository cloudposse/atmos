# Overriding Built-in Commands with Step-Based Custom Commands

## Overview

This PRD defines how a user-facing **built-in** Atmos command (for example `atmos terraform plan`) can be **overridden** by a step-based custom command, where one of the steps **invokes the original native built-in behavior** — without infinite recursion.

The goal is to let operators *wrap* a built-in (run pre-flight validation, gate on a confirmation, emit a banner) and then hand off to the real built-in, using the workflow step types that custom commands already share with `atmos workflow`.

The design adds two opt-in fields and **no new step type**:

- A command-level `override: true` that allows a custom command's `steps` to replace a built-in.
- A step-level `invoke:` modifier on `type: atmos` steps that selects *how* the atmos command runs: `exec` (re-exec the binary, the default and today's behavior) or `built-in` (call the retained native handler in-process, which cannot cycle).

## Problem Statement

### What users want

```yaml
# atmos.yaml
commands:
  - name: terraform                     # same name as the built-in
    description: Terraform with mandatory pre-flight validation
    steps:
      - "echo running validation..."
      - "atmos validate component vpc -s prod"
      - "<run the REAL native terraform plan here>"
```

The third step is the hard part: the user wants the **native** Atmos terraform behavior (varfile generation, backend configuration, workspace selection, auth context), not the raw `terraform` binary — and they want it reachable from *inside* an override of `terraform` itself.

### Why it does not work today

Two concrete blockers exist in the current code.

**1. A custom command cannot override a built-in with steps.** `processCustomCommands` (`cmd/cmd_utils.go:95-104`) looks for an existing command of the same name and, when it finds one, keeps the built-in and discards the custom steps:

```go
existing := findSubcommand(parentCommand, commandConfig.Name)
if existing != nil {
    if len(commandConfig.Steps) > 0 {
        ui.Warningf(
            "Custom command %q defines steps that conflict with built-in command %q; "+
                "built-in behavior preserved, custom steps ignored",
            commandConfig.Name, existing.CommandPath(),
        )
    }
    command = existing // reuse the built-in; the custom steps never run
}
```

This was made deliberate in **PR #2191** ("allow custom commands to merge into built-in namespaces at any level"): a same-named custom command may *add subcommands* under a built-in's namespace, but it may **not** replace the built-in's behavior with steps.

> Note: this contradicts the aspirational "Custom `terraform` command reuses the built-in and **replaces its behavior**" claim in `command-registry-pattern.md` (Scenario 1). That claim describes behavior that never shipped; this PRD is the authority on the actual and intended behavior.

**2. A `type: atmos` step would cycle.** The `atmos` step handler (`pkg/runner/step/atmos.go:128-152`) re-execs the atmos binary as a subprocess:

```go
atmosBin, err := os.Executable()
cmd := exec.CommandContext(ctx, atmosBin, args...) // runs `atmos <args>` as a child process
```

If a custom command named `terraform` could override the built-in, a step running `type: atmos` / `command: terraform plan` would re-exec `atmos terraform plan` → the child re-reads `atmos.yaml` → re-installs the same override → runs the same step → **infinite recursion**. `type: atmos` has no notion of "the built-in underneath my override"; it re-runs `atmos <args>` through full command resolution, which re-enters the override.

## Goals

- Allow an **opt-in** override of a user-facing built-in command with a step-based custom command.
- Provide a way for a step to invoke the **native built-in** behavior of the overridden command.
- **Guarantee no cycle** — by construction, not by a heuristic counter.
- Keep **100% backward compatibility**: existing custom commands and existing `type: atmos` steps behave exactly as today.
- Add **no new step type** and **no new templating syntax**.

## Non-Goals

- Overriding internal / non-user-facing plumbing commands.
- Deep-merging a custom command's properties into a built-in (override is whole-command replacement of the leaf behavior).
- A general plugin system (tracked separately under the command registry roadmap).
- Changing the default collision behavior — without explicit opt-in, the current "built-in preserved, custom steps ignored" warning (PR #2191) is retained.

## Design

### 1. Opt-in override (`override: true`)

A command-level boolean opts the custom command into replacing a built-in of the same name:

```yaml
commands:
  - name: terraform
    description: Terraform with mandatory pre-flight validation
    override: true          # opt-in: this custom command replaces the built-in's behavior
    arguments:
      - name: subcommand
      - name: component
    steps:
      - ...
```

Behavior:

- **Default (`override` unset / `false`)** — unchanged from PR #2191: if a same-named built-in exists and the custom command has `steps`, the built-in is preserved and the steps are ignored with the existing warning. Subcommand merging still works.
- **`override: true`** — `processCustomCommands` is allowed to replace the built-in's leaf `RunE` with the custom command's step execution. The original built-in command object is **retained** (see §3) so a step can still reach it.

Precedence is unchanged in ordering: built-ins register first (via the command registry), then custom commands are processed from `atmos.yaml`; the opted-in override rebinds the leaf behavior while keeping the built-in reachable for passthrough.

### 2. Invoking the native built-in: `invoke:` on `type: atmos`

`type:` continues to select the step **kind** (`atmos`, `shell`, `input`, `choose`, …). A new `invoke:` field selects **how** an `atmos` command step runs. It is orthogonal to `type:` and only meaningful when `type: atmos`:

```yaml
type: atmos
invoke: exec        # default. re-exec the atmos binary as a subprocess.
                    # full command resolution → custom overrides DO apply. identical to today.

type: atmos
invoke: built-in    # call the retained native built-in handler IN-PROCESS.
                    # bypasses command resolution → the override cannot re-enter itself → cannot cycle.
```

- `invoke:` defaults to `exec`, so **every existing `type: atmos` step is unchanged**. The field is purely additive.
- `invoke: built-in` is the cycle-proof passthrough intended for use inside an override.

#### Worked example

```yaml
# atmos.yaml
commands:
  - name: terraform
    description: Terraform with mandatory pre-flight validation
    override: true
    arguments:
      - name: subcommand
      - name: component
    steps:
      - name: validate
        type: atmos
        invoke: exec                    # full atmos CLI (custom overrides apply)
        command: validate component {{ .Arguments.component }} -s {{ .Flags.stack }}

      - name: plan
        type: atmos
        invoke: built-in                # native terraform handler, in-process — terminates
        command: terraform {{ .Arguments.subcommand }} {{ .Arguments.component }} -s {{ .Flags.stack }}
```

Running `atmos terraform plan vpc -s prod`:

1. The `terraform` override is invoked (replaces the built-in leaf because `override: true`).
2. Step `validate` runs `atmos validate component vpc -s prod` via `invoke: exec` — a normal subprocess. Different command path, no cycle.
3. Step `plan` runs the **retained native** `terraform plan` handler **in-process** via `invoke: built-in`. Because it never goes back through command resolution, the override is not re-entered. The step completes and returns.

Contrast with the footgun this prevents:

```yaml
      - name: plan
        type: atmos
        invoke: exec                    # would re-exec `atmos terraform plan` → re-enter THIS override → ♾️
        command: terraform plan {{ .Arguments.component }} -s {{ .Flags.stack }}
```

### 3. Anti-cycle mechanism: in-process invocation of the retained handler

The cycle is eliminated **by construction**, not detected after the fact.

When `override: true` rebinds a built-in's leaf behavior, Atmos **retains a reference to the original built-in command object** (its `RunE` / handler). An `invoke: built-in` step calls that retained handler directly, in-process:

- No subprocess is spawned.
- No `atmos.yaml` re-read, no `processCustomCommands`, no Cobra re-resolution.
- Therefore the override is structurally unreachable from within the native step → **no cycle is possible**, and no loop counter is needed.

### 4. Rejected alternative: `ATMOS_REEXEC_DEPTH`

Atmos already has a re-exec depth counter, `ATMOS_REEXEC_DEPTH` (`pkg/reexec/depth.go`), used to break loops in `pkg/version/reexec.go` and `pkg/auth/profile_fallback.go`. It is **not** suitable as the cycle guard here:

- It is a **single global counter** incremented on *every* re-exec — toolchain version re-exec, auth profile fallback, and any unrelated `type: atmos` step that legitimately targets a *different* command.
- It therefore cannot distinguish a genuine self-override cycle from legitimate deep cross-command nesting. A depth cap would **false-positive** on valid chains while still failing to pin the real signal.

This is recorded as a rejected alternative because it is the obvious-but-wrong approach; the per-construction in-process design above supersedes it.

### 5. Re-exec fallback (only if a built-in cannot be invoked in-process)

If a particular built-in cannot be safely invoked in-process (for example, it depends on process-global state set up only at CLI entry), the fallback is a **name-scoped bypass set** rather than a depth counter:

- The child process receives an env var listing the override names already entered on the current stack, e.g. `ATMOS_BYPASS_CUSTOM_COMMANDS=terraform`.
- The child disables exactly those overrides in `processCustomCommands`, so `terraform` resolves to the **native built-in** while every *other* custom command still works.
- Cycle detection becomes a precise per-command question — "is this override name already in the bypass set?" — not a global threshold.

This helper belongs in `pkg/reexec` alongside the existing `chdir.go` env-filtering helpers (it is a *set of names*, conceptually distinct from the `depth.go` counter). In-process invocation (§3) remains the primary path; this fallback is documented for completeness.

## Implementation Sketch

This PRD specifies the design; implementation is a separate follow-up. Anticipated touch points:

| Area | Change |
|------|--------|
| `cmd/cmd_utils.go` (`processCustomCommands`, ~`:95-104`) | Honor `override: true` to allow step-override of a built-in; **retain the original built-in command object** when replacing it. |
| Schema (`pkg/schema/workflow.go` `WorkflowStep`; command schema) | Add step-level `invoke` field (default `exec`) and command-level `override` field. |
| `pkg/runner/step/atmos.go` | Branch on `invoke`: `exec` = current re-exec path (`:128-152`); `built-in` = invoke the retained built-in handler in-process. |
| `pkg/reexec/` | (Fallback only) name-scoped bypass-set helper, mirroring `chdir.go`. |
| `pkg/datafetcher/schema/` | Update JSON schemas for the new `invoke` and `override` fields. |
| `errors/errors.go` | Sentinel for the fallback's "override cycle detected" case, if the fallback path is implemented. |

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Accidentally shadowing a critical built-in | Override is **opt-in** (`override: true`); default behavior preserves the built-in with a warning. |
| Retained-handler reference lost during override registration | Capture the original command object before rebinding; cover with a unit test that asserts the native handler is still invocable after override. |
| Fallback re-exec doesn't carry the bypass set across platforms | Reuse the `pkg/reexec` env-filtering pattern (already cross-platform); test the bypass set round-trips. |
| CI / non-TTY behavior of `invoke: built-in` | In-process invocation inherits the parent's IO context; add non-TTY coverage. |
| Users reach for `invoke: exec` inside a same-name override and self-cycle | Detect the obvious case (an `invoke: exec` step whose command targets the command currently being overridden) and emit a clear error pointing to `invoke: built-in`. |

## References

- `docs/prd/command-registry-pattern.md` — command registry; override behavior (corrected by this PRD).
- `docs/prd/workflow-step-types.md` — step type catalog; the `atmos` command step.
- PR #1899 — workflow step types via registry (DEV-263, DEV-2969).
- PR #1901 — `pkg/runner` unified task execution.
- PR #2191 — "allow custom commands to merge into built-in namespaces at any level" (the current collision guard).
- `pkg/reexec/depth.go`, `pkg/reexec/chdir.go` — re-exec primitives.

## Changelog

| Date | Change |
|------|--------|
| 2026-05-29 | Initial PRD: opt-in `override`, `invoke:` step modifier (`exec`/`built-in`), in-process cycle-proof passthrough, rejected `ATMOS_REEXEC_DEPTH` guard. |
