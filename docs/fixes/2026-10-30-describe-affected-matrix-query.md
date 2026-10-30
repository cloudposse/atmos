# `describe affected --format=matrix` Silently Ignored `--query`

**Date:** 2026-10-30

## Summary

`atmos describe affected --format=matrix --query ...` skipped the query and wrote the unfiltered matrix, so CI jobs
ran for components the user had filtered out. A new `--filter` flag selects the affected components for every output
format, and `--query` with the matrix format is now rejected.

## Symptom

With `settings.github.actions_enabled: false` on the only affected component, this printed the component in the
matrix instead of `{"include":[]}`:

```shell
atmos describe affected --include-settings \
  --query '.[] | select((.settings.github.actions_enabled // false) == true) | [.]' \
  --format=matrix
```

## Root Cause

`describeAffectedExec.view()` handled `--format=matrix` first: it converted the affected list to matrix entries and
returned before the `--query` branch was reached. A query can return any shape, so a matrix cannot be built reliably
from its result.

## Fix

- Added `--filter` (`ATMOS_DESCRIBE_AFFECTED_FILTER`), a YQ predicate evaluated per affected component and per
  dependent. A failing item is dropped with its whole dependents subtree. It is applied before `--flatten`.
- `--query` with `--format=matrix` now fails with `ErrInvalidFlag` and a hint to use `--filter`.
- `--filter` with `--upload` follows the `--tags` / `--labels` rule: an error for the flag, a warning for the
  environment variable.
- Added `YQPredicateFilter.Matches` in `pkg/list/filter`, the per-item building block shared with
  `atmos list instances --filter`.

## Tests

- `internal/exec/describe_affected_predicate_test.go`: regression test for the original report, matrix/json/yaml
  parity, dependents pruning and flattening, `included_in_dependents` recomputation, output files, `--upload`
  rules, and combination with `--labels`.
- `cmd/describe_affected_filter_flag_test.go`: flag registration and environment variable resolution.
- `pkg/list/filter/yq_test.go`: `Matches` truthiness.

## Review Notes

`atmos list affected` has no `--query` or `--filter`, and `atmos list instances` already rejects both with
`--format=matrix`, so neither needed a change.
