# Fix: `describe dependents` retains cross-stack same-name dependents

**Date:** 2026-09-10

## Summary

`describe dependents` now reports a component in another stack when it has the same name as the selected dependency target, without evaluating unrelated stack templates. `list dependencies` now includes cross-type component edges.

## Context

The reverse dependency index identified self-references by component name alone. A cross-stack dependency such as `vpc@prod-use1` depending on `vpc@dev-use1` was therefore discarded as a self-reference. Direct dependent lookup also rendered every stack before identifying reverse candidates, while `list dependencies` silently excluded all non-Terraform component types.

## Changes

- Identify self-references by both stack and component name.
- Add a regression test for a cross-stack same-name dependent.
- Resolve direct reverse lookups through the existing scoped closure engine, rendering only reachable candidates while preserving legacy dependency discovery.
- Build list dependency graphs for every component type, with type-aware graph IDs and dependency-kind target selection.
- Clarify required and optional unavailable-target behavior in the Go documentation and committed JSON Schema copies.
- Consolidate the forward and reverse optional-edge rendering assertions into table-driven subtests.

## Validation

- `go test ./internal/exec -run 'TestFindDependentsFromIndex_(IncludesCrossStackSameNameDependent|SkipsSelfReference)' -count=1`
- `go test ./pkg/list/dependencies -run TestRender_MarksOptionalDependency -count=1`
- `go test ./pkg/config/schema -run 'TestEmbeddedSchemaIsCurrent|TestGenerateExtractsDocCommentsAsDescriptions' -count=1`
- `go test ./pkg/datafetcher -run TestManifestSchema_ComponentDependencyRequiredForms -count=1`
- `go test ./pkg/list/dependencies -count=1`
- `go test ./internal/exec -run 'TestDescribeDependents_WithStacksNamePattern|TestDescribeDependents_DependenciesComponentsInheritance_WithAppendMerge|TestDescribeDependents_ScopesTemplateEvaluationToReverseClosure' -count=1`
- `atmos describe dependents root --stack app-a --format json` against `tests/fixtures/scenarios/dependencies-scoped-evaluation`
- `atmos list dependencies vpc --stack dev --direction reverse --format json` against `tests/fixtures/scenarios/dependencies-components-inheritance`

## Follow-ups

None.
