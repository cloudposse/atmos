# Fix: Restore literal provenance and stack naming context

**Date:** 2026-10-26

## Summary

Stack descriptions restore protected `!literal` values in typed provenance, and
`stacks.name_pattern` derives stack names and context from ordinary literal values.

## Context

The stack component processor restored ordinary maps after evaluation but did not
traverse `schema.ConfigSources`. When typed provenance was present, `FinalValue`
and dependency `VariableValue` fields retained internal literal envelopes and
shared mutable data with their inputs. Separately, `ResolveStackName` passed
protected variables directly to context extraction, producing encoded stack names
and causing logical stack selectors to return no components.

## Changes

- Restore typed provenance with the existing copying helper after all evaluators
  finish, including when template or YAML-function processing is disabled.
- Restore a copy of variables before deriving naming context, preserving the
  protected source values for later evaluation and cache reuse.
- Cover nested provenance, dependency slices, repeated descriptions, logical stack
  selection, Terraform workspace names, and context returned on filename fallback.

## Validation

Both regressions failed before the production change. The focused regression suite
passed:

```shell
go test ./internal/exec -run 'Test(DescribeStacksLiteral|ResolveStackName|StackLiteral|RestoreConfigSourceLiterals|ProcessComponentEntry_DoesNotEvaluateConfigSources|ProcessStacks_DoesNotEvaluateConfigSources)' -count=1
```

The tests exercise all four template/YAML-function processing combinations and
verify that mutating returned provenance leaves source caches unchanged. Scoped
custom lint (`custom-gcl run --config=.golangci.yml --new-from-rev=origin/main
./internal/exec/...`) reported zero issues. The fix-record validator and
`git diff --check` passed.

## Follow-ups

None.
