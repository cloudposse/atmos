# Quickstart / Verification: `--filter` for `describe affected`

## 1. Reproduce the original problem (before the change)

Use a stack with an affected component `test` that has `settings.github.actions_enabled: false`:

```shell
atmos describe affected --include-settings \
  --query '.[] | select((.settings.github.actions_enabled // false) == true) | [.]' \
  --format=matrix
# Before: {"include":[{"stack":"...","component":"test",...}]}   <- the bug
# After:  error, with a hint to use --filter
```

## 2. Use the new flag

```shell
atmos describe affected --include-settings \
  --filter '.settings.github.actions_enabled // false' --format=matrix
# {"include":[]}
```

Same set across formats:

```shell
atmos describe affected --include-settings --filter '.settings.github.actions_enabled // false' --format=json
atmos describe affected --include-settings --filter '.settings.github.actions_enabled // false' --format=yaml
```

## 3. Test plan (written test-first)

Do not use pipe redirection when running tests. Use `atmos test`.

1. **Regression test (must fail first)**: in `internal/exec/describe_affected_predicate_test.go`, call the validation/view path with `Format: "matrix"` and a non-empty `Query`. Before the change it succeeds and emits the unfiltered matrix, so the test that expects an `ErrInvalidFlag` fails. After the change it passes.
2. **Filter unit tests (table-driven)**:
   - matches none → `{"include":[]}`; some → exact items (assert first and last element values); all → identical to unfiltered.
   - json, yaml and matrix return the same item set.
   - empty or unset filter → identity; invalid expression → error and no output.
   - dependents: failing top-level item drops its subtree (including a dependent that would pass); failing nested dependent drops only its branch; flattened result excludes dependents of a filtered-out ancestor.
   - negative path: a passing parent keeps its passing dependents (pruning does not trigger without a failing ancestor).
   - isolation: mutating the result does not change the input forest, and the input is unchanged by filtering.
3. **Guard tests**: `--query` + matrix → `ErrInvalidFlag` with a `--filter` hint and nothing written; `--filter` flag + `--upload` → error; env-only filter + `--upload` → warning and filter dropped; flag beats env var.
4. **Matrix output unchanged**: existing matrix tests pass byte-for-byte; matrix written to an output file (`--output-file` and auto-detected CI output) with a filter writes the filtered matrix.
5. **cmd tests**: use `cmd.NewTestKit(t)`; cover flag registration, env var resolution and the env-source mark.
6. **Predicate package**: `YQPredicateFilter.Matches` truthiness table (true, false, nil, empty string/map/slice, non-empty values) and `Apply` still equals repeated `Matches`.

## 4. Commands

```shell
go build ./... && atmos test
atmos test --full          # before opening the PR
atmos lint --changed
cd website && npm run build
atmos --chdir=demo/casts casts generate screengrabs cli   # regenerate the help recording
```

## 5. Release checklist

- Label `minor`; blog post `website/blog/2026-10-30-describe-affected-filter.mdx`; roadmap entry; `describe-affected.mdx` updated (flag, `--query`/matrix restriction, difference from `--tags`/`--labels` promotion, CI example); fix record in `docs/fixes/`.
