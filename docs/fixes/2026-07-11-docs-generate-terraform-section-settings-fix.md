# `atmos docs generate` — terraform-docs section settings silently ignored

**Date:** 2026-07-11
**Branch:** `osterman/tf-docs-to-atmos-docs-skill`
**Found while:** authoring the `atmos-migration` skill's `from-terraform-docs.md` reference,
which required verifying every documented `docs.generate.<name>.terraform.*` field actually
works before writing migration guidance for it.

---

## What the Bug Was

`docs.generate.<name>.terraform.show_inputs`, `show_outputs`, `show_providers`, and
`hide_empty` had **no effect** on the Terraform documentation rendered by
`atmos docs generate`. Setting `show_inputs: false` still rendered the Inputs section;
`hide_empty: true` still rendered "No providers." placeholders for empty sections.
`indent_level` was never read at all.

`sort_by` was **not** affected — it worked correctly (see Root Cause).

## Root Cause

**File:** `internal/exec/docs_generate.go`, `runTerraformDocs`

The function built a `print.Config` from the atmos.yaml `terraform.*` settings:

```go
config := tfdocsPrint.DefaultConfig()
config.Sections.Providers = settings.ShowProviders
config.Sections.Inputs = settings.ShowInputs
config.Sections.Outputs = settings.ShowOutputs
```

This `config` was passed to `tfdocsTf.LoadWithOptions(config)` to load the module, but the
**formatter** — the code that actually renders section headings and tables via Go templates
(`{{- if .Config.Sections.Inputs -}}` etc., in the vendored
`terraform-docs/terraform-docs` templates) — was constructed with a **separate, fresh**
`tfdocsPrint.DefaultConfig()` instead of the customized `config`:

```go
formatter = tfdocsFormat.NewMarkdownTable(tfdocsPrint.DefaultConfig())  // wrong: not `config`
```

`tfdocsPrint.DefaultConfig()` defaults `Sections.Inputs/Outputs/Providers` to `true` and
`Settings.HideEmpty` to `false`, so the formatter always rendered every section regardless of
what the user configured. `Settings.Indent` was never touched by either config, so
`indent_level` was dead configuration.

`sort_by` was unaffected because sorting happens during module *loading*
(`tfdocsTf.LoadWithOptions` → `sortItems(module, config)`), which correctly used the
customized `config` — the bug was specific to section visibility settings consumed only by
the formatter's own config, and to settings (`hide_empty`, `indent_level`) that were never
wired into any config at all.

## Fix

Reuse the same `config` for both the module loader and the formatter, and wire `HideEmpty`
and `IndentLevel` into it:

```go
config.Settings.HideEmpty = settings.HideEmpty
if settings.IndentLevel > 0 {
    config.Settings.Indent = settings.IndentLevel
}
...
formatter = tfdocsFormat.NewMarkdownTable(config)  // reuse config, not a fresh DefaultConfig()
```

`IndentLevel` is only applied when greater than zero, since the schema's Go zero value (`0`
when unset in `atmos.yaml`) would otherwise silently override terraform-docs' own default
indent of `2`.

## Tests

**File:** `internal/exec/docs_generate_test.go`

- `TestRunTerraformDocs_SectionsRespected` — builds a minimal real Terraform module (one
  variable, one output, one resource implying one provider), runs `runTerraformDocs` with
  `ShowInputs: false, ShowOutputs: true, ShowProviders: false`, and asserts the rendered
  output omits the input/provider content and includes the output. Repeats with the flags
  flipped to confirm the reverse. Written first against the unfixed code to confirm it failed
  before applying the fix (per the repo's bug-fixing workflow).
- `TestRunTerraformDocs_HideEmptyRespected` — a module with an empty Inputs section; asserts
  the `"No inputs."` placeholder renders when `HideEmpty: false` and is fully suppressed
  (heading and placeholder both) when `HideEmpty: true`.

Both tests failed against the pre-fix code and pass after the fix. Full `internal/exec`
suite passes with no regressions.

## Files Changed

| File | Change |
|------|--------|
| `internal/exec/docs_generate.go` | `runTerraformDocs` reuses `config` for the formatter instead of a fresh `DefaultConfig()`; wires `HideEmpty`/`IndentLevel` |
| `internal/exec/docs_generate_test.go` | `TestRunTerraformDocs_SectionsRespected`, `TestRunTerraformDocs_HideEmptyRespected` |
