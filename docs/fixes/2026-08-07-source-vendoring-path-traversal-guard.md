# Fix: unguarded path traversal in the default (non-workdir) source-vendoring path

**Date:** 2026-08-07

## Summary

`source.DetermineTargetDirectory`'s non-workdir fallback — the **default**
vendoring path, used whenever `provision.workdir.enabled` is not set —
computed the vendor target as `filepath.Join(componentBasePath, component)`
with no containment check. A component named with a leading `..` segment
(e.g. `../escape-test-nowd`) vendored outside `components/terraform/`
entirely, into an arbitrary sibling directory. Confirmed via a real
`atmos terraform source pull "../escape-test-nowd" --stack dev` run against
the `tests/fixtures/scenarios/source-provisioner-workdir-nested` fixture:
the source landed at `components/escape-test-nowd/` instead of anywhere
under `components/terraform/`.

This is unrelated to the workdir-nesting fixes
(`docs/fixes/2026-08-05-workdir-nested-component-path-depth.md`,
`docs/fixes/2026-08-07-createworkdirdirectory-duplicate-unsanitized-formula.md`)
but was found during the same field-test pass, using the same fixture.

## Context

`DetermineTargetDirectory` (`pkg/provisioner/source/source.go`) has three
resolution branches: an explicit `working_directory` override, a
workdir-enabled path (via `workdir.BuildPath`), and — the default,
far-more-commonly-hit case when `provision.workdir.enabled` is not set — a
plain `filepath.Join(componentBasePath, component)`. `component` comes from
user-controlled stack manifest keys, and `filepath.Join`'s implicit
`filepath.Clean` resolves `..` segments against `componentBasePath`, so a
component name containing enough `../` can walk out of
`components/terraform/` (or `components/helmfile/`, etc.) into any sibling
directory the process can write to.

Two other callers that derive paths from the same kind of
user-controlled input already guard against exactly this:
`internal/terraform_backend/terraform_backend_local.go`'s
`resolveLocalBackendComponentPath` and `pkg/terraform/output/config.go`'s
`extractComponentPath`. Both absolutize the derived path, check it has the
absolutized base path as a prefix (or equals it), and — because both have a
safe alternative path to derive instead — silently fall through to that
alternative on failure. `DetermineTargetDirectory`'s non-workdir branch has
no such alternative to fall through to (it already **is** the final
default), so an escaping path is a hard error instead.

## Changes

- `pkg/provisioner/source/source.go`: `DetermineTargetDirectory`'s
  non-workdir fallback now validates the joined target directory against
  `componentBasePath` via a new `validateWithinComponentBasePath` helper
  before returning it — absolutizes both paths and requires the target to
  equal, or be nested under, the component base path. On failure it returns
  the existing `errUtils.ErrPathTraversal` sentinel (already used for an
  analogous guard in `pkg/generator/engine/templating.go`), wrapped via the
  error builder with an explanation, a hint, and the offending paths as
  context.

## Validation

- New regression test case, confirmed failing pre-fix and passing post-fix:
  `TestDetermineTargetDirectory/component_name_with_.._escapes_component_base_path`
  (`pkg/provisioner/source/source_test.go`) — calls the real
  `DetermineTargetDirectory` with `component: "../escape-test-nowd"` and a
  terraform component base path, and asserts an `ErrPathTraversal` error is
  returned instead of a silently-escaping directory.
  - Pre-fix failure: `An error is expected but got nil.`
  - Post-fix: `PASS`.
- `go build ./...` — clean.
- `gofumpt -l pkg/provisioner/source/source.go pkg/provisioner/source/source_test.go` — clean (no output).
- `go test ./pkg/provisioner/... ./internal/terraform_backend/...` (full
  packages, not just the new test) — all pass, no regressions. In
  particular all pre-existing `TestDetermineTargetDirectory` table cases
  (working_directory overrides, default base paths for terraform/helmfile/
  packer, missing-config error cases, workdir-enabled variants) still pass
  unchanged.
- Manual field-test fixture
  (`tests/fixtures/scenarios/source-provisioner-workdir-nested`, component
  `../escape-test-nowd`) retained for future manual verification; not
  re-run as part of this fix (see the fixture's README for the exact repro
  commands).

## Follow-ups

None.
