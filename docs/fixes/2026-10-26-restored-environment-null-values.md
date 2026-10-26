# Fix: Preserve null filtering when refreshing component environments

**Date:** 2026-10-26

## Summary

Refreshing component sections after evaluation or literal restoration continues to
omit null environment values from the execution environment.

## Context

Initial component configuration loading copied the `env` section and excluded
null values. The subsequent section refresh replaced that filtered copy with the
original map. Execution paths that formatted the refreshed map directly could
therefore produce assignments such as `TOKEN=<nil>`. Sharing the map also allowed
execution-time environment mutations to change the component configuration view.

## Changes

- Share the existing null-filtering logic between initial extraction and section
  refresh, returning a detached map in both cases.
- Preserve empty strings, `false`, zero, and restored literal strings.
- Keep source configuration and caches unchanged, including when template and
  YAML-function processing are disabled. OS environment inheritance is unchanged.

## Validation

The new regression failed before the fix in all four combinations of template and
YAML-function processing: the refreshed map retained `TOKEN: null`, and execution
conversion emitted `TOKEN=<nil>`. The focused regression suite verifies repeated
stack processing, execution-list contents, literal preservation, and isolation
between execution maps, component sections, and cached source data. Existing
literal, provenance, section-refresh, and environment-conversion tests are included
in the focused validation run, which passed:

```shell
go test ./internal/exec -run 'Test(ProcessStacksRestoredEnvironmentOmitsNulls|PostProcessEnvironmentNullFilteringIsolation|PostProcessTemplatesAndYamlFunctions|StackLiteral|DescribeStacksLiteral|RestoreConfigSourceLiterals|ComponentEnvSectionConversion)' -count=1
```

The fix-record validator and `git diff --check` passed.

## Follow-ups

None.
