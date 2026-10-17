# Fix: Preserve the component base directory diagnostic

**Date:** 2026-10-09

## Summary

Path-based component resolution now retains the specific error and navigation hint when a path
points to a configured component base directory instead of a component.

## Context

`ExtractComponentInfoFromPath` discarded `ErrPathIsComponentBase` while trying each component type,
then reported that the path was outside every component directory. Returning immediately would
also be incorrect: one type's base can be a valid component directory under another type's base.

## Changes

- Preserve the first component-base error and return it only if no component type matches.
- Cover Terraform, Helmfile, and Packer base directories, unrelated paths, and overlapping bases
  where a later component type must still resolve successfully.
- Regenerate the existing CLI error snapshot using the acceptance harness.

## Validation

- All three base-directory regressions failed before the fix and passed afterward.
- Focused path-resolution tests passed normally and with the race detector.
- `go build ./...` and the local CLI build passed.
- Patch-scoped `atmos lint --changed` passed with zero issues.
- The base-directory CLI snapshot was regenerated through `-regenerate-snapshots`.
- CLI acceptance checks passed without regeneration for base-directory errors, paths outside
  component directories, and Helmfile commands given Terraform component paths.
- The consolidated website build is handled on the owning stack branch after these fixes are combined.

## Follow-ups

None.
