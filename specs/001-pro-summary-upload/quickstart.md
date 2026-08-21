# Quickstart: Testing Pro Summary Upload Locally

**Date**: 2026-06-09

## Prerequisites

- Atmos Pro account with a workspace configured
- A stack with `settings.pro.enabled: true` in its component configuration
- `ci.enabled: true` in `atmos.yaml`
- `ATMOS_PRO_TOKEN` or OIDC configuration set

## Minimal atmos.yaml snippet

```yaml
ci:
  enabled: true

settings:
  pro:
    base_url: https://versioning.atmos.tools
```

## Minimal stack component snippet

```yaml
components:
  terraform:
    vpc:
      settings:
        pro:
          enabled: true
```

## Run a plan with upload

```bash
atmos terraform plan vpc --stack dev-us-east-1 --upload-status
```

**Expected**: The command runs normally, then uploads the status. Check the Atmos Pro dashboard
for the `vpc` component in the `dev-us-east-1` stack to confirm resource counts, warnings, and
the masked output log appear.

## Run an apply with upload

```bash
atmos terraform apply vpc --stack dev-us-east-1 --upload-status
```

**Expected**: Dashboard shows apply outcome and any Terraform output values (sensitive values
shown as `<MASKED>`).

## Verify backward compatibility (no metadata for helmfile)

```bash
atmos helmfile sync myapp --stack dev-us-east-1 --upload-status
```

**Expected**: Upload succeeds with only `command`, `exit_code`, `last_run` — no `component_type`
or `metadata` fields in the payload (confirm via debug logging).

## Enable debug logging to inspect payloads

```bash
ATMOS_LOGS_LEVEL=debug atmos terraform plan vpc --stack dev-us-east-1 --upload-status 2>&1 | grep -i upload
```

## Simulate large output (truncation test)

Generate a large output file and point terraform at it, or run a plan against a component with
hundreds of resources. Confirm `truncated: true` appears in the metadata when output exceeds 3 MB
(pre-encoding).

## Run unit tests for this feature

```bash
# BuildStatusData and extractOutputValues
go test ./pkg/ci/plugins/terraform/... -v -run 'TestPlugin_BuildStatusData|TestExtractOutputValues'

# Registry dispatch
go test ./pkg/ci/... -v -run 'TestBuildStatusData'

# Output capture and upload wiring
go test ./internal/exec/... -v -run 'TestExecuteMainTerraformCommand|TestUpload'

# All related tests fast
make test-short
```
