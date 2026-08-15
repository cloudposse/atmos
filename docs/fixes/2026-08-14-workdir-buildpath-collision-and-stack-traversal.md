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
  three. (A first pass at this fix replaced `/`/`\` with `--` instead of
  `-`; CodeRabbit correctly flagged that a component literally named
  `app--local` still collided with `app/local` under that scheme — see
  Context below.)
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

**Second pass.** A follow-up CodeRabbit review on the same PR caught that
the collision fix's first pass — encoding `/`/`\` as `--` while leaving
literal `-` untouched — was still not injective: a component literally
named `app--local` (a real, if less common, naming pattern) collided with
`app/local`, both encoding to `app--local`. Closing that residual gap
requires escaping the literal hyphen too, which changes the on-disk workdir
name for essentially every existing kebab-case component (the trade-off the
first pass's doc comment explicitly chose to avoid) — but a security/data-
integrity finding that's still collidable, even on a narrower input, isn't
an acceptable trade-off. Replaced the `--` scheme with a fully injective
one: escape the literal hyphen *before* encoding separators, using a
distinct two-character tag for each (`-h` for a literal hyphen, `-s` for
either separator) so `-` never appears unescaped in the output and every
encoded name is unambiguous.

## Changes

- `pkg/provisioner/workdir/types.go`:
  - `BuildPath` now returns `(string, error)` instead of `string`.
  - New `escapeComponentNameForPath` injectively encodes the component name:
    every literal `-` is escaped to `-h`, and every `/`/`\` is encoded to
    `-s`, in a single left-to-right pass (`strings.Builder` over runes, not
    sequential `strings.ReplaceAll` calls, which would let an escaped
    hyphen and an encoded separator become ambiguous when adjacent).
    Because `-` never appears unescaped in the output, every `-` in an
    encoded name unambiguously starts a two-character escape token, so no
    two distinct component names can ever produce the same encoded segment
    — `app/local` → `app-slocal`, `app-local` → `app-hlocal`, `app--local`
    → `app-h-hlocal`, all distinct. This does change the on-disk workdir
    name for existing components with hyphens (e.g. `my-component` →
    `my-hcomponent`); an existing `.workdir/` cache is effectively
    invalidated the first time each component is next provisioned, but
    re-provisioning is a normal, self-healing operation (a fresh sync plus
    `terraform init`), not user-visible data loss.
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
    component names `app/local`, `app-local`, and `app--local` all produce
    pairwise-distinct paths (the third case is the one the first-pass `--`
    encoding still collided on).
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
    the final `-h`/`-s` encoding.
  - Updated `TestReadTerraformBackendLocal_JITWorkdir`'s nested-component
    subtest (`internal/terraform_backend/terraform_backend_local_test.go`,
    also converted to a table-driven case per a separate, unrelated
    CodeRabbit nitpick on the same PR) for the new encoding.
  - Updated every other hardcoded expected-workdir-name assertion this
    encoding change touched, across
    `pkg/provisioner/workdir/integration_test.go`,
    `pkg/provisioner/source/{source,provision_hook}_test.go`,
    `pkg/terraform/output/config_test.go`, and
    `pkg/component/workdir_path_test.go` — the latter's
    `TestBuildAndResolveWorkdirPath_*` tests were switched to compute their
    expected path via the real `workdir.BuildPath` instead of a hand-rolled
    `stack+"-"+component` string, so they can't go stale again the same
    way.
- `go build ./...` — clean.
- `gofumpt -l` on all changed files — clean.
- `go test ./pkg/provisioner/... ./internal/terraform_backend/...
  ./pkg/terraform/output/... ./pkg/component/... ./pkg/schema/...
  ./cmd/terraform/workdir/... ./pkg/runner/step/... ./internal/exec/...`
  (full packages, not just the new tests) — all pass.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
- Updated `tests/fixtures/scenarios/source-provisioner-workdir-nested/README.md`
  and its `stacks/deploy/dev.yaml` comments for the final encoding (e.g.
  `dev-app-slocal-hnested` for `app/local-nested`, `dev-ecs-scluster` for
  `ecs/cluster`); the fixture's separate, hand-configured
  `backend.local.path` (unrelated to `BuildPath`, a literal string in the
  stack YAML) is unaffected since it depends only on path *depth*, not the
  encoded component name's exact characters.
- Noted but did not fix: `CleanWorkdir` (`pkg/provisioner/workdir/clean.go`)
  has its own separate, unsanitized `fmt.Sprintf("%s-%s", stack, component)`
  formula — never routed through `BuildPath`/`escapeComponentNameForPath` at
  all. It already diverged from `BuildPath` for `/`-containing component
  names before this fix; this fix widens that divergence to include
  ordinary hyphenated names too, so `atmos`'s workdir-clean path can no
  longer find the directory `Service.Provision` actually created for any
  such component. Out of scope for this CodeRabbit-driven fix (not
  flagged, and `clean.go` untouched by the current diff) — flagged to the
  user as a pre-existing, now-larger inconsistency worth its own follow-up.

## Follow-ups

None.
