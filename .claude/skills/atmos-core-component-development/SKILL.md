---
name: atmos-core-component-development
description: "Atmos CORE contributor guide for adding/modifying a component TYPE in the Go codebase (terraform/helmfile/packer/ansible/container): the component registry & provider, the CLI command group, the describe/list type whitelist, custom-component inheritance & deep-merge, schema, and tests. NOT for authoring user components in stacks (that is the atmos-components skill)."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Core: Component Type Development

Use this skill when **developing Atmos itself** — adding or modifying a component **type** (kind) in the
Go codebase. This is contributor/core guidance, distinct from the user-facing `atmos-components` skill
(which documents authoring terraform/helmfile/container components in stacks).

Start from `docs/developing-component-plugins.md` (the component-plugin development guide). The notes
below capture the non-obvious wiring learned while adding the `container` component type.

## The component provider (pkg/component)

A component type is a `ComponentProvider` (`pkg/component/provider.go`) registered via `init()` with
`component.Register(...)` (`pkg/component/registry.go`). Reference impls:

- `pkg/component/ansible/` — typed-config built-in style.
- `pkg/component/mock/`, `pkg/component/custom/` — the `Plugins`-map plugin style.
- `pkg/component/container/` — provider + `cmd`/lifecycle split.

Layout per the guide: `config.go` (typed `Config` + `parseConfig`), `<type>.go` (provider + `init`),
`executor.go` (verb implementations), `<type>_test.go` (>90% coverage). Wire a blank import into
`cmd/root.go` so `init()` runs.

Reusable error sentinels live in `errors/errors.go`: `ErrComponentExecutionFailed`,
`ErrComponentConfigInvalid`, `ErrComponentValidationFailed`, `ErrComponentTypeEmpty`.

## First-class component config (NOT `vars`)

Per-instance config that is **not arbitrary template data** must be first-class top-level sections
(siblings of `metadata`/`env`/`composition`), NOT nested under `vars`. For container, the config reuses
the workflow container-step structs (`schema.ContainerBuildStep`/`ContainerRunStep`/`ContainerMount`/
`ContainerPort` in `pkg/schema/workflow.go`) for consistency. Decode a YAML-derived `map[string]any`
into those structs with mapstructure using `TagName: "yaml"` so snake_case keys (`build_args`,
`read_only`) map.

## The CLI command group (cmd/<type>)

Mirror `cmd/ansible/`: a base `cobra.Command` registered through the command registry
(`cmd/internal` `CommandProvider`), persistent flags via `flags.NewStandardParser()` (NEVER
`viper.BindEnv`/`BindPFlag`), one thin file per verb dispatching to
`component.MustGetProvider(<type>).Execute(&component.ExecutionContext{...})`. Wire a blank import into
`cmd/root.go`.

## CRITICAL: the describe/list type whitelist

A new top-level `components.<type>` is **dropped** (stack renders `{}`, "component not found") unless
the type is added to several hardcoded lists. Grep `AnsibleSectionName` / `"ansible"` across
`internal/exec` + `pkg/list/extract` and mirror every hit:

1. `pkg/config/const.go` — `XComponentType` / `XSectionName` consts.
2. `internal/exec/describe_stacks_component_processor.go` — the `typeEntries` list AND
   `componentsSectionHasComponents`.
3. `internal/exec/describe_stacks.go` — `getComponentBasePath` switch.
4. `internal/exec/describe_component.go` — the `detectComponentType` auto-detect order (a loop over
   `[terraform, helmfile, packer, ansible, container]`); a type missing here makes `atmos describe
   component <name>` fail even when the lifecycle works.
5. `pkg/list/extract/components.go` — THREE hardcoded type lists (per-stack `extractComponentType` ×2,
   unique `extractUniqueComponentType`).

Verify with `atmos describe stacks` (stack with only the new type must be non-empty) and
`atmos describe component <name> -s <stack>`.

## Inheritance & deep-merge for custom types

Built-in types (terraform/helmfile/packer/ansible) get full inheritance via the processComponent
pipeline. Other types ride the **custom-component fallback** in
`internal/exec/stack_processor_process_stacks.go`. That fallback now resolves `metadata.inherits` and
**generic-deep-merges all top-level keys** (`resolveCustomComponentInheritance`), so custom types honor
catalog/abstract defaults. Gotchas:

- Strip `metadata.type`/`inherits`/`component` from a base before merging, or an abstract base poisons
  the concrete component (`sanitizeBaseForInheritance`).
- Reject `metadata.type: abstract` for execution and filter it from listings.
- Use the **native merge** (`pkg/merge`) — it already incorporates the slice-truncation and
  permissive-type-mismatch fixes in `docs/fixes/2026-03-19-*` and `docs/fixes/2026-03-24-*`.
- Component-level config *sections* (vars/settings/env/hooks/secrets/...) that need per-section
  inheritance still require the section whitelist plumbing (see `docs/errors.md` / the merge helpers).

## Schema, docs, tests

- JSON schema: `pkg/datafetcher/schema/atmos/manifest/1.0.json` — add `<type>_components` +
  `<type>_component_manifest` definitions and the `components.<type>` property.
- Docs: `website/docs/components/components-overview.mdx` (Component Types table + directory diagram),
  `website/docs/components/<type>.mdx`, and `website/docs/cli/commands/<type>/usage.mdx`.
- Tests: provider unit tests with a mockgen `Runtime`/dependency, `cmd.NewTestKit` for the command,
  inheritance + abstract + graceful-empty cases. Regenerate affected `--help` golden snapshots with
  `-regenerate-snapshots` (never hand-edit).
- Gate: `./custom-gcl run --new-from-rev=origin/<base> ./pkg/component/<type>/... ./internal/exec/...`.
