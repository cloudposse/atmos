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
  lifecycle default, and is left unset (so the reader default applies) when the key is absent.

### Deliberate non-changes

- `helmOverrideSectionKeys` (the `overrides:` block whitelist) was **left narrow** — it still
  accepts only `values`. Its comment states the intent to keep it narrow "until other Helm
  fields have documented override semantics," so allowing `create_namespace` there is a
  separate, documented decision rather than part of this bug fix. Setting `create_namespace`
  directly on a component or as a stack-level `helm:` default both work; setting it inside an
  `overrides:` block is intentionally not supported yet.

## Why the original PR's tests missed it

#3034's unit tests exercised `resolveCreateNamespace`/`buildChartSpec` by passing a section
map with `create_namespace` already populated. They never ran through the stack processor's
extraction whitelist, which is what strips the key at runtime. The regression test added here
goes through `extractHelmComponentSection`/`extractComponentSections` — the layer that
actually dropped the field.

## Validation

- `go test ./internal/exec/ -run TestExtractHelmComponentSectionCreateNamespace -v` — the new
  test fails on the pre-fix whitelist (reproduces the drop) and passes after adding the key.
- `go test ./internal/exec/ -run 'Helm'` — passes.
- `go build ./...`, `go test ./pkg/config/... ./pkg/component/helm/...` — pass.
- `gofumpt` clean on all changed Go files.

## Follow-ups

- Consider whether `create_namespace` should also be accepted in `overrides:` blocks
  (`helmOverrideSectionKeys`); currently intentionally excluded.
