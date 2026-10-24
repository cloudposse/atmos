# Fix: custom command env accepts map form

**Date:** 2026-07-06

## Problem

Command-level custom command `env` supported the legacy list form, but did not
reliably accept the map form already used by workflow steps:

```yaml
env:
  PATH: '{{ env "PWD" }}/bin:{{ env "PATH" }}'
  FROM_COMMAND:
    valueCommand: printf value
```

That made command-level env harder to share with step env and included command
configuration.

## Fix

Command env decoding now accepts both the legacy list form and map form. String
values become normal env entries, object values can set fields such as
`valueCommand`, and map keys are sorted for deterministic decoding.

## Tests

```shell
go test ./pkg/schema ./pkg/config -run 'TestCommandEnvDecodeHook_MapValues|TestDecodeCommandEnvMapValueDecodeErrorUsesSentinel|TestCommandEnvFromMapEntryVariants' -count=1
```
