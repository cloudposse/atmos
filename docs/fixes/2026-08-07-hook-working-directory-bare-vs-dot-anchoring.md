# Fix: `kind: step` hooks anchor bare-relative `working_directory` to the component, not the process cwd

**Date:** 2026-08-07

## Summary

An explicit `working_directory:` value on a `kind: step`/`kind: steps` hook was always resolved
against the Atmos process's own working directory, regardless of what the string looked like. This
refines that resolution to distinguish two shapes, following the existing
`docs/prd/base-path-resolution-semantics.md` Dot/Bare convention already used for `base_path`: a
dot-prefixed value (`.`, `..`, `./x`, `../x`) keeps resolving against the process cwd, while a bare
relative value (`x`, `x/y`, no dot anchor) now resolves against the component's own working
directory instead — the same directory an unset `working_directory` already defaults to. Workflows
and custom commands are unaffected: they have no "current component" concept, so a bare relative
value there still resolves against the process cwd exactly as before.

## Context

This builds on the fix shipped earlier on this branch (`osterman/fix-archive-step-workdir`, PR #2880):
`pkg/runner/step/handler_base.go`'s `BaseHandler.ResolveInWorkingDirectory` anchors step
fields (`source`, `path`, `files`, `context`, ...) to `step.WorkingDirectory`. That fix left one gap:
when `step.WorkingDirectory` itself was an explicit, non-empty, non-absolute string, it was always
treated as CWD-relative via `filepath.Abs`, with no distinction based on the string's shape.

`pkg/hooks.ComponentPath(ctx)` (`pkg/hooks/command_engine.go`) already computes "the component's
effective working directory" for the *unset*-`working_directory` default case
(`setDefaultStepWorkingDirectory`, `pkg/hooks/step_engine.go`), via a 3-tier fallback: a provisioned
workdir if one exists on disk, else the in-repo component path (following `metadata.component`
aliases), else the process cwd as a last resort. Reusing this exact function for the new
bare-relative case — rather than reimplementing it — keeps the new behavior automatically
compatible with provisioned working directories and every existing way of computing a component's
effective path, since it is the same call, not a parallel computation that could drift.

Classification has to happen *after* `vars.Resolve()` inside `pkg/runner/step`, not inside
`pkg/hooks` before dispatch: an explicit `working_directory:` value can itself contain a
`.steps.X.value`/`.env.X` Go template, which is only resolvable via the `vars.Resolve()` pass that
runs strictly after the hooks engine's own (earlier, different) templating pass. So the component
anchor has to flow down through `Variables` (which already flows from `pkg/hooks` into every
handler) rather than being pre-resolved in place by the hooks engine.

## Changes

- `pkg/runner/step/handler_base.go`: added `isDotPrefixedWorkingDirectory`, a private classifier
  mirroring `docs/prd/base-path-resolution-semantics.md`'s Dot-detection (`.`, `..`, `./x`, `../x`,
  and Windows `.\x`/`..\x`). Deliberately does not reuse `pkg/component.IsExplicitComponentPath`,
  which is missing the bare `".."` case and is scoped to CLI component-path arguments, a related but
  distinct concept. `resolveWorkingDirectory` now anchors a bare (non-dot-prefixed), non-absolute
  value to `vars.componentWorkingDir` when it's set to an absolute path; otherwise it falls through
  to the pre-existing `filepath.Abs`-against-cwd behavior, unchanged.
- `pkg/runner/step/variables.go`: added `componentWorkingDir string` field and
  `SetComponentWorkingDirectory(path string)` setter on `Variables`, populated exclusively by
  `pkg/hooks`. Distinct from the existing, unrelated `componentInfo` field (a lookup-by-name resolver
  for arbitrary components, used only by `tflint.go` and populated only by workflows/custom
  commands).
- `pkg/hooks/step_engine.go`: `stepVariables` (the single shared `Variables` constructor used by
  both `stepEngine.Run` and `stepsEngine.Run`) now calls `vars.SetComponentWorkingDirectory(ComponentPath(ctx))`.
  `setDefaultStepWorkingDirectory` (the unset-default path) is untouched — it already produces an
  absolute `step.WorkingDirectory`, which short-circuits past the new Dot/Bare branch entirely.
- Regression tests: four new subtests in `pkg/runner/step/handler_base_test.go`'s
  `TestBaseHandler_ResolveInWorkingDirectory` (bare-anchors-to-component, dot-always-wins for both
  `./x` and bare `..`, bare-falls-back-to-cwd when no anchor is set) and three new end-to-end tests
  in `pkg/hooks/step_engine_test.go` using the real `archive` step type
  (`TestStepEngineRunsArchiveTypeWithBareRelativeWorkingDirectory`,
  `TestStepEngineRunsArchiveTypeWithDotPrefixedWorkingDirectoryStaysCWDRelative`,
  `TestStepEngineRunsArchiveTypeWithBareRelativeWorkingDirectoryUsesProvisionedWorkdir` — the last
  one proving compatibility with `ComponentPath`'s provisioned-workdir tier, mirroring
  `TestComponentPathFor_Fallbacks`'s fixture in `pkg/hooks/command_engine_test.go`).
- Documentation: added a self-contained resolution table (no links to internal `docs/prd/` files —
  confirmed the website doesn't reference PRDs, with one pre-existing outlier not to be repeated) to
  `website/docs/stacks/hooks.mdx`'s `kind: step` section, cross-references from
  `website/docs/workflows/workflows/workflow/steps/working-directory.mdx` and
  `website/docs/cli/configuration/commands/command/working-directory.mdx` clarifying those surfaces
  are unaffected, and matching notes in `agent-skills/skills/atmos-steps/SKILL.md` and
  `agent-skills/skills/atmos-hooks/SKILL.md` (the latter carries the full authoritative rule).

## Validation

- Every new test was confirmed to fail against the pre-fix code before the fix was applied (the two
  bare-relative `pkg/hooks` end-to-end tests; the dot-prefix guard test already passed pre-fix by
  design, since everything was CWD-relative before this change — called out explicitly in its own
  doc comment so it isn't mistaken for a broken TDD cycle).
- `go build ./...` — clean.
- `go test ./pkg/runner/step/... ./pkg/hooks/...` (`-count=1`) — all packages `ok`, including the
  full pre-existing suite (`TestStepHooksDefaultToComponentWorkingDirectory`,
  `TestSetDefaultStepWorkingDirectory_ExcludesAtmosStepType`,
  `TestStepEngineRunsArchiveTypeWithRelativeWorkingDirectory`, `TestComponentPathFor_Fallbacks`, and
  all `TestBaseHandler_ResolveInWorkingDirectory` subtests).
- `go vet ./pkg/runner/step/... ./pkg/hooks/...` — clean.
- `atmos fix lint` (patch-scoped `golangci-lint` + `lintroller`) — 0 issues (after fixing one
  `godot` finding: a comment sentence starting with a lowercase package-path identifier).
- Patch-scoped coverage (`atmos fix coverage`) — `STATUS: OK`; verified via the raw profile that
  every new line (`isDotPrefixedWorkingDirectory`, the new `resolveWorkingDirectory` branch,
  `SetComponentWorkingDirectory`, and the `stepVariables` wiring) shows a nonzero execution count —
  no coverage gaps introduced.
- `cd website && npm run build` — succeeds; Docusaurus's broken-anchor check reported zero new
  broken links (the one pre-existing warning is in an unrelated changelog post).

## Follow-ups

None.
