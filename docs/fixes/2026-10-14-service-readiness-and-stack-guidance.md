# Fix: Require service readiness and clarify stack guidance

**Date:** 2026-10-14

## Summary

Require a healthy Docker service before CI tests proceed, and clarify two stack workflow instructions.

## Context

A running registry process may not yet accept requests. The recovery action previously treated a
missing Docker health check as healthy. Review also identified a nonportable temporary-file template
and an omitted description of successful no-change applies in the CloudFormation PRD.

## Changes

- Add an HTTP health check for the local registry and reject services without a Docker health check.
- Preserve stopped-container recovery, the running-state requirement, bounded waits, diagnostics,
  and the runner hardening policy.
- End the documented `mktemp` template with its `X` placeholders so macOS randomizes the filename.
- Specify that recognized no-change applies/deploys delete their changeset without executing it;
  other failed changesets remain errors.

## Validation

- Reproduced the missing-health-check bug with a failing regression, then passed all 11 recovery tests.
- Passed real Docker registry readiness and stopped-container recovery using the configured HTTP probe.
- Inspected the exact pinned Linux/AMD64 Floci image configurations: all three define health checks.
- Passed actionlint for the affected test workflow and the native CI caller, plus `git diff --check`.
- Verified the corrected temporary-file template creates a randomized filename on macOS.

## Follow-ups

None.
