# Fix: `validateWithinComponentBasePath` now resolves symlinks before checking containment

**Date:** 2026-08-13

## Summary

`pkg/provisioner/source/source.go`'s `validateWithinComponentBasePath` guarded
against component names escaping `componentBasePath` via literal `..` segments,
but did so with a purely lexical check. A symlink that exists under
`componentBasePath` and points outside it (e.g. `componentBasePath/evil ->
/etc`) passed the lexical check — the literal path string still starts with
`componentBasePath` — while resolving to a location outside it on the real
filesystem, defeating the containment guard. The function's own doc comment
already disclaimed this as a known, unhandled limitation. The check now also
resolves symlinks in the existing portion of both the target and base paths
and re-verifies containment against the resolved locations.

## Context

This surfaced from a code-review finding on `osterman/test-container-fields-ignored`
flagging that `validateWithinComponentBasePath` never resolved symlinks. Most
findings in that same review batch turned out to already be fixed by earlier
commits on the branch (see the accompanying
`docs/fixes/2026-08-13-custom-command-script-step-container-override-dropped.md`
for the sibling finding from the same batch); this one was verified as still
genuinely open by reading the current implementation and confirming no
symlink resolution existed anywhere in the call path.

`componentBasePath` (the components root, e.g. `<repo>/components/terraform`)
typically exists on disk, but the computed `targetDir` commonly does not exist
yet — this guard runs before the target directory is created — so a naive
`filepath.EvalSymlinks(targetDir)` would fail with `ENOENT` for the common
case and can't be the whole fix.

## Changes

- `pkg/provisioner/source/source.go`: `validateWithinComponentBasePath` now
  runs two containment checks. Phase 1 is the original cheap lexical check
  (`filepath.Abs` + `filepath.Rel`, extracted into a new `isWithinBase`
  helper), unchanged in behavior. Phase 2, which only runs if phase 1 passes,
  resolves symlinks in the longest *existing* ancestor of both the target and
  base paths (new `resolveExistingSymlinks` helper — walks up from the path
  until `filepath.EvalSymlinks` succeeds, then rejoins the non-existent
  suffix) and re-runs `isWithinBase` against the resolved paths. Resolving the
  base too (not just the target) avoids false positives on systems where the
  base itself sits behind a symlink (e.g. macOS `/tmp -> /private/tmp`).
  Rejection still returns `errUtils.ErrPathTraversal`, with a distinct hint
  for the symlink-escape case versus the literal `..`-escape case. A symlink
  loop (`ELOOP`) during resolution now fails closed as `ErrPathTraversal`
  rather than silently falling back — a deliberate, conservative addition
  beyond the literal reported gap.
- `pkg/provisioner/source/source_test.go`: added
  `TestValidateWithinComponentBasePath_SymlinkEscape` (symlink under base
  pointing outside it must be rejected),
  `TestValidateWithinComponentBasePath_SymlinkWithinBase` (symlink under base
  pointing to another location inside it must still be allowed), and
  `TestValidateWithinComponentBasePath_BaseItselfIsSymlink` (base path itself
  reached through a symlink must not cause a false positive). Added a local
  `trySymlink` test helper that skips gracefully when symlink creation isn't
  supported (e.g. Windows without Developer Mode, or a locked-down sandbox).

No caller changes were needed — `DetermineTargetDirectory` only consumes the
returned error, and the function's exported signature is unchanged.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/provisioner/source/... -run TestValidateWithinComponentBasePath -v`
  — all subtests pass, including the pre-existing
  `TestValidateWithinComponentBasePath_RootBase` suite (unaffected by phase 1
  being unchanged) and the three new symlink regression tests.
- `go test ./pkg/provisioner/source/...` (full package) — pass.
- `./custom-gcl run --new-from-rev=origin/main` — no findings in the changed
  files (fixed two `godot` sentence-capitalization findings introduced by the
  new doc comments during review).
- `gofumpt -l` on the changed files — clean.

## Follow-ups

None.
