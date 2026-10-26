# Fix: TUI styled-output test ignores ambient color-disabling env

**Date:** 2026-07-02

## Problem

`TestPrintStyledTextToSpecifiedOutput` forced color output with
`ATMOS_FORCE_COLOR=1` and then asserted that styled text was written to the
provided buffer.

In environments with `NO_COLOR=1`, the helper correctly treated color as
disabled and returned without writing. The test therefore failed because it
inherited ambient color-disabling environment variables.

## Fix

The test now clears `NO_COLOR`, `CLICOLOR_FORCE`, and `FORCE_COLOR` while
setting `ATMOS_FORCE_COLOR=1`.

This keeps the test focused on forced output to the supplied writer instead of
the developer or CI environment's terminal color preferences.

## Tests

```shell
go test ./internal/tui/utils -run TestPrintStyledTextToSpecifiedOutput -count=1
```
