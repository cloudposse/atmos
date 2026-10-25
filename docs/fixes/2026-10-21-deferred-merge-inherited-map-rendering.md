# Fix: Inherited maps are rendered when a YAML function overrides them

**Date:** 2026-10-21

## Summary

When a component sets a map-valued field to a YAML function that returns a map, such as `vars.tags: !labels`, the map it inherits from imported defaults reached the output with its Go templates and Git YAML functions unrendered. The component's tags showed `{{ .atmos_component }}` and `!git.repository unknown` as literal text. A plain-map override rendered the same inherited values correctly.

## Context

Overriding a field with a deferred YAML function (`!labels`, `!tags`, `!template`, `!terraform.output`, and others) keeps the lower-precedence inherited value as a separate concrete layer. The function resolves later, and the layers are deep-merged afterwards.

The document-wide Go template pass and YAML function pass run before that merge. At that point the field holds only the function string, so neither pass sees the inherited map. The deferred-merge resolver then merged the inherited layer back in without rendering it. Functions that the merge defers itself, such as `!exec` and `!env`, were not affected. Git functions such as `!git.repository` and plain Go templates were.

The bug was found while building the tagging strategy tutorial, where an organization-level `vars.tags` map holds `{{ .atmos_stack }}`, `{{ .atmos_component }}`, and `!git.repository`, and a component bridges `metadata.labels` into `vars.tags` with `!labels`. It reproduces on `main`.

## Changes

- `pkg/deferred/concrete.go` adds `WithConcreteResolution`, which wraps unresolved strings inside concrete map and list layers as lazy scalar values. `ApplyDeferredMerges` first resolves the competing functions and merges the layers, then renders only the inherited strings that survive. Replaced map leaves and lists are never evaluated, and already resolved function results are not evaluated again. The wrapper copies containers to protect cached contexts.
- `internal/exec/deferred_contexts.go` decorates the YAML processor with `WithConcreteResolution` in `resolveDeferredYamlFunctions`. When templating is unavailable for the invocation, only YAML function strings are resolved and templates stay as written, matching the whole-document pass.
- `internal/exec/describe_stacks_component_processor.go` passes a nil template context to the deferred resolver when template processing is off, as `processStacks` does. The `list` commands pass `processTemplates` and `processFunctions` independently, so a `false, true` invocation previously rendered inherited strings that contain template delimiters even though the whole-document template pass had been skipped.
- `tests/fixtures/scenarios/atmos-labels-inherited-tags/` and `tests/labels_inherited_tags_test.go` reproduce the bug end to end and pin the plain-map behavior as a negative path.
- `pkg/deferred/concrete_test.go` covers nested maps, lists, isolation in both directions, scalar and function layers, templating unavailable, custom delimiters, and processor errors.
- `examples/tagging-null-label` bridges `metadata.labels` into `vars.tags` with `!labels` in the same stack hierarchy as the other components.

## Validation

- `go test ./tests -run TestLabelsOverrideRendersInheritedTags` failed before the change, with `{{ .atmos_stack }}`, `{{ .atmos_component }}`, and `!git.repository unknown` left in the tags, and passes after it.
- `go test ./tests -run TestLabelsOverrideLeavesInheritedTemplatesWhenTemplatesDisabled` failed before the describe-stacks change, with `{{ .atmos_stack }}` rendered to `inherited-tags` while templates were off, and passes after it. Its second subtest pins that the same strings still render when templates are on.
- `go test ./pkg/deferred ./pkg/merge` and `go test ./internal/exec -run 'Deferred|deferred|Labels|YAMLFunction|DescribeComponent'` pass. `go test ./tests -run 'Label|YAMLFunction|YamlFunction|Deferred|InheritedTags'` passes.
- `pkg/deferred/concrete_test.go` verifies replaced map leaves, list replacement/append/merge strategies, literal function results, repeated resolution of cached contexts, and errors from surviving values. `TestLabelsOverrideSkipsReplacedInheritedValues` covers both describe paths using real templates and YAML functions.
- `atmos describe component app-labeled -s payments-dev` in `examples/tagging-null-label` now returns the inherited tags rendered alongside the `!labels` result, and `atmos test` in that directory passes.
- `atmos lint --changed` reports 0 issues.
- The full `atmos test --full` suite was not run.

## Follow-ups

None.
