# Fix: Terraform dry-run provisioning and prerequisite output streams

**Date:** 2026-10-28

## Summary

Dry-run Terraform commands no longer invoke SDK provisioners, refresh source/workdirs,
invalidate initialization markers, or delete plan/variable files. Prerequisite initialization
for output/show/state-pull commands sends diagnostics to stderr.

## Context

Paired field tests reproduced S3 bucket creation during dry-run and invalid JSON stdout during
cold output on both main `7e43ddf900` and reconciled PR tree `1935d330b4`.

## Changes

The typed provisioner execution context carries dry-run intent. Source/workdir preparation
resolves and validates paths without provisioning; init housekeeping and cleanup preserve disk
artifacts. Real provisioning remains independent of the opportunistic engine-init decision.
Data-command prerequisites retain diagnostic/retry capture without entering the caller's data stream.
The merged stderr display and capture destinations share a write lock so concurrent
subprocess stdout/stderr pumps cannot race a caller's diagnostic buffer.

## Validation

Additional tests assert that provider-recovery preparation failures prevent both explicit and
implicit init from launching the engine, dry-run cleanup retains caller plan bytes, shell dry-run
preserves disabled authentication without fetching source, and partial diagnostic streams do not
pollute data stdout. Invalid dry-run source paths still fail before filesystem mutation.

Portable subprocess and provisioner-spy tests pass. On Terraform 1.15.6 and OpenTofu 1.12.5,
two fresh runs per engine kept absent Floci buckets absent after plan/apply/init dry-runs and
preserved existing object-version lists, versioning configuration, and local state/plan hashes.
Normal explicit init and backend provisioning with skipped engine init still work. Cold native
JSON output parses in its entirety after prerequisite initialization on local and Git sources.
Concurrent diagnostic display/capture and real subprocess routing tests pass twice
under the race detector, including a caller sharing one diagnostic buffer.

## Follow-ups

Validation covers isolated S3 emulation; it does not establish parity for other live backend services.
