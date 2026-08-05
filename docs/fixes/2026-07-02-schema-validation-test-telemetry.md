# Fix: schema validation dogfood test disables telemetry

**Date:** 2026-07-02

## Problem

`TestTestCaseSchemaValidation` dogfoods the real
`atmos validate schema` command by invoking `cmd.Execute()` in-process.

That also exercised command telemetry. In environments where telemetry is
enabled, the test could hang while the PostHog client closed and waited on
network I/O. This made a schema validation test depend on external network
behavior.

## Fix

The schema validation dogfood helper now sets
`ATMOS_TELEMETRY_ENABLED=false` before invoking the real CLI command.

The test still exercises the command path, config loading, schema matching, and
failure handling. It only removes the unrelated telemetry network side effect.

## Tests

```shell
go test ./tests -run TestTestCaseSchemaValidation -count=1
```
