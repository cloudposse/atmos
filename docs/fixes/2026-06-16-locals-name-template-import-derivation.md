# Stack-name derivation reads vars/settings/env merged across imports

**Issues:** [#2374](https://github.com/cloudposse/atmos/issues/2374), partial
[#2343](https://github.com/cloudposse/atmos/issues/2343)
**Date:** 2026-06-16
**Status:** Fixed (derivation path)

---

## Problem

When `stacks.name_template` references identifying values that live in a parent `_defaults.yaml`
import, and a leaf stack file declares any `locals:` block, the locals pre-pass derived a
**malformed** stack name. Reported symptoms:

- **#2343** — `name_template: "{{ .vars.namespace }}-{{ .vars.stage }}"` with `namespace` in a
  parent `_defaults.yaml`: `atmos list stacks` shows `-prod` instead of `acme-prod`; the stack
  disappears from listings.
- **#2374** — same shape but `name_template` references `.settings.*`. `describe locals -s <name>`
  returns `stack not found`, and `{{ .locals.* }}` renders as `<no value>-...` in component vars.

## Root cause

`deriveStackNameFromTemplate` (`internal/exec/describe_locals.go`) had two defects:

1. **Only `vars` was provided to the template.** The `templateData` map wrapped only
   `varsSection`, never `settings` or `env`. So any `name_template` referencing `.settings.*`
   (or `.env.*`) could never resolve — the direct cause of **#2374**.
2. **No import merge.** The template was rendered against the leaf file's own sections only. A
   `namespace`/`tenant` defined in an imported `_defaults.yaml` was invisible — the cause of
   **#2343**'s `-prod` malformed name.

## Fix

### `deriveStackNameFromTemplate` — vars + settings + env, reject `<no value>`

- `templateData` now carries `vars`, `settings`, and `env` (each defaulted to an empty, non-nil
  map so `missingkey=default` renders `<no value>` rather than panicking).
- The derivation pre-pass always renders with `ignoreMissingTemplateValues=true` (independent of
  the user-facing global flag, which governs the main rendering pipeline). Stack-name derivation
  is internal best-effort machinery: a missing identifying value must fall back to the filename,
  not abort.
- A rendered name containing `<no value>` is explicitly rejected (falls back to the filename), so
  downstream code never sees a malformed identifier.

### `deriveStackNameSections` — lite, YAML-only import overlay

New helper that walks the import graph from the raw config and returns `vars`/`settings`/`env`
merged across the file and all transitively-imported files (imports are the base; the current
file overrides them). It deliberately does **not** process Go templates or YAML functions — it is
a fast, side-effect-free overlay used solely to feed `name_template` during derivation. Unresolvable
import paths and non-pure-YAML files are silently skipped; cycles are broken with a `visited` set.

Supporting helpers: `deriveStackNameSectionsInto`, `mergeImportedStackNameSections`,
`loadAndMergeStackNameImport`, `resolveImportFilePathsForStackName` (+ `hasYAMLExt`,
`findFirstExistingWithExt`, `globMatchesWithExt`), and `mergeMapShallow`.

`deriveStackName` now merges the imported sections (plus the caller's `varsSection` on top) before
trying `name_template` and `name_pattern`.

## Tests

Fixtures (shared with PR 3):

- `tests/fixtures/scenarios/locals-name-template-vars-imports/` (#2343 repro)
- `tests/fixtures/scenarios/locals-name-template-settings-imports/` (#2374 repro)

Integration (`tests/cli_locals_test.go`) — the `describe locals` path, fixed by this PR:

- `TestLocalsNameTemplateVarsFromImportsDescribeLocals`
- `TestLocalsNameTemplateSettingsFromImportsDescribeLocals`

Both verified **failing before** (`stack not found: acme-prod` / `cloudlabs-plat-ue1-prod`) and
passing after.

Unit (`internal/exec/describe_locals_test.go`):

- `TestDeriveStackNameFromTemplate_Sections` — settings/env/combined resolution, `<no value>`
  rejection, empty template, nil sections.
- `TestDeriveStackNameFromTemplate_RejectsUnresolvedMarkers`, `..._MalformedTemplate`.
- `TestDeriveStackNameSections` — nil, leaf-only, import-merge with leaf override;
  `..._ImportCycle`, `..._InvalidImportSection`.
- `TestStackNameImportHelpers`, `TestResolveImportFilePathsForStackName_Glob`.

All new functions are covered ≥80%.

## Out of scope (PR 3)

The **full `describe stacks` / `describe component` pipeline** still mis-derives these stacks
because imported `vars`/`settings` are clobbered during import recursion when the leaf declares
`locals:` (the `processYAMLConfigFileWithContextInternal` overwrite, fixed via `localsContextResult`).
PR 3 lands that fix plus:

- the `describe-stacks-name-template` site in `resolveStackName`
  (`describe_stacks_component_processor.go`) which still hardcodes `false` (a multi-line
  `ProcessTmpl` call that PR 1's grep missed — the exact site named by #2345); and
- the full-pipeline integration tests (`...DescribeStacks` / `...DescribeComponent`) that close
  #2343 and #2374 end-to-end.
