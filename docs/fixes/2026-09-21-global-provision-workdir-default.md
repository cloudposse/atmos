# Fix: Honor the global `settings.provision.workdir.enabled` default

**Date:** 2026-09-21

## Summary

The documented global default `settings.provision.workdir.enabled` (set in `atmos.yaml`) was
ignored — only the component-level `provision.workdir.enabled` was honored. The global default is
now layered into every component's `provision` section as the lowest-precedence base, so it enables
workdir provisioning for all components unless a component (or stack) overrides it. Fixes
cloudposse/atmos#3197.

## Context

`workdir.IsWorkdirEnabled` reads the component's merged `provision.workdir.enabled` from the
component config section. The component `provision` section is assembled in
`mergeComponentConfigurations` (`internal/exec/stack_processor_merge.go`) by merging, lowest to
highest precedence: the component-type global (`terraform.provision`), the base component, the
component, and overrides. That is why `terraform.provision.workdir.enabled` worked.

The atmos.yaml `settings.provision` block (typed as `schema.AtmosSettings.Provision`) lives outside
that merge and was never projected into any component's `provision` section — `atmosConfig.Settings`
is only consumed for terminal/template settings. So the documented global default
(`/stacks/components/provision/workdir#global-defaults`) had no effect, even though the schema and
docs describe it as working.

## Changes

- `internal/exec/stack_processor_provision.go` (new) —
  - `globalWorkdirProvisionDefaults(atmosConfig)` projects the atmos.yaml
    `settings.provision.workdir` block (`enabled`, `ttl`) into a `provision` map, returning `nil`
    when nothing is set (so an unset global never introduces a spurious `enabled: false` base).
  - `mergeComponentProvision(...)` merges the component `provision` section and prepends that global
    default as the lowest-precedence layer.
- `internal/exec/stack_processor_merge.go` — the inline provision merge now calls
  `mergeComponentProvision`. (Extracting to a new file keeps this already-large file from growing —
  it is 8 lines shorter than before.)
- `tests/fixtures/scenarios/workdir-global-default/` (new) — atmos.yaml sets
  `settings.provision.workdir.enabled: true`; `vpc-global` declares no component-level provision,
  `vpc-opt-out` sets `provision.workdir.enabled: false`.
- `internal/exec/stack_processor_global_provision_test.go` (new) — integration tests that
  `vpc-global` inherits the global default (`enabled: true`) and `vpc-opt-out` overrides it
  (`enabled: false`), plus a unit test for `globalWorkdirProvisionDefaults`.

Precedence is unchanged for everything already working: component/stack-level `provision.workdir`
(including an explicit `enabled: false`) overrides the global default.

## Validation

```bash
# Integration test fails before the fix (provision.workdir.enabled absent for vpc-global),
# passes after; precedence test confirms component-level override wins.
go test ./internal/exec/ -run TestGlobalWorkdirProvisionDefault -v

# No regressions in the merge/provision/describe paths.
go build ./...
go test ./internal/exec/ -run 'Merge|Provision|Workdir|DescribeComponent|Inheritance'
go test ./pkg/provisioner/...
```

All listed tests pass. The integration test was verified to fail with the merge injection disabled
(the resolved component had no `provision.workdir.enabled`) and to pass with it in place.

## Follow-ups

None. `settings.provision` also carries `default` and `targets` (target provisioning) global
defaults; those are resolved separately and are intentionally out of scope for this workdir fix.
