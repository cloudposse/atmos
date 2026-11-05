# Fix: Qualify CloudFormation render offline support

**Date:** 2026-10-16

## Summary

Explain when rendering can run without network access.

## Context

Rendering does not deploy or validate the template through CloudFormation. Source provisioning and configured secret or output lookups can still require authentication and network access.

## Changes

Qualified render help to distinguish local template rendering from authenticated source provisioning or value lookups; regenerated the actual CLI help recording.

## Validation

Go 1.26.9 source build and `go test ./cmd/aws/cloudformation` passed.
The filtered repository screengrab generator recorded the updated help.
Targeted cast validation passed for body color, absence of errors and local paths,
and the corrected offline guidance. The generator's full-library validation
encountered a pre-existing missing-color failure in `atmos-config-get--help.cast`;
the same failure reproduces from the unchanged HEAD recording.
Website prebuild and navigation tests, plus all applicable pre-commit hooks, passed.
The hook run ignored only generated `.context/` scratch files via a temporary Git
excludes file; no tracked files or hooks were skipped.
The production website build passed and includes the exact regenerated cast.

## Follow-ups

None.
