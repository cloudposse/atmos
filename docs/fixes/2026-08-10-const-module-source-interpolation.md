# Fix: Support `const` interpolation in Terraform module sources

**Date:** 2026-08-10

## Summary

Updated `terraform-config-inspect` so Atmos can load Terraform components whose local `module.source` interpolates a static `const` variable.

## Context

Earlier versions of `terraform-config-inspect` rejected these module sources with a `Variables not allowed` diagnostic, causing `describe component` to fail before Terraform or OpenTofu could process a valid configuration.

## Changes

- Updated `github.com/hashicorp/terraform-config-inspect` to `v0.0.0-20260709150029-2fb54c236733`.
- Added a regression fixture and exec tests for a static variable interpolated in a local module source.
- Added coverage confirming genuine HCL syntax errors still return `ErrFailedToLoadTerraformComponent`.
- Updated the OpenTofu interpolation regression test to assert parsed Terraform configuration uses `*tfconfig.Module`.

## Validation

- Ran `git diff --cached --check` successfully.
- Could not run `gofumpt`, focused exec tests, `go build ./...`, or `atmos test` because the Go toolchain is unavailable in the execution environment.

## Follow-ups

None.
