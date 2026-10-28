# Fix: Helmfile dry-run previews without execution setup

**Date:** 2026-10-27

## Summary

`atmos helmfile --dry-run` displays the selected invocation without starting Helmfile or preparing its execution environment. Version previews are visible even when debug logging is disabled.

## Context

The version shortcut ignored dry-run. Passing the flag to the shell executor stopped the subprocess, but the command display remained debug-only and the executor could still create a redirected stderr file. The component path also performed execution preparation and removed an existing varfile during dry-run.

## Changes

Handle previews before authentication, dependency or plugin installation, source provisioning, component hooks, target delivery, and generated-file writes or cleanup. Share argument builders with normal execution, retain disabled/abstract/locked component handling, and display EKS setup and external delivery targets without executing them. Mask sensitive arguments before shell quoting so embedded quotes cannot bypass output masking. Configuration and stack expression evaluation retain their existing behavior.

## Validation

Cross-platform CLI tests use the Go test executable to record subprocess invocation, including normal execution and `--dry-run=false` controls. The original execution regression failed twice before its fix. With corrected CLI output capture, the published follow-up also failed twice: version previews were missing, redirected stderr was created, and component varfiles were deleted. The early preview implementation passes those output and file-preservation regressions, plus component argument ordering and unavailable auth, dependency, source, plugin, and delivery-target cases. EKS and metadata restrictions pass. A synthetic secret containing an apostrophe reproduced the quoting/masking gap twice, with masking-disabled controls passing. Final focused race regressions, the Go build, and patch lint pass after the masking fix and helper refactor.

## Follow-ups

None.
