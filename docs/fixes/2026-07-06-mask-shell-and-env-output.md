# Fix: masking applies consistently to shell and env output

**Date:** 2026-07-06

## Problem

Secret masking could be initialized before all config and flags were known.
That meant `--mask=false` might not disable a previously initialized masker, and
late-loaded terminal mask settings might not be applied to subprocess output.

`atmos env` also needed different behavior for interactive terminal output than
for file or piped output: interactive display should mask secrets, while machine
readable output should remain raw unless explicitly written through a masked
terminal stream.

## Fix

Masking is reconciled after flags and config are available, shell subprocess
output uses the configured mask writer, and `atmos env` masks only interactive
stdout. File output and non-interactive stdout remain raw so scripts can consume
the requested env data.

## Tests

```shell
go test ./pkg/io ./pkg/env ./cmd/env ./internal/exec -run 'TestReconcileMasking|TestMaskerSetEnabled|TestOutput|TestExecuteShellCommandAppliesAtmosMaskSettingsToSubprocessOutput' -count=1
```
