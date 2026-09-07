# Fix: container step `with:`/`driver:` blocks now reject unknown fields

**Date:** 2026-08-08

## Summary

A typo'd or nonexistent field inside a `type: container` step's `with:`
block (e.g. `platforms:` -- a plausible Docker-Compose-style name that isn't
a field on any of `ContainerBuildStep`/`ContainerRunStep`/`ContainerPushStep`/
`ContainerInspectStep`) was silently dropped: no error, no warning, and the
field simply had no effect. The JSON Schema used for `commands:`/workflow
autocomplete (`pkg/datafetcher/schema/atmos/manifest/1.0.json`) already
documents `with:` as "validated by the step handler at run time" -- but
nothing in the runtime ever fulfilled that promise for unknown *keys* (only
known fields' *values* were validated). This surfaced a real, pre-existing
test bug: `TestWorkflowStep_DecodeWith`'s "explicit push action" case used
`tag: v1` (singular) instead of the real field `tags: []string`, and passed
regardless because the typo was silently discarded.

## Context

Both loading paths -- standalone workflow files (`WorkflowStep.UnmarshalYAML`)
and custom commands (`Task`'s `decodeTaskFromMap` via
`decodeStepWithFromMapValue`) -- funnel a container step's `with:` block
through the same shared `decodeContainerWith` -> `decodeYAMLInto` functions
(`pkg/schema/workflow.go`), which is exactly the design the sibling `with:`-
block-dropped fix (`docs/fixes/2026-08-05-custom-command-container-with-
block-dropped.md`) put in place so the two paths can't drift apart. Until
now, `decodeYAMLInto` called plain `node.Decode(&cfg)`. `yaml.Node.Decode` has
no strict/unknown-field-rejection mode -- `KnownFields(true)` only exists on
the stream-level `yaml.Decoder` (`yaml.NewDecoder(r).KnownFields(true)`), not
on an already-parsed `*yaml.Node`'s own `Decode` method -- so there was no way
for this call site to reject an unknown key even though the intent (per the
JSON Schema's own docstring) was clearly to have the step handler enforce it.
The same gap existed one level deeper: `ContainerDriverConfig.UnmarshalYAML`
(the `driver:` sub-block's custom decode for the bare-string-or-mapping
shape) called its own `value.Decode(&decoded)`, bypassing any strictness the
parent decode might otherwise have applied even if it had one.

Repo-wide, neither `KnownFields` nor mapstructure's `ErrorUnused` was used
anywhere (confirmed via `grep -rn`), so this wasn't a case of one loading path
being stricter than the other -- both were equally permissive, by omission
rather than design.

## Changes

- `pkg/schema/workflow.go`: added `decodeYAMLKnownFields(node *yaml.Node, dst
  any) error`, which re-marshals `node` back to YAML bytes and decodes it
  through a fresh `yaml.NewDecoder(...).KnownFields(true)`, since a
  stream-level decoder is the only place go-yaml exposes strict decoding.
  `decodeYAMLInto` (used by `decodeContainerWith` for all four container
  action structs) and `ContainerDriverConfig.UnmarshalYAML`'s mapping branch
  (the nested `driver:` block) both now call this helper instead of plain
  `node.Decode`/`value.Decode`. This is scoped narrowly to the typed
  container structs -- the generic `with: map[string]any` fallback other step
  types use (`decodeStepWith`'s non-container branch) is untouched and stays
  fully permissive, since it has no fixed field set to validate against.
- `pkg/schema/workflow_with_test.go`: fixed `TestWorkflowStep_DecodeWith`'s
  "explicit push action" case, which used the nonexistent field `tag: v1`
  instead of the real `tags: []string` field -- this typo only passed because
  of the bug being fixed here, and now correctly fails without the fix.

## Validation

- New regression tests, confirmed failing pre-fix and passing post-fix, for
  both loading paths (mirroring the existing
  `TestContainerStepWithBlock_WorkflowAndCustomCommandDecodeIdentically`
  pattern):
  - `TestContainerStepWithBlock_RejectsUnknownField`
    (`pkg/schema/task_test.go`) -- a `platforms:` field in a build step's
    `with:` block errors and names the field, for both the workflow-file
    (`yaml.Unmarshal`) and custom-command (`mapstructure` +
    `TasksDecodeHook`) paths.
  - `TestContainerStepWithBlock_RejectsUnknownNestedDriverField`
    (`pkg/schema/task_test.go`) -- a typo'd field nested inside `driver:`
    (a field with its own custom `UnmarshalYAML`) is also rejected, not just
    top-level `with:` keys.
- Fixing the pre-existing `tag: v1` typo in `TestWorkflowStep_DecodeWith`
  surfaced as a real, expected failure once the fix landed (`field tag not
  found in type schema.ContainerPushStep`) -- corrected to `tags: [v1]` and
  now asserts on the decoded value.
- Audited every real, tracked YAML fixture in the repo using `type:
  container` (`examples/container-step/atmos.yaml`,
  `examples/container-step/workflows/container-step.yaml`,
  `examples/background-steps/workflows/background.yaml`) -- all `with:`
  blocks use only real fields; none regress under strict decoding.
- `go build ./...`, `gofumpt -l`, `./custom-gcl run --new-from-rev=origin/main`
  -- all clean.
- `go test ./pkg/schema/... ./pkg/workflow/... ./cmd/... ./internal/exec/...
  ./pkg/config/... ./pkg/runner/step/...` (full packages, not just changed
  tests) -- all pass, no regressions beyond the one pre-existing test typo
  this fix itself surfaced and corrected.

## Follow-ups

- `WorkflowContainer.UnmarshalYAML` (the top-level `container:` sandbox
  block, both workflow-level and the step-level override fixed in
  `docs/fixes/2026-08-07-custom-command-container-block-dropped.md`) has the
  same class of gap -- a typo'd field there is also silently dropped. Left
  out of scope here since it wasn't part of the reported symptom; a natural,
  low-risk follow-up using the same `decodeYAMLKnownFields` helper.
