# Fix: `describe component` schema filter now surfaces all stack-definable sections

**Date:** 2026-09-25

## Summary

`atmos describe component` (with the default `describe.component.filter: schema`) dropped several
sections that a stack manifest can legitimately define. For native Helm components it hid `values`
and `chart` - the primary configuration a user wants to inspect - and for all component types it
hid `secrets`, `generate`, `auth`, `command`, `backend_type`, `provision`, `retry`, `workspace`,
`remote_state_backend`, `required_version`, and others. `atmos describe stacks` does not apply this
filter, so the same component showed the full set of sections there. The `schema` filter now removes
only the internal fields Atmos computes and keeps every user-definable section, so the default
`describe component` output is consistent with `describe stacks`. Fixes cloudposse/atmos#3218.

## Context

`FilterComputedFields` (`internal/exec/describe_component.go`) scopes the default
`describe component` output. It ran only when the filter was the default `schema`
(`describe.component.filter`, journaled in `pkg/edition`); `full` restores every computed field,
and any `--query` always runs against the full, unfiltered section.

The function used an **allowlist** of 12 fields to keep and dropped everything else. That allowlist
had drifted: its own inline `NOTE` already admitted it was missing real sections (`retry`,
`generate`, `auth`, `secrets`, `command`, `backend_type`, `workspace`), and it never included the
Helm `values`/`chart` sections at all. Every time a new user-definable section was added to Atmos,
the allowlist silently hid it until someone remembered to update the list - exactly how `flags`
came to be missing (added later in PR #2992) and how `values`/`chart` were missing here.

## Changes

- `internal/exec/describe_component.go`:
  - Replaced the keep-allowlist in `FilterComputedFields` with a **denylist**,
    `atmosComputedFields`, of the internal fields Atmos computes while resolving a component:
    identity/provenance (`atmos_cli_config`, `atmos_component`, `atmos_stack`, `atmos_stack_file`,
    `atmos_manifest`, `stack`, `component_info`, `inheritance`, `sources`), the resolved
    dependency graph (`deps`, `deps_all`), derived CLI plumbing (`cli_args`, `tf_cli_vars`,
    `env_tf_cli_args`, `env_tf_cli_vars`), integration-derived identity (`atlantis_project`,
    `spacelift_stack`), and internal managed-workdir bookkeeping (`_workdir_path`,
    `_workdir_reprovisioned`, `_workdir_subpath_applied`). Constants from `pkg/config` and
    `pkg/provisioner/workdir` are reused where they exist. Note: the singular `source` section is
    NOT denied - it is stack-definable component configuration (JIT-vendored/remote component
    source, and it participates in base-component inheritance), distinct from the computed
    `sources` list of resolved source files.
  - The filter now keeps everything else, so all user-definable sections (including Helm
    `values`/`chart`, `secrets`, `generate`, `auth`, `command`, `backend_type`, `provision`,
    `retry`, `workspace`, and any future section) surface by default without needing the list
    updated. Effective-config fields that Atmos derives but a user cares about (`workspace`,
    `component_type`, `backend`, `backend_type`) are intentionally NOT denied.

## Why a denylist

The `schema` filter exists to hide Atmos bookkeeping, not to curate a hand-maintained set of
"real" sections. A denylist of computed fields is stable: the set of things Atmos adds while
resolving a component is small and well-known, whereas the set of sections a manifest can define
grows over time. Inverting the check makes new sections visible by default and eliminates the
allowlist-drift class of bug that produced this issue (and #2992 before it). This matches the
alternative called out in the issue.

## Behavior change

`workspace` was previously dropped by the schema filter. It is effective configuration a user cares
about (and `describe stacks` shows it), so it is now kept. This is the only field that flips from
"hidden" to "shown" beyond the newly surfaced user sections; the existing unit test that asserted
`workspace` was removed was updated accordingly.

## Validation

```bash
# Unit tests (updated + new cases: Helm values/chart kept, workdir keys denied, workspace kept).
go test ./internal/exec/ -run 'TestFilterComputedFields|TestDescribeComponentFilter|TestDescribeComponentWithProvenance' -count=1

# End-to-end: the default schema filter now surfaces the previously dropped sections.
# Terraform component (tests/fixtures/scenarios/atmos-terraform-flags):
atmos describe component testcomponent -s dev --format json | jq 'keys'
#   now includes: auth, backend_type, command, generate, provision, workspace, ...
#   still hides:  atmos_cli_config, atmos_component, cli_args, component_info, deps, inheritance, sources, stack, tf_cli_vars

# Native Helm component (tests/fixtures/scenarios/helm-lifecycle):
atmos describe component demo -s dev --format json | jq 'keys'
#   now includes: chart, values, release, namespace, secrets, ...

# CLI golden snapshots regenerated for the affected default-filter cases.
go build ./...
atmos lint --changed
```
