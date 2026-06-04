# Hooks + Outputs for Custom Component Types

**Status:** Implemented (PR #2469, stacked on and pending merge into #1904)
**Depends on:** #1904 (custom component types for custom commands)

## Summary

Extend the hooks engine so it fires lifecycle events for **custom component
types** (the ones PR #1904 introduces), and let custom command steps publish
**outputs** that hooks can read and persist to stores. This closes the apply
→ outputs → next-apply loop for non-Terraform component types, the same way
`after-terraform-apply` + `store` + `!terraform.state` closes it for Terraform.

## Motivation

PR #1904 lets users define custom component types (e.g. `agent`, `ansible`,
`manifest`) and operate on them through custom commands with full stack
configuration, inheritance, and `{{ .Component.* }}` template access. What's
missing is a way to persist the *result* of an apply (the IDs, ARNs, or
whatever the custom command created) and read it back on subsequent runs.

For Terraform components this loop exists today:

1. `atmos terraform apply` runs Terraform.
2. The `after-terraform-apply` hook fires.
3. The `store` hook command pulls outputs from Terraform state and writes
   them to a configured store (SSM, Vault, etc.).
4. Other components — or the next apply of this one — read those values via
   `!store` or `!terraform.state`.

For custom component types, step 2 doesn't fire (only `after-terraform-apply`
exists today) and step 3 has nothing to read from (the store-cmd's
`outputGetter` is hard-wired to Terraform state).

The motivating concrete case is an "Anthropic Managed Agent" component type:
`atmos agent apply slack-knowledge -s dev` creates an agent via the Anthropic
API, gets back `agent_id` and `env_id`, and today must write them back
imperatively (boto3 inside the custom command's Python step). We want
declarative hook-driven write-back, identical in shape to what Terraform users
already have.

## Design

### 1. Lifecycle events: `<phase>.<type>.<subcommand>`

The built-in terraform events are already named `<phase>.<kind>.<subcommand>`
(`before.terraform.apply`, `after.terraform.apply`, `after.terraform.plan`, …).
Custom component types join the **same taxonomy** rather than being anchored to
terraform's `apply` verb: for `atmos <type> <subcommand>` we fire
`before.<type>.<subcommand>` before the steps run and `after.<type>.<subcommand>`
after they all succeed.

```go
// pkg/hooks/event.go
const (
    PhaseBefore = "before"
    PhaseAfter  = "after"
)

// Built-in terraform events — unchanged, same <phase>.<kind>.<subcommand> shape.
const (
    BeforeTerraformInit  HookEvent = "before.terraform.init"
    BeforeTerraformApply HookEvent = "before.terraform.apply"
    AfterTerraformApply  HookEvent = "after.terraform.apply"
    BeforeTerraformPlan  HookEvent = "before.terraform.plan"
    AfterTerraformPlan   HookEvent = "after.terraform.plan"
)

// ComponentEvent builds the event for a custom component kind + subcommand,
// e.g. ComponentEvent(PhaseAfter, "agent", "apply") => "after.agent.apply".
func ComponentEvent(phase, componentType, subcommand string) HookEvent {
    return HookEvent(strings.Join([]string{phase, componentType, subcommand}, "."))
}
```

For `atmos agent apply router` (type `agent`, subcommand `apply`):

| phase | event fired |
|-------|-------------|
| before steps | `before.agent.apply` |
| after steps succeed | `after.agent.apply` |

A hook subscribes with `events: [after.agent.apply]` (dashed `after-agent-apply`
also works — `NormalizeEvent` converts dashes to dots consistently on both the
fired event and the configured events).

**Why not a generic `after-apply`?** `apply` is a terraform verb. Encoding the
subcommand in the event name means `atmos agent describe` fires
`after.agent.describe` — a *distinct* event that apply-oriented `store` hooks
don't subscribe to. The event name itself is the gate, so there is **no opt-in
`fires:` field**: a component-bound command simply fires its own
`<phase>.<type>.<subcommand>` events, and hooks subscribe to the ones they care
about. This is exactly how terraform behaves (it never needed a `fires:` list).

### 2. Outputs file: how custom commands publish values

Custom commands don't have Terraform state to inspect, so they need a way
to surface what their apply *produced*. We pass that path through an env
var that atmos sets before invoking each step:

- Before running a custom command whose `component:` binding is set, atmos
  creates a temp file (e.g. `/tmp/atmos-outputs-<random>`).
- atmos exports `ATMOS_OUTPUTS=<path>` into the step's environment.
- Steps write to the file. Both formats are accepted, auto-detected by
  examining the first non-whitespace byte:
  - **JSON object** (starts with `{`): `{ "agent_id": "agent_abc123", "env_id": "env_xyz789" }`
  - **Shell key=value lines** (one per line): `agent_id=agent_abc123\nenv_id=env_xyz789\n`
- The shell form is for cheap-and-cheerful custom commands that just `echo
  KEY=VALUE >> "$ATMOS_OUTPUTS"`. The JSON form is for typed values, nested
  structures, and anything needing escaping.
- After all steps complete successfully, atmos reads + parses the file and
  makes it available to hook execution.

This is the analogue of "Terraform state, but for custom components." JSON
is preferred when values are non-trivial; shell form is preferred when the
custom command is shell-only and you'd rather not pipe to `jq`.

### 3. Generalized output resolution in `store` hook command

`pkg/hooks/store_cmd.go` currently does:

```go
outputValue, exists, err = c.outputGetter(c.atmosConfig, c.info.Stack, c.info.ComponentFromArg, outputKey, ...)
```

with `outputGetter` hard-typed as `TerraformOutputGetter`. Rather than collapse
everything into one signature, the `StoreCommand` carries **two** typed getters
and dispatches between them, so the terraform path keeps its existing signature
untouched:

```go
// Existing — unchanged.
type TerraformOutputGetter func(
    atmosConfig *schema.AtmosConfiguration,
    stack, component, output string,
    skipCache bool,
    authContext *schema.AuthContext,
    authManager any,
) (any, bool, error)

// New — reads the ATMOS_OUTPUTS file for a custom-component apply.
type CustomOutputGetter func(
    info *schema.ConfigAndStacksInfo,
    outputKey string,
) (any, bool, error)

type StoreCommand struct {
    // ...
    terraformOutputter TerraformOutputGetter // tfoutput.GetOutput
    customOutputter    CustomOutputGetter    // defaultCustomOutputter
}
```

And inside `getOutputValue`, dispatch on component type via `isTerraformComponent()`:

- If `info.ComponentType == "terraform"` (or empty, for legacy) → `terraformOutputter`.
- Otherwise → `customOutputter`, which reads `info.OutputsFilePath` (set by the
  custom command runner), parses it (JSON or `KEY=VALUE`), and looks up `outputKey`.

Both getters are struct fields so tests can inject fakes and assert which path
ran without touching terraform state or the filesystem.

`ComponentType` and `OutputsFilePath` get added to `ConfigAndStacksInfo`
(both currently exist for component, just need plumbing for the outputs file
path).

### 4. Firing the event from the custom command execution path

In `cmd/cmd_utils.go`, the custom command step loop is where steps run. When the
command has a `component:` binding, atmos fires `before.<type>.<subcommand>`
before the steps and `after.<type>.<subcommand>` after they all succeed, via the
same `hooks.RunAll` path the terraform command uses today (a `fireComponentHook`
closure builds the event with `hookspkg.ComponentEvent`).

Pseudocode:

```go
// before the step loop, if commandConfig.Component != nil:
outputsFile := createOutputsFile()
defer os.Remove(outputsFile)
env = append(env, fmt.Sprintf("ATMOS_OUTPUTS=%s", outputsFile))

// ... existing step loop ...

// before the loop, if component-typed: fire before.<type>.<subcommand>
info.OutputsFilePath = outputsFile
info.ComponentType = commandConfig.Component.Type
runHooks(ComponentEvent(PhaseBefore, type, subcommand), cmd, args, &info)

// ... existing step loop ...

// after the loop, if component-typed: fire after.<type>.<subcommand>
runHooks(ComponentEvent(PhaseAfter, type, subcommand), cmd, args, &info)
```

The `runHooks` helper currently lives in `cmd/terraform/utils.go`. We extract
it to a shared location (`cmd/internal/runhooks.go`) so both terraform and
custom command paths use it.

### 5. No `fires:` field — events are derived from type + subcommand

There is **no opt-in field** on the command. The event name encodes the
subcommand, so firing is automatic and the gating is structural:

```yaml
commands:
  - name: agent
    commands:
      - name: apply
        component: { type: agent }
        steps: [...]      # fires before.agent.apply / after.agent.apply

      - name: describe
        component: { type: agent }
        steps: [...]      # fires before.agent.describe / after.agent.describe
```

A `store` hook subscribing to `after.agent.apply` runs after `apply` but not
after `describe`, because `describe` fires `after.agent.describe` — a different
event. No config flag is needed to keep read-only commands quiet.

Firing is implemented by a `fireComponentHook(phase)` closure in
`cmd/cmd_utils.go`: it builds `hookspkg.ComponentEvent(phase, type, name)` and
calls `internal.RunHooks` with the resolved component. `before` fires once after
the component resolves (before step 0); `after` fires once all steps succeed.

### 5b. Resolved-component path: skip the re-describe trap

`pkg/hooks/GetHooks` calls `e.ExecuteDescribeComponent` to discover the
component's `hooks:` block. That describe path only knows about the built-in
component types — custom types resolved via the #1904 registry path are
**not** discoverable through it, so `GetHooks` returns "component not found"
for custom types.

The custom command runner already has the resolved component in scope (it
called `processCustomComponentType`, which writes `data["Component"]` to
power `{{ .Component.* }}` template variables). We thread that same
resolved map through to a new constructor:

```go
// pkg/hooks/hooks.go
func HooksFromComponent(
    atmosConfig *schema.AtmosConfiguration,
    info *schema.ConfigAndStacksInfo,
    resolvedComponent map[string]any,
) (*Hooks, error)
```

`HooksFromComponent` pulls `resolvedComponent["hooks"]` and reuses the
existing YAML marshal/unmarshal trick from `GetHooks` to coerce it into a
typed `map[string]Hook`. If the `hooks:` key is absent, it returns an empty
`Hooks` (no error). If the key is present but malformed (not a map), it
returns a clean error rather than panicking.

`cmd/internal/RunHooks` gains an optional final parameter:

```go
func RunHooks(
    event h.HookEvent,
    cmd *cobra.Command,
    args []string,
    commandName string,
    populate func(info *schema.ConfigAndStacksInfo),
    preResolvedComponent map[string]any, // nil → fall back to GetHooks
) error
```

The terraform code path passes `nil` and is unchanged. The custom command
path passes `data["Component"].(map[string]any)`, hoisted out of the step
loop into a `resolvedComponent` variable so the post-loop firing block can
see it.

### 5c. Hook configs for custom components

The `Hook` struct stays the same. The `events:` list is just strings;
`after.agent.apply` (custom) and `after-terraform-apply` (built-in) are both
valid values.

Hook configs for custom components look identical to terraform configs:

```yaml
# stacks/catalog/agent/slack-knowledge.yaml
components:
  agent:
    slack-knowledge:
      vars: { ... }
      hooks:
        store-runtime:
          events: [after.agent.apply]
          command: store
          name: '{{ .vars.stage }}/ssm'
          outputs:
            agent-id: .agent_id    # leading dot = look up in outputs file
            env-id: .env_id
```

The leading-dot convention for "output reference" stays the same; only the
*source* of the output differs by component type.

## Worked example

`atmos.yaml`:

```yaml
toolchain: { ... }   # unchanged

components:
  agent:
    base_path: apps

stores:
  dev/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-west-2
      prefix: /ai-harness/dev

commands:
  - name: agent
    commands:
      - name: apply
        component: { type: agent }
        arguments:
          - { name: component, type: component, required: true }
        flags:
          - { name: stack, shorthand: s, semantic_type: stack, required: true }
        steps:
          - |
            uv run ai-harness-apply \
              --component {{ .Component.component }} \
              --stack {{ .Component.atmos_stack }}
            # ai-harness-apply writes agent_id + env_id to $ATMOS_OUTPUTS
```

Catalog file:

```yaml
# stacks/catalog/agent/slack-knowledge.yaml
components:
  agent:
    slack-knowledge:
      vars:
        agent: { name: slack-knowledge, model: claude-opus-4-7, ... }
        environment: { name: slack-knowledge-env, ... }
        runtime:
          agent_id: !store.get '{{ .vars.stage }}/ssm' /slack-knowledge/agent-id | default ""
          env_id:   !store.get '{{ .vars.stage }}/ssm' /slack-knowledge/env-id   | default ""
      hooks:
        store-runtime:
          events: [after.agent.apply]
          command: store
          name: '{{ .vars.stage }}/ssm'
          outputs:
            /slack-knowledge/agent-id: .agent_id
            /slack-knowledge/env-id:   .env_id
```

The `ai-harness-apply` Python script writes:

```json
// $ATMOS_OUTPUTS
{ "agent_id": "agent_abc123", "env_id": "env_xyz789" }
```

After `atmos agent apply slack-knowledge -s dev` succeeds, SSM has
`/ai-harness/dev/slack-knowledge/agent-id = agent_abc123`. On the next
`atmos agent run slack-knowledge -s dev`, the catalog's `!store.get` reads
that value back, the apply tool sees a non-empty `agent_id`, and hits the
update branch instead of create.

## Out of scope (for this PR)

- `after-plan` / other terraform-specific phases for custom components. Custom
  commands don't have a notion of "plan" yet; the `<phase>.<type>.<subcommand>`
  scheme already accommodates them later without breaking changes.
- Generalizing output resolution to hooks other than `store` (e.g. webhooks
  posting outputs to a service). Easy follow-up once the abstraction is
  proven for `store`.

## Implementation plan

1. `pkg/hooks/event.go` — add `PhaseBefore`/`PhaseAfter` constants and the
   `ComponentEvent(phase, type, subcommand)` helper; `NormalizeEvent` maps
   dashes to dots. Built-in terraform events keep their `<phase>.terraform.<sub>`
   names.
2. `pkg/hooks/store_cmd.go` — refactor `outputGetter` to dispatch on
   component type. Add `defaultCustomOutputter(info, key)`.
3. `pkg/schema/schema.go` — add `ComponentType` and `OutputsFilePath` to
   `ConfigAndStacksInfo` if not already present.
4. `cmd/internal/runhooks.go` — extract `RunHooks` so it isn't tied to the
   terraform package; add an optional `preResolvedComponent` parameter.
5. `cmd/cmd_utils.go` — in the custom command path, create the outputs file,
   export `ATMOS_OUTPUTS`, and fire `before.<type>.<subcommand>` (before steps)
   and `after.<type>.<subcommand>` (after steps succeed) for component-typed
   commands.
6. Tests:
   - Unit: event naming/normalization, outputs-file parsing, dispatch in store_cmd.
   - Integration: a custom command writes to `$ATMOS_OUTPUTS`, an
     `after.<type>.<subcommand>` hook reads from it and writes to a mock store.
7. Docs:
   - `website/docs/stacks/hooks.mdx` — document the custom-component events, the
     outputs file, and the per-component-type behavior.
   - `examples/custom-components/` — add a worked example with a store hook.

## Test scenarios

| # | Setup | Expected |
|---|-------|----------|
| 1 | Custom command, `component:` set, step exits 0, writes JSON to `$ATMOS_OUTPUTS`, hook `events: [after.<type>.<sub>]` | Hook fires, values written to store |
| 2 | Custom command, no `component:` set | No outputs file created, no hook fires (no regression) |
| 3 | Custom command, step fails | `after` hook does NOT fire (loop aborts before it) |
| 4 | `atmos <type> describe` with an `after.<type>.apply` hook configured | Hook does NOT fire (different event name) |
| 5 | Terraform component, `events: [after-terraform-apply]` | Existing behavior unchanged |
| 6 | Custom command writes nothing to `$ATMOS_OUTPUTS`, hook tries to reference `.foo` | Clear error: "output `foo` not found in outputs file" |
| 7 | Hook value with no leading dot (literal) | Stored as literal (existing behavior, unchanged) |

## Resolved design decisions

1. **Output file format**: accept both JSON and shell `KEY=VALUE` (auto-detect).
2. **Event naming**: `<phase>.<type>.<subcommand>` for all component types,
   matching the existing terraform events. No generic `after-apply`; no opt-in
   `fires:` field — the subcommand in the event name is the gate.
3. **`before` events**: in scope — `before.<type>.<subcommand>` fires before the
   step loop.

## Remaining open question

**Concurrent step writes to `$ATMOS_OUTPUTS`**: for v1, assume the typical
pattern (one step writes, others read). Document that simultaneous appends
from parallel steps are undefined behavior. Revisit if a real use case appears.
