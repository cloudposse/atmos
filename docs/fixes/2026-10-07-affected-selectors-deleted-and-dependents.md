# Fix: `--tags` / `--labels` now apply to deleted components and nested dependents in `describe affected`

**Date:** 2026-10-07

## Summary

The `--tags` and `--labels` selectors added to `atmos describe affected` and `atmos list affected` filtered components that exist in `HEAD` but let two groups through unfiltered: components deleted in `HEAD` (including every component of a deleted stack), and the nested `dependents` lists produced by `--include-dependents`. A privileged component labeled `ci: manual` could therefore still reach the `--labels=ci=auto` matrix. Both paths now honor the selectors, and the documentation was corrected where a field test found it wrong or incomplete.

## Context

A hands-on field test of the selectors ran against a throwaway git repository with a base and a head commit. With `--labels=ci=auto`, the matrix still contained `prod/iam` (a whole deleted stack) and `dev/gone` (a deleted component), both labeled `ci: manual`. The same run with `--include-dependents` nested the `ci: manual` component `dev/iam` under `dev/vpc`. `atmos list affected` shared the deleted-component leak. `terraform plan --affected --labels=ci=auto` was not affected, because it drops deleted entries and applies its own tag and label narrowing after dependents are added.

Two other items from the same pass turned out to be wrong or by design and were not changed in code:

- An empty `--labels=` or `ATMOS_LABELS=` selects everything. That is how an empty `--labels`, `--tags`, `--components` and `--query` already behave across Atmos (`terraform --all`, `list components`), so the command stays consistent and the docs now say so.
- A templated label appeared to match nothing. That was a test-fixture artifact: the scratch `atmos.yaml` never set `templates.settings.enabled: true`. With templates enabled the label renders and matches, in both `describe affected` and `terraform --all`.

## Changes

- `detectDeletedComponents` and its helpers take an `AffectedFilter`. A deleted component is matched against its `metadata` as it was in the base ref, with the same rules as the live path (tags match any, labels match all, and a component with no `metadata` is excluded when a selector is set). `ExcludeLocked` still does not apply to deleted items.
- `filterAffectedDependents` runs after dependents are added and prunes the nested tree recursively. A matching descendant of a removed dependent is promoted to the nearest remaining ancestor, so an affected `ci: auto` component is not lost. `included_in_dependents` is recomputed. `schema.Dependent` gained a non-serialized `Metadata` field, filled from the already-loaded stacks, to support this.
- Each dependent's `metadata` is recorded only when a selector is set. `addDependentsToAffected` keeps its signature and behavior and delegates to `addDependentsToAffectedWithFilter`, which `Execute` uses when `--tags` or `--labels` is present, and the transient `Metadata` is cleared after pruning. The first version recorded it unconditionally, which added a populated field to every dependent and failed the whole-struct comparisons in `TestDescribeAffectedWithDependents`, `...WithoutTemplates` and `...FilteredByStack` on CI.
- `terraform --affected` is unchanged: it never reaches this code and still clears `Tags` and `Labels` before describing.
- Tests: deleted component, deleted stack and empty-stack cases, tags and labels, `ExcludeLocked`, `deletion_type` preserved, recursive dependents pruning and promotion, `included_in_dependents`, end-to-end matrix and dependents runs, and `errors.Is(err, ErrInvalidFlag)` for the `--upload` rejection.
- Docs, blog post and the `atmos-ci` agent skill:
  - `schemas.opa.base_path` is required for the documented OPA backstop whenever an `atmos.yaml` exists.
  - An empty selector value applies no filter, with a shell guard (`--labels="ci=${CI_LABEL:?}"`).
  - Deleted-component and dependents behavior is described.
  - Templates in labels and tags are rendered when `templates.settings.enabled` is `true`.
  - `list affected` now documents `--tags` and `--labels`.

## Validation

- `go build ./...` and `go vet` on `internal/exec`, `pkg/schema`, `pkg/list` and `cmd/...`: pass.
- `go test ./internal/exec/ -run 'Affected|Deleted|Dependents|Selector' -count=1`, and `go test` on `./cmd/`, `./cmd/list/`, `./pkg/list/`, `./cmd/terraform/`, `./pkg/schema/`: pass.
- `gofumpt -l` on the changed Go files: clean. `atmos lint --changed`: 0 issues (run by the implementing agent).
- Live, with a rebuilt binary against the throwaway repository:
  - `--labels=ci=auto` returns `dev/vpc` and the deleted `prod/vpc`; `--labels=ci=manual` returns `dev/iam`, deleted `prod/iam` and deleted `dev/gone`.
  - `--tags=privileged` returns `dev/iam` and deleted `prod/iam`; `--format=matrix --labels=ci=auto` returns `dev/vpc` and `prod/vpc` only.
  - `--include-dependents --labels=ci=auto` nests only `dev/app` under `dev/vpc`.
  - `atmos list affected --labels=ci=auto` returns `dev/vpc` and the deleted `prod/vpc`.
  - `terraform plan --affected --labels=ci=auto --dry-run` plans only `vpc`.
- Website build passes. The only warning is a pre-existing broken anchor on `/cli/commands/scaffold/validate`.
- The `TestDescribeAffectedWithDependents*` tests skip inside a git worktree because go-git rejects the `worktreeconfig` extension, so they only run in a normal clone. They failed on CI for the first version of the dependents change, and they were reproduced and verified in a local clone: the committed version fails and the fix passes, along with the rest of the affected tests (157 pass, 3 skip).
- Not run: the gomonkey-based `TestGetAffectedComponents`, which is skipped on darwin/arm64, so it is covered only by CI. `list affected --include-dependents` was not run live.

## Follow-ups

None.

Two items found during the same pass were fixed in this pull request rather than deferred:

- Setting `ATMOS_SCHEMAS_OPA_BASE_PATH` (or `ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH`, `ATMOS_SCHEMAS_CUE_BASE_PATH`, `ATMOS_SCHEMAS_ATMOS_MANIFEST`, or the matching `--schemas-*` flags) when `atmos.yaml` has no `schemas:` block panicked with `assignment to entry in nil map`. This predates the selectors work. `AtmosConfiguration` already had a nil-safe `SetSchemaRegistry` setter, but the eight write sites in `pkg/config/utils.go` still assigned into the map directly. A new `SetResourcePath` setter now covers the `jsonschema`, `opa` and `cue` entries, and all eight sites use the nil-safe setters. Regression tests fail with the original panic before the change and pass after it, and the original `validate component` run with the environment variable no longer crashes.
- The `atmos describe affected --help` and `atmos list affected --help` screengrabs were stale after the new flags. Only those two casts were regenerated, with `atmos --chdir=demo/casts casts generate screengrabs cli --filter "<command>"`, and the manifest's own validation passed.
