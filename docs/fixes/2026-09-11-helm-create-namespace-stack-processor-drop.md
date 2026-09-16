# Fix: native-Helm `create_namespace` toggle silently dropped by the stack processor

**Date:** 2026-09-11

## Summary

The `create_namespace` setting added to native Helm components in PR #3034 had no effect.
Setting `create_namespace: false` on a component validated fine but was silently dropped
during stack processing — it never reached the Helm executor, never appeared in
`atmos describe stacks`, and the default of `true` always won. Installs into a
platform-owned, pre-existing namespace still forced a namespace create, which fails with a
`403` for an identity scoped to a single namespace (Helm issues the create request before
the already-exists check applies).

## Symptom

Reported against Atmos 1.228.0 with a component like:

```yaml
components:
  helm:
    api:
      chart: .
      name: api
      namespace: lakehouse-api
      create_namespace: false   # cw-infra owns the namespace; Helm must not create it
      values:
        # ...
```

The toggle was invisible in the resolved config:

```console
$ atmos describe stacks -s dev --components api --component-types helm --sections namespace
dev:
  components:
    helm:
      api:
        namespace: lakehouse-api

$ atmos describe stacks -s dev --components api --component-types helm --sections create_namespace
{}
```

`namespace` (a whitelisted field) survived; `create_namespace` came back empty.

## Root cause

PR #3034 wired the *reading* side of the toggle — `resolveCreateNamespace` /
`boolFieldDefault` in `pkg/component/helm/values.go`, the `CreateNamespace` field on
`chartSpec`, the install-action plumbing in `pkg/component/helm/client.go`, and the JSON
schemas — but never wired it through the **stack processor**.

The stack processor carries only a fixed whitelist of native-Helm fields from a component's
raw stack manifest into the final, resolved component config. That whitelist lives in
`internal/exec/stack_processor_process_stacks_helpers_extraction.go`:

```go
var helmComponentSectionKeys = []string{
  cfg.ChartSectionName,
  cfg.ValuesSectionName,
  cfg.ValuesFilesSectionName,
  cfg.RepositoriesSectionName,
  cfg.RenderSectionName,
  "version",
  "repository",
  "namespace",
  "name",
  cfg.HelmReleaseSectionName,
  // create_namespace was missing here
}
```

`extractHelmComponentSection` copies only these keys; any unrecognized key (including
`create_namespace`) is dropped before the resolved section reaches
`buildChartSpec`/`resolveCreateNamespace`. The reader then never sees the key and falls back
to its `true` default.

The JSON schema and the runtime whitelist are two independent lists. #3034 updated the schema
(so validation accepted the key) but not the whitelist (so the key never survived
processing) — hence "validates fine, shows up nowhere."

## Changes

- `pkg/config/const.go`: added `HelmCreateNamespaceSectionName = "create_namespace"` so the
  key is referenced by a named constant rather than a bare string literal.
- `internal/exec/stack_processor_process_stacks_helpers_extraction.go`:
  - Added `cfg.HelmCreateNamespaceSectionName` to `helmComponentSectionKeys` so the toggle
    flows through base-component inheritance and into the final component config (the core
    fix — this is what the customer hit).
  - Added `cfg.HelmCreateNamespaceSectionName` to `helmLifecycleSectionKeys` so the toggle
    can also be set once as a stack-level `helm:` default and apply to every Helm component
    (useful when a platform team owns all namespaces).
- `internal/exec/stack_processor_process_stacks_helpers_test.go`: added
  `TestExtractHelmComponentSectionCreateNamespace` — a regression test that confirms
  `create_namespace: false`/`true` survives `extractHelmComponentSection`, survives the full
  `extractComponentSections` path into `result.ComponentHelm`, is carried as a stack-level
  lifecycle default, is dropped from an `overrides:` block (current contract — see below), and
  is left unset (so the reader default applies) when the key is absent.
- `internal/exec/stack_processor_merge_test.go`: added
  `TestMergeComponentConfigurations_CreateNamespacePrecedence` — proves a component-level value
  wins over a stack-level default, and an unset component inherits the stack default.

### Precedence

The toggle can be set in two places (this fix); the more specific wins:

| Where | Precedence | Use case |
|-------|-----------|----------|
| component field (`helmComponentSectionKeys`) | wins | per-release opt-out |
| stack-level `helm:` default (`helmLifecycleSectionKeys`) | weaker — a component can override it | convenience default across components |

### Deliberate non-change: overrides block

`create_namespace` in an `overrides:` block is **intentionally not supported in this PR** and is
tracked as a separate follow-up. Empirical CLI testing showed it needs more than a whitelist
entry: the `helm_overrides` JSON schema is `additionalProperties: false` and rejects the key, and
the type/global-level override propagation (what a platform team would use to enforce a setting
across every component) is a separate mechanism that a `helmOverrideSectionKeys` change alone does
not wire. `helmOverrideSectionKeys` is left narrow (only `values`), and a test guards that current
contract.

## Why the original PR's tests missed it

PR #3034's unit tests exercised `resolveCreateNamespace`/`buildChartSpec` by passing a section
map with `create_namespace` already populated. They never ran through the stack processor's
extraction whitelist, which is what strips the key at runtime. The regression test added here
goes through `extractHelmComponentSection`/`extractComponentSections` — the layer that
actually dropped the field.

## Validation

- `go test ./internal/exec/ -run TestExtractHelmComponentSectionCreateNamespace -v` — the new
  test fails on the pre-fix whitelist (reproduces the drop) and passes after adding the key.
- `go test ./internal/exec/ -run TestMergeComponentConfigurations_CreateNamespacePrecedence -v`
  — proves component value > stack-default, and stack-default applies when the component is unset.
- `go test ./internal/exec/ -run 'Helm'` — passes.
- Empirical CLI run against a copy of `examples/helm` with `create_namespace: false` on the
  component: `atmos describe stacks -s dev --components demo --component-types helm --sections
  create_namespace` returned `create_namespace: false` (was `{}` before the fix).
- `go build ./...`, `go test ./pkg/config/... ./pkg/component/helm/...` — pass.
- `gofumpt` clean on all changed Go files.

## Follow-ups

- Support `create_namespace` in an `overrides:` block (component, type, and global levels),
  including the `helm_overrides` JSON schema and type/global override propagation. Deferred to a
  separate PR. See "Deliberate non-change: overrides block" above.
