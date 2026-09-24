# Fix: Provision local-component workdir before file generation (backend race)

**Date:** 2026-09-21

## Summary

For a **local** Terraform component (no JIT `source:`) with `provision.workdir.enabled: true`,
Atmos generated `backend.tf.json` and the `*.tfvars.json` varfile into the shared **source**
component directory (`components/terraform/<component>/`) instead of the per-run workdir. This
polluted the source tree and, under `atmos terraform plan --all --max-concurrency N`, caused
parallel runs to read/write the same source `backend.tf.json` concurrently, producing HCL parse
errors like `Missing value: The JSON data ends prematurely`. Fixes the race reported in
cloudposse/atmos#3192.

## Context

The Terraform working directory is resolved by `constructTerraformComponentWorkingDir`
(`internal/exec/path_utils.go`): it returns the workdir when `info.ComponentSection[WorkdirPathKey]`
is set, and otherwise falls back to the source component directory. Backend and varfile generation
run in `prepareComponentExecution` → `runPreExecutionSteps`
(`internal/exec/terraform_execute_helpers_exec.go`), which resolves the working dir up front via
`resolveAndProvisionComponentPath`.

The bug was one of **ordering**:

- `resolveAndProvisionComponentPath` → `ProvisionAndResolveComponentPath`
  (`pkg/component/workdir_path.go`) set `WorkdirPathKey` early only for components with a JIT
  `source:` (via `AutoProvisionSource`, which downloads directly into the workdir).
- For a plain **local** component, the workdir copy is performed by the `workdir` provisioner
  (`pkg/provisioner/workdir`), which was registered to run at the **`before.terraform.init`** hook —
  i.e. **after** backend/varfile generation had already happened.

So for local components `WorkdirPathKey` was unset at generation time, `constructTerraform...WorkingDir`
returned the source dir, and the generated files were written there. The later workdir copy then
picked them up (explaining the accumulation of stray varfiles), and concurrent `--all` runs collided
on the same source `backend.tf.json`.

JIT-`source:` components were unaffected because their workdir is set before generation.

## Changes

- `pkg/component/workdir_path.go` — `ProvisionAndResolveComponentPath` now calls
  `provWorkdir.ProvisionWorkdir(...)` **up front** (gated to `cfg.TerraformComponentType`), before the
  no-source short-circuit and before any file generation, and (in the no-source branch) resolves the
  `metadata.component` subpath onto the freshly provisioned workdir and returns it. The Terraform gate
  matters because this helper is shared by Helmfile/Packer/Ansible while `ProvisionWorkdir` builds a
  terraform-specific workdir path (and the `before.terraform.init` hook that also runs it fires only
  for Terraform) — so an unconditional call would resolve a local non-Terraform component through a
  terraform workdir. `ProvisionWorkdir` is otherwise self-gating: it no-ops unless
  `provision.workdir.enabled: true` and the component has no JIT source, and it no-ops when
  `WorkdirPathKey` is already set — so the existing `before.terraform.init` run of the same
  provisioner remains a safe, idempotent skip (and the `-reconfigure` signal it records persists on
  the shared `ComponentSection`).
- `pkg/component/workdir_path_test.go` — added
  `TestProvisionAndResolveComponentPath_LocalComponentWorkdirProvisionedEarly`, a regression test that
  provisions a local component with workdir enabled and asserts the resolved path is under
  `.workdir/terraform/` (not the source dir), that `WorkdirPathKey` is set, and that the component
  files were synced into the workdir.

## Validation

```bash
# New test fails before the fix, passes after.
go test ./pkg/component/ -run TestProvisionAndResolveComponentPath_LocalComponentWorkdirProvisionedEarly -v

# No regressions in the component/provisioner packages, the exec provision path, or the CLI workdir suite.
go build ./...
go test ./pkg/component/ ./pkg/provisioner/...
go test ./internal/exec/ -run 'Provision|Workdir|Backend|ComponentPath'
go test ./tests/ -run 'Workdir'
```

All listed tests pass. The new test failed before the change (resolved to the source directory with
no `WorkdirPathKey`) and passes after.

## Follow-ups

The issue also notes that the documented **global** default
`settings.provision.workdir.enabled: true` does not take effect — only the component-level
`provision.workdir.enabled` is honored by `IsWorkdirEnabled` (`pkg/provisioner/workdir/workdir.go`),
which reads the component's merged `provision` section rather than `settings.provision`. That is a
separate config-wiring gap from this ordering/race fix and is tracked in its own issue,
cloudposse/atmos#3197.
