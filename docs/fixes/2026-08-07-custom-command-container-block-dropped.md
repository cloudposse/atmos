# Fix: custom-command steps now decode and execute their step-level `container:` override

**Date:** 2026-08-07

## Summary

A field test found the same class of bug as
[docs/fixes/2026-08-05-custom-command-container-with-block-dropped.md](2026-08-05-custom-command-container-with-block-dropped.md),
in two more places, which turned out to have three independent root causes:

1. A custom command's step-level `container: false` (bare boolean opt-out)
  broke `InitCliConfig` outright for the **entire** `atmos.yaml` --
  `atmos example ...`, `atmos workflow ...`, `atmos describe stacks`,
  `atmos list stacks`, and every other command failed identically with
  `'container' expected a map or struct, got "bool"`, as long as this step
  existed anywhere in the merged config tree.
2. A custom command's step-level `container: {image: ..., ...}` (mapping
  form) decoded without error, but the step then silently ran on the bare
  host instead of inside the configured container -- the execution wiring
  never consulted the field at all.
3. Even after fixing (1) and (2), `container: false` still silently ran the
  step *inside* a container instead of on the host: `cmd/cmd_utils.go`'s
  `cloneCommand` gives each custom command's Cobra closure an independent
  copy of its `schema.Command` via a `json.Marshal`/`json.Unmarshal`
  round-trip, and `WorkflowContainer.Enabled` was tagged `json:"-"`,
  dropping the opt-out during that round-trip (`Enabled` came back `nil`,
  which `IsEnabled()` treats as *enabled*).

All three symptoms only affected custom commands (`commands:` in
`atmos.yaml`, loaded via mapstructure/Viper, then cloned via `cloneCommand`);
the identical `container:` block on a workflow-file step (loaded via
`yaml.Node.Decode`, never cloned through JSON) already worked correctly.

## Context

Same root cause class as the `with:` fix: `WorkflowContainer.UnmarshalYAML`
(`pkg/schema/workflow.go`) -- which accepts either a mapping config or the
bare boolean opt-out -- is a `yaml.Unmarshaler` method, invoked only when
something calls `yaml.Node.Decode` directly on a `*WorkflowContainer` value.
Standalone workflow files take that path. Custom commands decode via
`mapstructure` (`pkg/config/load.go`'s `atmosDecodeHook` ->
`schema.TasksDecodeHook()` -> `decodeTaskFromMap`), which has no notion of
`yaml.Unmarshaler`, so `WorkflowContainer.UnmarshalYAML` never ran for this
path:

- For the boolean form, `mapstructure` rejected the value outright (it
  expects a struct/map for a struct-typed field), and that decode error
  propagated all the way up through `atmosDecodeHook`, failing the entire
  config load rather than being scoped to the one offending step.
- For the mapping form, `mapstructure` decoded the struct's fields directly
  off its own `mapstructure` tags (which happen to exist on
  `WorkflowContainer` for other reasons) without error, but this only looks
  correct by coincidence -- it bypasses `WorkflowContainer.UnmarshalYAML`
  entirely.

Even with decoding fixed, a second gap remained: `cmd/cmd_utils.go`'s
custom-command step-execution loop dispatched every `type: shell` step
straight to `process.RunShellStep` on the host and never consulted
`Task.Container` at all. The actual sandbox session/merge logic
(`workflowPkg.StepContainerOverride`, `workflowPkg.RunStepContainerOverride`,
`pkg/workflow/container.go`) was only ever called from
`internal/exec/workflow_utils.go`, the workflow-file execution path.

Custom commands have no ambient command-level `container:` block (unlike
`schema.WorkflowDefinition.Container` for workflow files -- no such field
exists on `schema.Command`), so a step-level override is always merged
against a nil base (`mergeWorkflowContainer(nil, override)` returns the
override itself), meaning a step's own `container:` mapping is a
self-contained sandbox spec.

A third, independent gap surfaced only once (1) and (2) above were fixed:
`processCustomCommandsWithWorkingDirectory` (`cmd/cmd_utils.go`) calls
`cloneCommand` on each `schema.Command` before capturing it in a Cobra
command's closure, to give each command its own mutable copy (working
directory resolution, etc. mutate the clone in place per command, and
siblings must not see each other's mutations). `cloneCommand` implements this
with a generic `json.Marshal`/`json.Unmarshal` round-trip. `WorkflowContainer.
Enabled` carried `json:"-"` (alongside `yaml:"-"` and `mapstructure:"-"`)
because it's normally populated only by `UnmarshalYAML`'s polymorphic
bool-or-mapping decode, not by any tag-based decoder -- but that also means a
generic JSON round-trip silently drops it, and `IsEnabled()` treats a nil
`Enabled` as *enabled*, inverting a `container: false` opt-out into "run
inside a container" once the clone's JSON round-trip erased the marker.

## Changes

- `pkg/schema/task.go`: `decodeTaskFromMap` now pulls the `container:` key
  out of the map before the `mapstructure` decode runs (the same treatment
  `with:` already got), then replays the polymorphic decode via a new
  `decodeTaskContainerFromMapValue` helper, which round-trips the value
  through YAML and calls `WorkflowContainer.UnmarshalYAML` directly -- the
  same method the workflow-file path relies on. The YAML round-trip itself
  was extracted out of `decodeStepWithFromMapValue` into a shared
  `yamlNodeFromMapValue` helper so `with:` and `container:` can't drift
  apart on how they bridge a plain Go value into a `*yaml.Node`.
- `cmd/cmd_utils.go`: the custom-command step-execution loop's `"shell"` case
  now converts the step to a `WorkflowStep` and checks
  `workflowPkg.StepContainerOverride`; when the step has an enabled
  `container:` override, it calls `workflowPkg.RunStepContainerOverride` --
  the exact function the workflow-file path calls for the identical
  scenario -- instead of falling through to the host-only
  `process.RunShellStep` path. The `WorkflowDef` passed is an empty
  `*schema.WorkflowDefinition{}` since custom commands have no ambient
  command-level container block to merge against.
- `pkg/schema/workflow.go`: `WorkflowContainer` gained `MarshalJSON`/
  `UnmarshalJSON` methods (backed by a `workflowContainerJSON` mirror struct
  with an explicit `"enabled"` key) so the type round-trips losslessly
  through JSON -- the same guarantee `UnmarshalYAML` already provides for
  YAML -- fixing `cloneCommand`'s silent loss of the `Enabled` opt-out
  marker.

## Validation

- New regression tests, reproduced through real production paths, matching
  this repo's existing convention for this bug class:
  - `TestContainerStepOverride_WorkflowAndCustomCommandDecodeIdentically`
    (`pkg/schema/task_test.go`) -- decodes a step's `container:` block (both
    the mapping form and the boolean opt-out form) once via `yaml.Unmarshal`
    (the workflow-file path) and once via `mapstructure`+`TasksDecodeHook`
    (the custom-command path), asserting the resulting `*WorkflowContainer`
    values are equal.
  - `TestCustomCommandContainerStepBoolOptOutDoesNotBreakConfigLoad` and
    `TestCustomCommandContainerStepMappingOverrideDecodesFully`
    (`pkg/config/custom_command_container_override_test.go`) -- write a real
    `atmos.yaml` and load it via `InitCliConfig`, asserting the whole config
    loads successfully and `Container` decodes fully.
  - `TestCustomCommandStepContainerOverrideRunsInsideContainer` and
    `TestCustomCommandStepContainerFalseOptOutRunsOnHost`
    (`cmd/custom_command_container_override_test.go`) -- write a real
    `atmos.yaml`, load it via `InitCliConfig`, register it via
    `processCustomCommands`, and execute the custom command through
    `RootCmd.Execute()` with a fake logging `docker` executable on `PATH`
    (`tests/testhelpers.InstallFakeContainerRuntime`), asserting a real
    `docker create ...` + `docker exec ... /bin/sh -lc "..."` invocation for
    the override case (the sandbox is created, then the step's command is
    execed against it -- the same create/start/exec sequence
    `RunEphemeralContainer` always uses) and a host-only execution (no
    docker invocation) for the `false` opt-out case.
  - `TestWorkflowContainerJSONRoundTripPreservesEnabled` and
    `TestWorkflowContainerJSONRoundTripEnabledOmittedWhenNil`
    (`pkg/schema/workflow_container_test.go`) -- directly exercise
    `json.Marshal`/`json.Unmarshal` on a `*WorkflowContainer` (the mechanism
    `cloneCommand` uses), asserting `Enabled` survives the round-trip in both
    the `false` and unset cases.
- All seven confirmed failing against the pre-fix code (whole-config decode
  error for the boolean form; no docker invocation recorded for the mapping
  form; `Enabled` nil after a JSON round-trip) and passing post-fix.
- `go build ./...`, `gofumpt` -- clean.
- `go test ./pkg/schema/... ./pkg/workflow/... ./cmd/... ./internal/exec/...`
  (full packages, not just the new tests) -- all pass, including the
  existing `with:` fix's tests
  (`TestCustomCommandContainerBuildStepDecodesWithBlock`,
  `TestCustomCommandContainerBuildPassesWithBlockToDocker`,
  `TestContainerStepWithBlock_WorkflowAndCustomCommandDecodeIdentically`),
  confirming no regression.

## Scope notes / follow-ups

- This fix covers `type: shell` (the default) steps, matching the exact
  symptom reported. `type: script` steps have the identical gap in
  `cmd/cmd_utils.go` (workflow files special-case `script` the same way
  `shell` is special-cased in `internal/exec/workflow_utils.go`, outside the
  registered `pkg/runner/step` handler, which has no ambient-container
  concept at all); fixing that is a natural, low-risk follow-up but was not
  reported and is left out of scope here.
- An *ambient* command-level `container:` block (a `schema.Command`-level
  sibling to `schema.WorkflowDefinition.Container`, so every step in a
  command can share one sandbox the way workflow steps can) does not exist
  today -- there is no schema field for it. Adding one is a larger, separate
  feature (new schema field, JSON schema regen, Docusaurus docs) and was not
  part of the confirmed repro for this bug; the step-level override fixed
  here does not require it, since a step's own `container:` mapping is a
  self-contained spec even with no ambient block to merge against.
