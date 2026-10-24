# Fix: Filter Terraform CLI Arguments from Internal Output Commands

**Date:** 2026-08-24

## Summary

Post-apply output-store hooks now ignore `TF_CLI_ARGS` and `TF_CLI_ARGS_*` values while retrieving Terraform outputs.

## Context

Terraform-exec rejects manually configured command-specific CLI argument environment variables. Atmos filtered inherited process variables but restored those values from the resolved component environment, causing `after-terraform-apply` output-store hooks to fail after a successful apply.

## Changes

The Terraform output environment no longer copies `TF_CLI_ARGS` or `TF_CLI_ARGS_*` entries from component configuration. Other component environment variables, including required `TF_VAR_*` values, are preserved. A regression test covers apply and plan arguments alongside allowed variables.

## Validation

`go test ./pkg/terraform/output -run '^TestDefaultEnvironmentSetup_ComponentCLIArgsFiltered$' -count=1` passed after the fix.

`go test ./pkg/terraform/output -count=1` and `go build ./...` passed with the ambient `TF_PLUGIN_CACHE_DIR` removed because unrelated existing tests require it to be unset.

`atmos test` did not complete: unrelated tests observed the host `AWS_DEFAULT_REGION`, and the CLI suite timed out after five minutes while inspecting fixture state.

## Follow-ups

None.
