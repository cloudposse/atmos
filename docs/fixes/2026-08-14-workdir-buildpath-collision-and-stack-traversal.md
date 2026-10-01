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
an acceptable trade-off. Replaced the `--` scheme with an injective one:
escape the literal hyphen *before* encoding separators, using a distinct
two-character tag (`-h` for a literal hyphen, `-s` for `/`) so `-` never
appears unescaped in the output.

**Third pass.** A third CodeRabbit review round on the same PR caught that
the second pass's scheme mapped *both* `/` and `\` to the same `-s` token,
so it was still not fully injective: `ecs/cluster` and `ecs\cluster` both
encoded to `ecs-scluster`. That aliasing was deliberate — the doc comment
at the time claimed it was necessary to keep a given component name's
workdir identical across OSes — but that claim doesn't hold up: the
encoding is pure Go string processing (a rune loop, never delegated to the
OS-dependent `path/filepath` package), so a name's encoded output is
already fully deterministic regardless of host OS whether or not `/` and
`\` share a token. Gave `\` its own token (`-b`), closing this last
collision at no cost to the property the aliasing was meant to protect.
This also surfaced (in the same review round) that `CleanWorkdir`
(`pkg/provisioner/workdir/clean.go`) had never been routed through
`BuildPath`/`escapeComponentNameForPath` at all — see Changes below.

**Fourth pass.** A fourth CodeRabbit review round on the same PR caught two
more issues:

1. `containWithinBase`'s `absBase+sep` prefix check breaks when `basePath`
  resolves to a filesystem root (`/` on Unix, `` C:\ `` on Windows): `absBase`
  already ends in the separator there, so `absBase+sep` produces a doubled
  separator (`//`, `` C:\\ ``) that no real path under that root ever has,
  rejecting every legitimate workdir path derived from a root `basePath`.
  Replaced the prefix check with `filepath.Rel`, mirroring the same pattern
  already established for the same problem in
  `pkg/provisioner/source/source.go`'s `isWithinBase`.
2. The third pass's claim (in this doc's own Validation section, previously)
  that switching `pkg/component/workdir_path_test.go`'s `TestBuildAndResolveWorkdirPath_*`
  tests to compute their expected path via the real `workdir.BuildPath`
  meant they "can't go stale again the same way" was itself a mistake:
  `BuildAndResolveWorkdirPath` (the function under test) also calls
  `BuildPath` internally, so if `BuildPath`'s encoding were ever wrong
  again, both the test's setup and its assertion would derive the same
  wrong path and agree with each other — the test would pass regardless.
  Replaced those `BuildPath`-derived expected paths with independent,
  hand-computed literal strings (e.g. `"my-component"` → `"my-hcomponent"`)
  for every encoding-sensitive assertion, keeping `BuildPath` only for
  setup steps that just need a real directory on disk, not for the
  comparison oracle.

## Changes

- `pkg/provisioner/workdir/types.go`:
  - `BuildPath` now returns `(string, error)` instead of `string`.
  - New `escapeComponentNameForPath` injectively encodes the component name
    in a single left-to-right pass (`strings.Builder` over runes, not
    sequential `strings.ReplaceAll` calls, which would let an escaped
    hyphen and an encoded separator become ambiguous when adjacent): every
    literal `-` is escaped to `-h`, `/` to `-s`, and `\` to `-b`. Because
    `-` never appears unescaped in the output, every `-` in an encoded name
    unambiguously starts a two-character escape token, so no two distinct
    component names can ever produce the same encoded segment —
    `app/local` → `app-slocal`, `app-local` → `app-hlocal`, `app--local` →
    `app-h-hlocal`, `` app\local `` → `app-blocal`, all distinct. This does
    change the on-disk workdir name for existing components with hyphens
    (e.g. `my-component` → `my-hcomponent`); an existing `.workdir/` cache
    is effectively invalidated the first time each component is next
    provisioned, but re-provisioning is a normal, self-healing operation (a
    fresh sync plus `terraform init`), not user-visible data loss.
  - New `containWithinBase` absolutizes the derived path and `basePath` for
    comparison and returns `errUtils.ErrPathTraversal` if the derived path
    doesn't fall within `basePath`. On success it returns the path
    unchanged (not the absolutized form), preserving `BuildPath`'s existing
    return format for callers that pass a relative `basePath`. Uses
    `filepath.Rel` rather than an `absBase+separator` prefix check (fourth
    pass — see Context), so a filesystem-root `basePath` is handled
    correctly.
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
- `pkg/provisioner/workdir/clean.go` (`CleanWorkdir`): now derives the
  workdir path via `BuildPath` instead of its own separate
  `fmt.Sprintf("%s-%s", stack, component)` formula, so `atmos`'s
  workdir-clean path can actually find what `Service.Provision` created for
  a hyphenated or nested component. This was flagged as a known,
  out-of-scope divergence in an earlier pass of this fix; CodeRabbit's
  third review round asked for it to be fixed outright given the severity
  (Major/Functional Correctness), so it's fixed here instead.

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
    `pkg/component/workdir_path_test.go`.
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
- `TestBuildPath_NoCollisionBetweenSlashAndBackslash`
  (`pkg/provisioner/workdir/types_test.go`) — asserts `ecs/cluster` and
  `` ecs\cluster `` produce distinct paths (the case the second-pass `-s`
  aliasing still collided on).
- `TestCleanWorkdir_FindsProvisionedHyphenatedComponent`
  (`pkg/provisioner/workdir/integration_test.go`) — confirmed failing
  pre-fix and passing post-fix: provisions a real workdir for a hyphenated
  component via `Service.Provision`, then asserts `CleanWorkdir` finds and
  removes that exact directory. Pre-fix, `CleanWorkdir`'s own unsanitized
  formula computed a different, non-existent path and silently no-op'd
  ("No workdir found") instead of removing anything.
- Updated `TestCleanWorkdir` (`pkg/provisioner/workdir/integration_test.go`)
  to create its fixture directory via `BuildPath` instead of a hand-rolled
  `stack-component` string, for the same reason as the `component` package
  tests above.

- `TestBuildPath_AcceptsFilesystemRootBasePath`
  (`pkg/provisioner/workdir/types_test.go`, fourth pass) — asserts `BuildPath`
  accepts a Unix filesystem-root `basePath` (`/`, skipped on Windows) and a
  Windows volume-root `basePath` (`` C:\ ``, skipped elsewhere). Confirmed
  failing pre-fix (rejected with `errUtils.ErrPathTraversal` under the old
  prefix check) and passing post-fix.
- `pkg/component/workdir_path_test.go`'s five `TestBuildAndResolveWorkdirPath_*`
  encoding-sensitive tests (fourth pass) — `ExistingDir`, `AllComponentTypes`,
  `AllComponentTypesWithSubpath`, `InheritancePointerFallsBack`,
  `NonExistentDir` — now assert against independent, hand-computed literal
  expected paths instead of a `BuildPath`-derived oracle; `BuildPath` is
  still used for setup where a scenario needs a real on-disk fixture without
  itself testing the encoding formula (e.g. `AllComponentTypesWithSubpath`'s
  subpath join). All five re-verified passing after the change.
- `go build ./...`, `gofumpt -l` on all fourth-pass changed files, and
  `go test ./pkg/component/... ./pkg/provisioner/workdir/...` — all clean.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.

## Update (2026-08-17)

A later CodeRabbit review round on the same PR raised three further points against this fix;
addressed here rather than in a new file since two are direct continuations of it.

1. **Stack collision, not just traversal.** The original fix validated `stack` only against
  `containWithinBase` (catching a stack value that climbs *out* of `basePath`, e.g.
  `../../../../../../evil`), but a stack containing a real `/` that stays *within* `basePath`
  after `filepath.Join`'s implicit `Clean()` — e.g. `team/../prod` — still aliased the same
  workdir as stack `prod`, undetected. Fixed in `BuildPath` (`types.go`,
  `validateStackForPath`): `stack` is now rejected outright (not escaped, unlike
  `workdirComponent`) whenever it contains `/` or `\`. Escaping was deliberately not chosen
  here even though it's the same technique already used for the component name: it would
  change the on-disk path for the overwhelmingly common case of a stack name containing a
  literal `-` (e.g. `us-east-1-dev`), reopening exactly the kind of existing-workdir
  invalidation point 3 below addresses. Rejecting is free of that cost since real `/`/`\` in a
  resolved stack name isn't a supported Atmos naming convention. New tests:
  `TestBuildPath_RejectsStackContainingSlash`, `_RejectsStackContainingBackslash`,
  `_AllowsHyphenatedStackName` (`types_test.go`).
2. **`CleanWorkdir`'s fix didn't reach the CLI.** `pkg/provisioner/workdir.CleanWorkdir` (fixed
  above to use `BuildPath`) turned out to have zero production callers — `atmos terraform
  workdir clean/get/describe` go through `cmd/terraform/workdir/workdir_helpers.go`'s
  `DefaultWorkdirManager`, which had its own **separate**, never-updated
  `fmt.Sprintf("%s-%s", stack, component)` formula in `CleanWorkdir`, `GetWorkdirInfo`, and
  (transitively) `DescribeWorkdir`. That means the CLI's clean/get/describe commands couldn't
  find *any* hyphenated or `atmos_component`-overridden component's real workdir, not just the
  override case CodeRabbit flagged. All three now delegate to `BuildPath` and accept the
  resolved `componentConfig` (via a new `resolveComponentConfig` helper that falls back to
  `nil` when the component can no longer be described — e.g. an orphaned workdir for a
  component since removed from stack manifests — so cleanup of stale workdirs still works).
3. **Legacy on-disk paths orphaned by the hyphen re-encoding.** `escapeComponentNameForPath`
  (this doc's collision fix) changes the on-disk segment for any component name needing
  escaping, so a workdir an earlier Atmos version created under the pre-escaping formula
  becomes unreachable the moment that component is next provisioned. The "self-healing, not
  data loss" framing in this doc's Changes section holds for the common case (workdir is just
  a synced copy of component source + generated backend config), but not for a **local backend**
  component: `pkg/terraform/output/config.go` resolves the JIT workdir as Terraform's actual
  working directory, so a `backend "local"` component's `terraform.tfstate` lives inside it.
  Fixed with a scoped, best-effort migration in `createWorkdirDirectory`
  (`workdir.go`, `migrateLegacyWorkdir`): before creating a fresh directory at the new encoded
  path, it checks for a sibling at the pre-escaping path and renames it forward if the new path
  doesn't already exist. A full migration framework was judged unnecessary since
  `escapeComponentNameForPath` is itself unreleased (only exists on this branch, confirmed via
  `git merge-base --is-ancestor` against `origin/main`) — but the rename still matters for
  anyone who provisioned a workdir on an earlier commit of this same branch. New tests:
  `TestServiceProvision_MigratesPreExistingLegacyWorkdir` (`integration_test.go`),
  `TestMigrateLegacyWorkdir_SkipsWhenNewPathAlreadyExists`, `_SkipsWhenLegacyPathMissing`,
  `_ToleratesRenameError` (`workdir_test.go`).

## Follow-ups

Point 3 above surfaced a separate, pre-existing gap while writing its regression test:
`syncLocalToWorkdir`'s `SyncDir` step (`fs.go`, `deleteRemovedFiles`) deletes any workdir file
not present in the source component directory, and only protects the workspace-specific
`terraform.tfstate.d/` directory and provider lock files (`shouldSkipSyncFile`) from that —
**not** a plain `terraform.tfstate` at the workdir root (the default workspace's state). That
means a local-backend, default-workspace component's state is deleted on every re-provision
today, independent of this fix or the hyphen re-encoding. Not fixed here: out of scope for a
workdir-*path* fix, and changing sync's delete behavior deserves its own dedicated tests and
consideration of what else besides `.terraform.lock.hcl` should be protected. Flagged for the
user to decide on a follow-up.
