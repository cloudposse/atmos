# Fix: Resolve source configuration using the command's provider

**Date:** 2026-10-09

## Summary

Source pull, describe, and delete now load component configuration for their selected provider, including the component-level authentication used by pull.

## Context

The shared source commands described components without a component type. When Terraform and CloudFormation declared the same component name, the default lookup selected Terraform first. This could display or vendor the wrong source, use the wrong component authentication, or delete Terraform's configured working directory from a CloudFormation source command.

## Changes

- Pass the command's explicit component type to component description and configuration initialization.
- Use the same provider for pull's authentication lookup and provisioning configuration.
- Preserve the existing untyped public `DescribeComponent` and `InitConfigAndAuth` contracts for callers that request automatic component detection.
- Exercise actual mixed-provider manifests through delete, describe, and pull commands, checking the directory removed, displayed source, authentication configuration, and provisioning configuration.

## Validation

- Before the fix, the delete regression removed Terraform's directory and left CloudFormation's directory intact.
- Affected source command packages passed race-enabled tests, including the complete shared source command test suite.

- `go build ./...` passed.
- Patch-scoped custom GolangCI lint reported zero issues.
- Fix-log validation and `git diff --check` passed.

## Follow-ups

None.
