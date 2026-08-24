# Fix: custom command includes resolve inside env and step defaults

**Date:** 2026-07-06

## Problem

Custom commands loaded from `atmos.d` could fail to resolve `!include`
expressions inside command-level `env`, step-level `env`, and step `defaults`.
In affected cases, the raw include expression could survive config processing
instead of becoming the selected map or object.

Environment key case could also be lost during decoding, which broke variables
that depend on uppercase names such as `PATH`, `ATMOS_FORCE_COLOR`, and
provider-specific environment variables.

## Fix

The command import pipeline now preserves the preprocessed command data and
restores original environment key case after config loading. Included command
env maps and included step defaults decode into the structured fields used by
custom command execution.

## Tests

```shell
go test ./pkg/config -run 'TestAtmosDCommandStepEnvInclude|TestAtmosDCommandLevelEnvInclude|TestAtmosDCommandStepDefaultsInclude|TestLoadConfigFromCLIArgs_WithCommandStepDefaultsInclude|TestRestoreCommandEnvCase' -count=1
```
