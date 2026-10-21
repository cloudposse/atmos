# Fix: Inherited maps are rendered when a YAML function overrides them

**Date:** 2026-10-21

## Summary

When a component sets a map-valued field to a YAML function that returns a map, such as `vars.tags: !labels`, the map it inherits from imported defaults reached the output with its Go templates and Git YAML functions unrendered. The component's tags showed `{{ .atmos_component }}` and `!git.repository unknown` as literal text. A plain-map override rendered the same inherited values correctly.

## Context

Overriding a field with a deferred YAML function (`!labels`, `!tags`, `!template`, `!terraform.output`, and others) keeps the lower-precedence inherited value as a separate concrete layer. The function resolves later, and the layers are deep-merged afterwards.

The document-wide Go template pass and YAML function pass run before that merge. At that point the field holds only the function string, so neither pass sees the inherited map. The deferred-merge resolver then merged the inherited layer back in without rendering it. Functions that the merge defers itself, such as `!exec` and `!env`, were not affected. Git functions such as `!git.repository` and plain Go templates were.

The bug was found while building the tagging strategy tutorial, where an organization-level `vars.tags` map holds `{{ .atmos_stack }}`, `{{ .atmos_component }}`, and `!git.repository`, and a component bridges `metadata.labels` into `vars.tags` with `!labels`. It reproduces on `main`.

## Changes

- `pkg/deferred/concrete.go` adds `ResolveConcreteLayers`, which resolves the template and YAML function strings inside the concrete map and list layers of every deferred path, and `ConcreteStringFilter`, which selects those strings. It returns resolved copies and never mutates the maps held by the deferred context, because the context can share them with a cache.
- `internal/exec/deferred_contexts.go` calls `ResolveConcreteLayers` in `resolveDeferredYamlFunctions` before `ApplyDeferredMerges`. When templating is unavailable for the invocation, only YAML function strings are resolved and templates stay as written, matching the whole-document pass.
- `tests/fixtures/scenarios/atmos-labels-inherited-tags/` and `tests/labels_inherited_tags_test.go` reproduce the bug end to end and pin the plain-map behavior as a negative path.
- `pkg/deferred/concrete_test.go` covers nested maps, lists, isolation in both directions, scalar and function layers, templating unavailable, custom delimiters, and processor errors.
- `examples/tagging-strategy` bridges `metadata.labels` into `vars.tags` with `!labels` in the same stack hierarchy as the other components.

## Validation

- `go test ./tests -run TestLabelsOverrideRendersInheritedTags` failed before the change, with `{{ .atmos_stack }}`, `{{ .atmos_component }}`, and `!git.repository unknown` left in the tags, and passes after it.
- `go test ./pkg/deferred ./pkg/merge` and `go test ./internal/exec -run 'Deferred|deferred|Labels|YAMLFunction|DescribeComponent'` pass. `go test ./tests -run 'Label|YAMLFunction|YamlFunction|Deferred|InheritedTags'` passes.
- `pkg/deferred/concrete.go` has 100% statement coverage except one branch in `resolveContainer` (89.5%).
- `atmos describe component app-labeled -s null-label-dev` in `examples/tagging-strategy` now returns the inherited tags rendered alongside the `!labels` result, and `atmos test` in that directory passes.
- `atmos lint --changed` reports 0 issues.
- The full `atmos test --full` suite was not run.

## Follow-ups

None.
