# Fix: `workdir.BuildPath` no longer collides on separator-vs-hyphen names or trusts an unsanitized stack name

**Date:** 2026-08-14

## Summary

`workdir.BuildPath` — "the single canonical formula every workdir-path
consumer uses" — had two independent defects:

1. **Collision**: it replaced `/` and `\` in a component name with a
   single `-`, so `app/local` and `app-local` both resolved to the same
   workdir (`dev-app-local`). `Service.Provision` writes component files,
   metadata, and Terraform state directly into whatever `BuildPath`
   returns, so two entirely distinct components would silently share all
   three.
2. **Traversal**: it sanitized the *component* name but never validated
   the *stack* name, which is concatenated into the same directory name
   (`fmt.Sprintf("%s-%s", stack, component)`). A stack value containing
   enough `../` segments (e.g. `../../../../../../evil`) could resolve a
   path outside `basePath` entirely.

Both `component` and `stack` originate from user-controlled YAML (stack
manifests / custom command config), so both are realistic inputs, not just
theoretical ones.

## Context

Two CodeRabbit review comments on PR #2879, both against
`pkg/provisioner/workdir/types.go` (the collision, lines 159-160) and
`pkg/provisioner/workdir/workdir.go` (the traversal, line 246,
`Service.Provision` → `createWorkdirDirectory`). CodeRabbit's own probes
demonstrated both: a Python collision check showing `app/local` and
`app-local` both producing `dev-app-local`, and a path-join simulation
showing a `../`-laden stack value escaping `basePath`.

Investigating the traversal finding surfaced that it wasn't isolated to
`Service.Provision`. `BuildPath` has six call sites total
(`pkg/provisioner/workdir/workdir.go`, `pkg/provisioner/source/source.go`,
`pkg/provisioner/source/provision_hook.go`,
`pkg/component/workdir_path.go`, `pkg/terraform/output/config.go`,
`internal/terraform_backend/terraform_backend_local.go`). Two of the six
(`config.go`'s `extractComponentPath`,
`terraform_backend_local.go`'s `resolveLocalBackendComponentPath`) already
had their own ad hoc, near-identical "absolutize + check prefix, fall back
to a safe alternative on escape" containment guards — explicitly
cross-referencing each other as a "mirror guard" in their comments. A prior
fix doc
(`docs/fixes/2026-08-06-terraform-output-containment-guard-test-stale-vector.md`)
explicitly documents that stack-name traversal was a known, still-open gap
at the time, deliberately left unfixed at its source. The other four call
sites (`Service.Provision`, `buildWorkdirPath`, `determineSourceTargetDirectory`,
`BuildAndResolveWorkdirPath`) had no containment check at all — including
two that write real files (`source.go`, `provision_hook.go`).

Given `BuildPath` is documented as the one formula every caller must share,
patching a third ad hoc copy of the same guard at just the newly-flagged
call site would leave the other three write paths equally exploitable via
the same stack-name vector, and would add a third near-duplicate of logic
that already existed twice. Moved the containment check into `BuildPath`
itself instead, so every caller gets it automatically, and simplified the
two existing ad hoc guards down to a single error check now that `BuildPath`
guarantees containment.

## Changes

- `pkg/provisioner/workdir/types.go`:
  - `BuildPath` now returns `(string, error)` instead of `string`.
  - New `escapeComponentNameForPath` encodes `/` and `\` in the component
    name as `--` (a doubled hyphen) instead of a single `-`, so a name
    containing a path separator can never collide with a differently-named
    component that uses a literal hyphen in the same spot (`app/local` →
    `app--local`, distinct from `app-local` → `app-local`). This isn't a
    fully injective encoding — a component name containing a literal `--`
    could still collide with a differently-placed separator — a deliberate
    trade-off documented in the function's comment: fully escaping single
    `-` too would change the on-disk workdir name for the overwhelming
    majority of real (kebab-case, single-hyphen) components, not just the
    rare ones containing a separator.
  - New `containWithinBase` absolutizes the derived path and `basePath` for
    comparison and returns `errUtils.ErrPathTraversal` if the derived path
    doesn't fall within `basePath`. On success it returns the path
    unchanged (not the absolutized form), preserving `BuildPath`'s existing
    return format for callers that pass a relative `basePath`.
- `pkg/provisioner/workdir/workdir.go` (`createWorkdirDirectory`),
  `pkg/provisioner/source/source.go` (`buildWorkdirPath`),
  `pkg/provisioner/source/provision_hook.go`
  (`determineSourceTargetDirectory`), `pkg/component/workdir_path.go`
  (`BuildAndResolveWorkdirPath`): updated for the new `(string, error)`
  signature. None of these four had a prior fallback path, so `BuildPath`'s
  error is now returned as a hard failure — matching CodeRabbit's own
  suggested fix ("reject derived paths... return the validation error
  without creating directories").
- `pkg/terraform/output/config.go` (`extractComponentPath`),
  `internal/terraform_backend/terraform_backend_local.go`
  (`resolveLocalBackendComponentPath`): simplified their existing ad hoc
  containment guards down to a single `if err != nil` check against
  `BuildPath`'s new error, preserving their existing "log and fall back to
  a safe alternative path" behavior exactly.

## Validation

- New regression tests, confirmed failing pre-fix and passing post-fix
  (verified by temporarily reverting just the relevant logic while keeping
  the new function signatures, per this repo's test-first bug-fixing
  workflow):
  - `TestBuildPath_NoCollisionBetweenSlashAndHyphen`
    (`pkg/provisioner/workdir/types_test.go`) — asserts `BuildPath` with
    component `app/local` and component `app-local` produce different
    paths.
  - `TestBuildPath_RejectsStackTraversal`
    (`pkg/provisioner/workdir/types_test.go`) — asserts `BuildPath` rejects
    a `../`-laden stack name with `errUtils.ErrPathTraversal`.
  - `TestServiceProvision_RejectsStackTraversal`
    (`pkg/provisioner/workdir/workdir_test.go`) — integration-level:
    `Service.Provision` with a traversal stack name returns
    `errUtils.ErrPathTraversal` and never calls `MkdirAll` (no mock
    expectations set, so an unexpected call fails the test on its own).
    Reverting only the containment check reproduced the real escape: the
    unguarded path resolved to a directory outside the test's temp dir
    entirely (`.../evil-vpc`, a sibling of the temp dir).
  - Updated `TestBuildPath`'s existing table cases (nested/backslash
    component names) and
    `TestServiceProvision_NestedComponentName_SanitizesLikeBuildPath` for
    the new `--` encoding.
  - Updated `TestReadTerraformBackendLocal_JITWorkdir`'s nested-component
    subtest (`internal/terraform_backend/terraform_backend_local_test.go`,
    also converted to a table-driven case per a separate, unrelated
    CodeRabbit nitpick on the same PR) for the new encoding.
- `go build ./...` — clean.
- `gofumpt -l` on all changed files — clean.
- `go test ./pkg/provisioner/... ./internal/terraform_backend/...
  ./pkg/terraform/output/... ./pkg/component/... ./pkg/schema/...
  ./cmd/terraform/workdir/...` (full packages, not just the new tests) —
  all pass.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
- Updated `tests/fixtures/scenarios/source-provisioner-workdir-nested/README.md`'s
  documented expected workdir directory name for its `app/local-nested`
  manual-testing case (`dev-app-local-nested` → `dev-app--local-nested`);
  the fixture's separate, hand-configured `backend.local.path` (unrelated
  to `BuildPath`, a literal string in the stack YAML) is unaffected since
  it depends only on path *depth*, not the encoded component name's exact
  characters.

## Follow-ups

None.
