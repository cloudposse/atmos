# Fix: terminal color and forced TTY handling

**Date:** 2026-07-06

## Problem

Color and terminal detection could behave inconsistently across TTY, non-TTY,
CI, and forced-output scenarios. In particular, `--force-color`, `--force-tty`,
`NO_COLOR`, `CLICOLOR`, `CLICOLOR_FORCE`, configured terminal color, and terminal
width detection did not all follow the same precedence.

Non-TTY output could also pick up `COLUMNS`, producing unstable help wrapping in
CI and piped output.

## Fix

Terminal capability detection now centralizes color and forced-TTY decisions.
`NO_COLOR` and explicit no-color settings disable color, force-color settings
can enable color without a TTY, force-tty supplies stable fallback dimensions,
and non-TTY width ignores `COLUMNS` unless a caller supplies an explicit
recording width.

## Tests

```shell
go test ./pkg/terminal ./pkg/ui ./cmd -run 'TestForceTTY|TestForceColor|TestShouldUseColor|TestWidth_NonTTYFallback|TestTerminalWidth|TestConfigureEarlyColorProfile|TestGetTerminalWidth' -count=1
```
