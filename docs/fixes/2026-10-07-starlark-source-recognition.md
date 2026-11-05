# Fix: Recognize only loader-encoded !starlark sources

**Date:** 2026-10-07

## Summary

Only values produced by the YAML loader's `!starlark` tag are now executed as Starlark. Quoted strings and included file text that merely begin with `!starlark` stay plain data. A `!starlark` list item that follows an `!unset` item now resolves instead of leaking its encoded source into `describe component` output.

## Context

Field testing found two defects:

1. `note: "!starlark return 'injected'"` in a stack manifest was executed (result `injected`), and `!include.raw` of a file whose text begins with `!starlark` was executed too. Detection used `starlarksource.Is`, which accepts any string starting with the tag plus a space, tab, or newline.
2. In `[a, !unset, !starlark return "c"]`, the Starlark item was never evaluated and the raw `!starlark\n# atmos-starlark-source: <base64>\n...` text appeared in output with exit code 0. `collectConfigurationSources` keyed list sources by their index in the original list, but `ProcessCustomYamlTags` drops `!unset` items before the resolver walks the compacted list, so the recorded path no longer matched.

## Changes

- `pkg/function/starlarksource/source.go`: added `IsEncoded`, which requires the location header line that `Source.Encode` writes. `Is` remains for the loose over-approximating callers (`yaml_processor.go`, `pkg/deferred/evaluation.go`).
- Switched strict callers to `IsEncoded`: `starlarksource/template.go` (`protectNode`), `internal/exec/yaml_func_starlark.go` (`containsStarlark`, `resolveString`), `internal/exec/yaml_func_starlark_phase.go` (`collectConfigurationSources`), and the `!starlark` prefix check in `processCustomTagsWithContext` (`internal/exec/yaml_func_utils.go`). Without the last change a header-less string recursed forever between `processCustomTagsWithContext` and `resolveString`.
- `internal/exec/yaml_func_utils.go`: factored `isUnsetTagString` and `isUnsetListItem` and reused the former at the existing list `!unset` checks.
- `collectConfigurationSources` now takes the skip list and computes list indexes the way the post-unset list will look.
- Existing in-memory tests that used loose `!starlark ...` strings now build loader-encoded values through `starlarkTestSource`.

## Validation

- `go test ./pkg/function/starlarksource/ -count=1` covers `IsEncoded` (encoded true; `"!starlark return 1"`, `"!starlark\nreturn 1"`, `"!starlarkish"`, `"!STARLARK x"` false).
- `go test ./internal/exec/ -run 'Starlark' -count=1` covers quoted strings staying data and list shapes with `!unset` (before, after, leading, several, and marker items).
- `go test ./tests -run 'TestCLICommands/starlark_values' -count=1` runs the new `tests/test-cases/starlark-values.yaml` cases against the `starlark-steps` fixture.
- `go test ./pkg/function/... ./pkg/utils/ -count=1`.

## Follow-ups

None.
