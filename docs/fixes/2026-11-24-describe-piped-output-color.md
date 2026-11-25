# Fix: Keep piped describe output free of ANSI color codes

**Date:** 2026-11-24

## Summary

Custom command steps can pipe `atmos describe stacks --format json` into `jq`
from an interactive terminal. Redirected JSON and YAML stay plain even when
`ATMOS_FORCE_COLOR=true`. Direct terminal output still supports highlighting.

## Context

Starting in Atmos 1.234.0, custom commands running in a terminal could pass
`ATMOS_FORCE_COLOR=true` to their child processes. The shared describe output
writer treated forced color as evidence that the child's stdout was a terminal.
When a step piped JSON into `jq`, ANSI escape sequences in the stream caused
`jq: parse error: Invalid numeric literal at line 1, column 2`.
[Issue #3757](https://github.com/cloudposse/atmos/issues/3757)

## Changes

The describe output writer now checks the destination of stdout before choosing
a serializer. It uses the existing plain JSON and YAML serializers for pipes
and redirects. The regression tests enable forced color and check the exact
bytes written for both formats.

## Validation

Reproduced the parse error with Atmos 1.234.1 in a pseudo-terminal. With the
rebuilt CLI, the same custom command printed the component name `demo` both
with and without a pseudo-terminal. Direct terminal JSON retained highlighting.
The focused tests passed with `go test ./internal/exec -run '^TestPrintOrWriteToFile' -count=1`.

## Follow-ups

None.
