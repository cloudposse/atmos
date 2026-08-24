# Fix: Terraform CI planfile upload resolves workdir paths

**Date:** 2026-07-02

## Problem

`atmos terraform plan --ci` could silently skip planfile upload for
workdir-provisioned components.

Terraform writes the generated planfile in the component working directory. For
source-provisioned components with `provision.workdir.enabled`, that directory is
under `.workdir`. The CI upload hook rebuilt component information with
`ProcessStacks` and then constructed the planfile path from the source component
directory, so it checked the wrong location and treated the missing file as a
non-fatal skipped upload.

## Fix

When the CI upload hook has to derive the planfile path, it now detects
workdir-enabled Terraform components and resolves the effective workdir path
from first principles before constructing the planfile path.

This mirrors the existing workdir-aware behavior in plan verification and
plan-diff, where post-processing code also has to recover the execution
directory after `ProcessStacks` rebuilds component configuration without the
runtime `_workdir_path` value.

## Tests

```shell
go test ./pkg/ci/plugins/terraform -count=1
go test ./tests -run 'TestTerraformPlanCIUploadAndPlanfileList|TestJITSource_MetadataComponentSubpath' -count=1
```
