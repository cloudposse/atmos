# Fix: generated atmos.yaml schema now accepts the `dependencies.commands`/`.workflows` string shorthand

**Date:** 2026-08-06

## Summary

The generated JSON Schema for `atmos.yaml` (`pkg/datafetcher/schema/atmos/config/1.0.json`)
rejected the plain-string shorthand form of `dependencies.commands`/`dependencies.workflows`
entries (e.g. `dependencies: {commands: [compile]}`), even though `schema.UnitDependencies`'
custom `UnmarshalYAML` has always accepted it (a bare string is equivalent to `{name:
<value>}`). Every real config using this shorthand -- including
`examples/task-runner-dependencies/atmos.yaml` -- failed `TestGeneratedSchemaAcceptsRealConfigs`
in CI on both the macOS and Linux acceptance-test jobs.

## Context

`pkg/config/schema/overrides.go`'s `applyPolymorphicOverrides` is the manually-curated list of
places where a Go field's custom decode logic accepts more shapes than reflection alone can see
(the file's own doc comment: "new configuration fields ... must be classified here before the
build goes green"). It already had the equivalent entry for `Tasks` (custom command steps' plain
shell-string shorthand), added when that feature shipped -- but no analogous entry was ever added
for `UnitDependencies` when this branch's dependency-graph feature introduced its own,
structurally identical, string-or-object polymorphism. The generated schema only ever saw the
object form via reflection, so a bare dependency name failed `anyOf` validation.

## Changes

- `pkg/config/schema/overrides.go`: added an `applyPolymorphicOverrides` entry for
  `UnitDependencies`, mirroring the existing `Tasks` entry exactly -- its `Items` schema gains an
  `anyOf` branch accepting a plain string.
- Regenerated `pkg/datafetcher/schema/atmos/config/1.0.json` via `go generate
  ./pkg/config/schema/...` (a 3-insertion/2-deletion diff: the new `UnitDependencies` `$defs`
  entry's `items` gained the string alternative).
- Confirmed out of scope: `pkg/datafetcher/schema/atmos/manifest/1.0.json` and the stack-config
  schema have no `UnitDependencies` reference at all -- `dependencies.commands`/`.workflows` are
  `atmos.yaml`-only fields (on `schema.Command`/`schema.WorkflowDefinition`), not part of stack
  manifests, so neither needed a corresponding fix.

## Validation

- `go test ./pkg/config/schema/...`: all 16 tests pass, including
  `TestGeneratedSchemaAcceptsRealConfigs` (the corpus test that was failing in CI) and
  `TestGeneratedSchemaCompiles` (the drift/compile guard).
- `go build ./...` and `atmos lint --changed`: clean.

## Follow-ups

None.
