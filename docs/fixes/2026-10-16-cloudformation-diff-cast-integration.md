# Fix: Preserve later CloudFormation flags in the corrected diff screengrab

**Date:** 2026-10-16

## Summary

Regenerate the `diff --help` recording after integrating the corrected render guidance with later CloudFormation flags.

## Context

The earlier help fix corrected the statement that render always stays local. Its recording predates the `--identity` and `--skip-hooks` flags shown by this layer. Selecting either conflicting recording would discard part of the current help.

## Changes

Record the actual merged CLI through the repository's filtered screengrab workflow. The result retains both flags and explains that render can provision sources or resolve secrets and outputs through authenticated remote services.

## Validation

- The filtered generator built and recorded the merged CLI successfully.
- Decoded output contains both flags and the corrected guidance, contains ANSI styling, and excludes local filesystem paths and the obsolete claim.
- The subsequent full-library cast validator still reports the previously documented unrelated missing-ANSI recording; the regenerated target passes its focused checks.
- Final combined website validation is recorded separately in the stack integration evidence.

## Follow-ups

None.
