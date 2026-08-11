# Fix: help output renders without a valid config

**Date:** 2026-07-06

## Problem

Help output could depend on loading `atmos.yaml`. When config was missing or
invalid, some help requests could fail before rendering the command help the
user requested.

Help wrapping also varied in CI or piped output when terminal width was inferred
from environment rather than a real terminal.

## Fix

Help requests are detected early and tolerate config-load failures that should
not block help rendering. Help layout uses a stable default width when no real
terminal width is available, while still respecting configured max width after
configuration is available.

## Tests

```shell
go test ./cmd ./pkg/terminal ./pkg/ui -run 'Test.*Help|TestGetTerminalWidth|TestWidth_NonTTYFallback|TestTerminalWidth' -count=1
```
