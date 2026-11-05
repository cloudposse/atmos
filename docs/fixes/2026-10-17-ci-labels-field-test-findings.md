# Fix: `!ci.labels` field-test findings (schema, `list --labels`, errors, docs)

**Date:** 2026-10-17

## Summary

A hands-on field test of `!ci.labels` found four user-visible problems. All are fixed on the branch with
regression tests: the documented `settings.pro.enabled: !ci.labels [...]` failed `validate stacks` and
`describe stacks`, boolean labels were invisible to `list components/stacks --labels`, label errors were
unactionable and misleadingly hinted at Go templates, and `!ci.labels` was missing from the YAML function index
and agent skills.

## Context

- The stack JSON schemas typed `metadata.enabled` and `settings.pro.enabled` as strict booleans, but stack
  validation sees the unevaluated function string, so `!ci.labels [preview]` (and `!env`) was rejected with
  "expected boolean, but got string".
- `pkg/list/extract` had its own `getStringMapFromMetadata` that dropped non-string label values. The PR taught
  `tags.ToStringMap` about booleans, but `list components/stacks --labels preview:true` kept returning nothing,
  while `describe affected` and `terraform --labels` matched.
- Argument errors were wrapped with "rename it to .yaml.tmpl" even though they were not template errors, and
  fetch failures repeated the failure prefix and said nothing about tokens or permissions.
- Quoted scalars such as `!ci.labels "[preview]"` and empty names such as `[""]` were silently accepted.

## Changes

- `pkg/datafetcher/schema/atmos/manifest/1.0.json` and `stack-config/1.0.json`: `metadata.enabled` (component and
  stack-wide) and `settings.pro.enabled` accept a boolean or a string beginning with `!`. Other strings stay invalid.
- `pkg/list/extract/components.go`: `getStringMapFromMetadata` delegates to `tags.ToStringMap`.
- `pkg/ci/labels.go`: label failures are built once with the failure sentinel and carry hints (token variables,
  `pull-requests: read`, merge-queue ref format, `--skip=ci.labels`).
- `internal/exec/stack_processor_utils.go`: the `.yaml.tmpl` hint is not attached to `ErrInvalidArguments`.
- `pkg/function/parser`: a non-empty scalar argument and an empty label name are rejected.
- Docs: `ci.labels.mdx` (argument rules, flow-mapping caveat, validation), the YAML functions index, and the
  `atmos-yaml-functions` and `atmos-ci` skills.

## Validation

- Regression tests were written first and confirmed failing: `TestYAMLFunctionsInBooleanFlagFields`,
  `TestGetLabelsFromMetadata`, `TestStacks_BooleanLabelsFilter`, `TestLabelsErrorsAreActionable`,
  `TestExtractLocalsFromRawYAML_FunctionArgumentErrorHasNoTemplateHint`. The parser cases (quoted scalar,
  empty name) were added with the fix and were not run red first.
- `go test` passes for `pkg/datafetcher`, `pkg/ci`, `pkg/function`, `pkg/list`, `pkg/tags`, `cmd/list`, and
  `tests -run CILabels`.
- Re-ran the field-test fixture (`tests/fixtures/scenarios/ci-labels-adversarial`) against a local mock GitHub
  API: `validate stacks` passes, and `list components --labels preview:true` and `preview:false` list the
  expected components.
- `atmos lint --changed` reports three findings in files this change did not touch
  (`pkg/function/parser/parser.go`, `cmd/aws/cloudformation/backend/backend_create.go`); none are in changed lines.
- Not run: the website build, and the full `internal/exec` suite (only the locals-extraction tests).

## Follow-ups

- A stack-level `locals:` block plus a sibling component calling `atmos.Component` on another component in the
  same stack makes `describe component` spin at very high CPU and memory. It reproduces with plain locals (no
  `!ci.labels`) and was not checked against `main`, so it needs separate triage.
