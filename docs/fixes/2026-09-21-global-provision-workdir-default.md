# Fix: Global workdir default belongs in stack config, not `settings.provision`

**Date:** 2026-09-21

## Summary

Issue #3197 reported that the documented global default `settings.provision.workdir.enabled` (set
in `atmos.yaml`) was ignored. The resolution, per maintainer review, is that a workdir global
default does not belong under `atmos.yaml` `settings` at all: provisioning is component
configuration, so its global default belongs in the stack configuration under the toolchain section
(`terraform.provision`, `helmfile.provision`, etc.), consistent with global `vars`, `metadata`, and
`secrets`. That stack-level mechanism already works. This change removes the non-functional
`settings.provision.workdir` surface (schema, JSON Schema, docs) and adds regression tests plus
corrected documentation for the stack-level default. Fixes cloudposse/atmos#3197.

## Context

`workdir.IsWorkdirEnabled` reads the component's merged `provision.workdir.enabled` from the
component config section. That section is assembled in `mergeComponentConfigurations`
(`internal/exec/stack_processor_merge.go`) by merging, lowest to highest precedence: the
component-type global (`terraform.provision`, extracted as `opts.GlobalProvisionSection`), the base
component, the component, and overrides. Because the stack-level `terraform.provision` block is the
lowest-precedence layer, a stack-wide default declared there already cascades to every component and
is overridden by any component-level `provision.workdir` (including an explicit `enabled: false`).

The `atmos.yaml` `settings.provision.workdir` block (typed as `schema.ProvisionSettings.Workdir`)
lived outside that merge and was never projected into any component's `provision` section, so it had
no effect - the behavior #3197 reported. The initial fix attempt injected `settings.provision` into
the merge. Maintainer review rejected that: the correct location is the stack configuration, not
`settings`. The dead `settings.provision.workdir` config surface is therefore removed rather than
wired up.

## Changes

- `pkg/schema/schema.go` — removed the `Workdir` field from `ProvisionSettings` and deleted the
  now-unused `ProvisionWorkdirSettings` type. `ProvisionSettings` retains `default`/`targets`
  (delivery targets), which are unrelated to the workdir default. Added a doc comment stating the
  workdir global default belongs in the stack `terraform.provision` section.
- `pkg/datafetcher/schema/atmos/config/1.0.json` — regenerated via `go generate ./pkg/config/schema`
  (drops the `settings.provision.workdir` definition). The component-level `provision.workdir`
  definitions in the stack-config and manifest schemas are unchanged.
- `internal/exec/stack_processor_merge.go` — reverted the provision merge to the inline four-layer
  merge (`opts.GlobalProvisionSection` → base component → component → overrides), with a comment
  noting the global layer is the stack-level `terraform.provision`. The earlier
  `stack_processor_provision.go` extraction is removed.
- `website/docs/stacks/components/provision/workdir.mdx`,
  `website/docs/stacks/components/provision/index.mdx`,
  `website/docs/stacks/components/provision/backend.mdx` — the "Global Defaults" guidance now points
  to the stack-level `terraform.provision` section; removed the `settings.provision.workdir`
  examples and the links to the deleted settings page.
- `website/docs/cli/configuration/settings/provision.mdx` — deleted. It documented only the
  non-functional `settings.provision.workdir` global (including a three-level precedence that was
  never implemented).
- `tests/fixtures/scenarios/workdir-global-default/` — the global default is declared as stack-level
  `terraform.provision.workdir.enabled: true` (in `stacks/deploy/dev.yaml`), not in `atmos.yaml`.
  `vpc-global` declares no component-level provision; `vpc-opt-out` sets
  `provision.workdir.enabled: false`.
- `internal/exec/stack_processor_global_provision_test.go` — regression tests that `vpc-global`
  inherits the stack-level default (`enabled: true`) and `vpc-opt-out` overrides it
  (`enabled: false`).

## Validation

```bash
# Regression tests: vpc-global inherits terraform.provision.workdir.enabled: true;
# vpc-opt-out's component-level enabled: false overrides it.
go test ./internal/exec/ -run TestGlobalWorkdirProvisionDefault -v

# Schema drift guard passes with the regenerated config JSON Schema.
go test ./pkg/config/schema/ ./pkg/schema/

# No regressions in the merge/provision/describe paths.
go build ./...
go test ./internal/exec/ -run 'Merge|Provision|Workdir|DescribeComponent|Inheritance'
go test ./pkg/provisioner/...

# Website builds with no broken links after deleting the settings page.
cd website && npm run build
```

All listed tests pass.

## Follow-ups

None. `settings.provision` still carries `default` and `targets` (delivery-target provisioning),
which are resolved separately and are intentionally out of scope for this workdir fix.
