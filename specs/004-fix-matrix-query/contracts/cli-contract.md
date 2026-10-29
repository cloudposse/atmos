# CLI Contract: `atmos describe affected`

## New flag

| Flag | Env var | Type | Default |
|------|---------|------|---------|
| `--filter` | `ATMOS_DESCRIBE_AFFECTED_FILTER` | string (YQ predicate) | empty (no filtering) |

Precedence: CLI flag, then environment variable, then default.

Help text: `YQ predicate evaluated against each affected component (and each dependent). Items for which it is truthy are kept; an item that fails is dropped together with its dependents. Works with every --format, including matrix.`

## Behavior matrix

| Invocation | Result |
|------------|--------|
| `--format=matrix --filter EXPR`, nothing matches | stdout/output file gets `{"include":[]}`, exit 0 |
| `--format=matrix --filter EXPR`, some match | matrix has exactly the matching items; entry fields unchanged |
| `--format=json\|yaml --filter EXPR` | same items as matrix would have; full item shape; empty list when nothing matches |
| `--format=matrix` (no filter/query) | byte-identical to the current output |
| `--format=matrix --query Q` | **error**, exit non-zero, nothing written |
| `--format=json\|yaml --query Q` | unchanged. When `--filter` is also set, the filter runs first and the query runs on the filtered list |
| `--filter EXPR --tags/--labels` | the result must satisfy both |
| `--filter EXPR --include-dependents` | the filter is evaluated at every depth. A failing item removes its whole dependents subtree; no promotion |
| `--filter EXPR --include-dependents --flatten` | pruning happens before flattening |
| `--filter EXPR --upload` (flag) | **error**, nothing uploaded |
| `ATMOS_DESCRIBE_AFFECTED_FILTER` set, `--upload` | warning, filter ignored, full set uploaded |
| invalid expression | error naming the expression, nothing written |

## Error contract

Both errors use the sentinel `ErrInvalidFlag` through the error builder.

`--query` with matrix:

```text
Error: invalid flag: --query is not supported with --format=matrix
Hint: Use --filter to select which affected components appear in the matrix.
Hint: Example: --filter '.settings.github.actions_enabled // false' --include-settings
```

`--filter` with `--upload`:

```text
Error: invalid flag: --filter is not supported with --upload (the upload is always unfiltered)
Hint: Run `atmos describe affected --upload` without --filter, and apply --filter in a separate `describe affected` step for the matrix.
Hint: To override a filter supplied through ATMOS_DESCRIBE_AFFECTED_FILTER for this run, pass --filter=.
```

(Final wording is set during implementation. The sentinel, the pointer to `--filter`, and the exit behavior are the contract.)

## Matrix schema

Unchanged: `{"include":[{"stack","component","component_path","component_type"}]}`.

## Example (reported scenario)

```shell
atmos describe affected \
  --process-functions=false \
  --identity=false \
  --include-settings \
  --filter '.settings.github.actions_enabled // false' \
  --format=matrix
# -> {"include":[]} when no affected component has settings.github.actions_enabled: true
```
