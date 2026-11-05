# Fix: Harden `stack create`, `stack deploy`, and `stack destroy` after a field test

**Date:** 2026-10-19

## Summary

A hands-on field test of the new stack lifecycle commands found selections that
silently changed, a Helmfile handler that never ran, custom component kinds that
vanished, destroys that could strand consumers, and `stack create` runs that wrote
manifests which broke every later `describe stacks`. This change fixes those
findings, plus three scaffold bugs shared with `scaffold generate`.

## Context

The field test ran real Terraform operations with a local backend against the
`tests/fixtures/scenarios/stack-lifecycle-field-test` fixture, which mixes
Terraform, Helmfile, Packer, and a custom component kind across stacks.

Highest-severity findings:

- Environment variables bound by other commands (`ATMOS_TERRAFORM_DESTROY_ALL`,
  `ATMOS_ALL`, `ATMOS_INCLUDE_DEPENDENCIES`) changed what `stack deploy` selected,
  because the command read the shared global configuration registry.
- With `ATMOS_DRY_RUN=true` exported, a real `stack destroy` reported success while
  every child ran as a dry run.
- The Helmfile deploy handler named a `helmfile deploy` command that does not
  exist, so Helmfile components were always excluded.
- Custom component kinds never reached the inventory, because describe-stacks
  only processes built-in kinds.
- Destroying a stack removed prerequisites that components in other stacks still
  required.
- `stack create` accepted missing imports, unresolvable stack names, stack-name
  collisions, and invalid matrix outputs, then wrote files that broke the project.

## Changes

`stack deploy` and `stack destroy`:

- Parse flags with a private configuration registry, so only the command's own
  environment variables apply, and strip `ATMOS_DRY_RUN` from child processes.
- Map Helmfile deploy to `helmfile apply`. A test resolves every built-in handler
  against the real command tree.
- Register custom component kinds and describe their sections before planning.
  Selection by `--affected` remains limited to built-in kinds.
- Fail before execution when destroy would strand a consumer outside the
  selection, when a required prerequisite has no handler, or when concurrent
  operations would need interactive approval.
- Exclude locked components with a notice, print readable cycle paths, validate
  each flag separately, reject comparison flags without `--affected`, reject
  unknown stacks and positional arguments, and normalize disabled identities.
- Report each outcome with success, error, or warning styling and a tally. Route
  grouped child stdout to stdout, and drop the redundant exit-status wrapper.
- Add `--ci`, which passes `ATMOS_CI=true` to every component command. Keep GitHub
  workflow commands such as `::add-mask::` at column 0 when concurrent output is
  prefixed, so masking and annotations still apply. Give each component its own
  invocation log group, and publish one combined job summary with namespaced
  outputs instead of letting component outputs overwrite each other.

`stack create`:

- Before any hook or write, process the existing manifests together with every
  rendered stack file, and validate every generated stack file against the schema.
- Fail when a template ignores `--name`, `--import`, or `--component`. Share strict
  `--set` parsing with `scaffold generate`, reject `--ref` for non-Git templates,
  and keep commas in repeatable values.
- Give existing destinations a create-specific error, replace stale generation
  records, list every created file, quiet the planning output on real runs, and
  separate files in dry-run output.
- Resolve a relative `--file` from the project base path and configured template
  sources from the directory containing `atmos.yaml`.

Shared scaffold behavior:

- Scaffold hooks receive `ATMOS_HOOK_STATUS`, `ATMOS_HOOK_EXIT_CODE`, and
  `ATMOS_HOOK_ERROR` through the shared outcome builder.
- Step hook output is labelled with the hook name instead of the step type.
- Copied files that contain template actions without the `atmos:template` marker
  produce a warning, and YAML failures name the file and the marker.
- Existing-file errors show the rendered path, `atmos init` hides stack-only
  templates, and summaries pluralize "file".

## Validation

- Unit tests cover each finding, including fixture-based plan tests in
  `cmd/stack/lifecycle_fixture_test.go`.
- `go build ./...` passes. Package tests pass for `cmd/stack`, `cmd/helmfile`,
  `cmd/init`, `cmd/scaffold`, `cmd/terraform/...`, `pkg/stack/...`,
  `pkg/generator/...`, `pkg/hooks/...`, `errors`, and describe-stacks tests in
  `internal/exec`.
- golangci-lint reports no findings in changed files.
- Every field-test repro was re-run against a disposable copy of the fixture with
  real local-backend Terraform. The dev deploy runs in dependency order with hook
  outcomes, the custom script component runs, destroy refuses to strand
  `terraform/prod/app` and tears down in reverse order with `--include-dependents`,
  and every failing `stack create` case leaves `describe stacks` working.
- `pkg/runner/step` cast tests time out locally on both this branch and its
  unmodified parent commit, so they are environment-related and unaffected here.

## Follow-ups

Remaining work stays in the
[stack lifecycle PRD](../prd/stack-lifecycle.md): `--affected` selection for
custom component kinds, and deleted-definition recovery for `--allow-destroy`.
