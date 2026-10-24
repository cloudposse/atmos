# Fix: Honor the stack on custom Atmos command steps

**Date:** 2026-10-24

## Summary

A custom `type: atmos` step passes its rendered `stack:` to the child command.

## Context

On main at `cb23b3f701`, a step with `command: describe component noop` and `stack: dev` failed because the child command received no stack flag. Workflow steps already supported this behavior.

## Changes

Append `-s` when the command has no explicit stack flag and insert it before any passthrough `--`. Keep explicit command-line stack precedence, including pflag's attached short form `-sprod`. An attached flag after `--` remains passthrough data and does not suppress the step stack. Cover custom command and workflow execution using Go subprocess helpers, and clarify the shared step documentation and agent guidance.

## Validation

Regressions failed before the fix. Ran `go test ./cmd ./pkg/workflow -run "TestCustomCommand.*Stack|TestWorkflow.*Stack|TestAppendAtmosStepFlags" -count=1` and package lint on the isolated PR. Changed-code lint, `go build ./...`, and the combined website build passed. Tests require no external CLI or services.

The attached-short regression failed before its fix: `-sprod` incorrectly became `-sprod -s dev`. A temporary parser contract check and inspection of the local pflag dependency verified attached values and passthrough handling. The custom-command argument table covers both attached stack selection and attached flags after `--`.

## Follow-ups

None.
