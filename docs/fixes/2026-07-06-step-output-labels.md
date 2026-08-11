# Fix: step output labels can be suppressed

**Date:** 2026-07-06

## Problem

Raw and log output modes printed step labels and completion footers by default
without a way for a step to suppress them. That made command output harder to
use in scripts and in generated output flows where only the command output
should be displayed.

## Fix

Step output writers now honor `show.labels`. Labels remain enabled by default,
but setting labels to false suppresses step headers and footers for raw and log
output modes.

## Tests

```shell
go test ./pkg/runner/step -run 'TestOutputModeWriterRawLabelsEnabledByDefault|TestOutputModeWriterRawLabelsCanBeDisabled|TestOutputModeWriterLogLabelsCanBeDisabled|TestShowLabels' -count=1
```
