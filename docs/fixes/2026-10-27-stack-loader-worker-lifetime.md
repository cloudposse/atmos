# Fix: Wait for stack workers before returning an error

**Date:** 2026-10-27

## Summary

Stack loading now waits for every worker to finish before returning, including when
one stack fails to load.

## Context

The first failed worker caused stack loading to return while other workers were
still reading configuration. Callers could then change shared process state while
those workers used it. The CI race detector caught this between an invalid-template
Terraform test and the following auth test's update to `os.Args`.

## Changes

- Join all stack workers on return without changing the first error reported.
- Keep parallel loading and the result buffer that lets every worker finish sending.
- Add a regression that holds one source read while another worker fails, then
  verifies that loading waits for the held worker and returns the original error.

## Validation

- The regression failed before the fix and passed 20 times with the race detector.
- Stack-loader, invalid-template Terraform, and auth identity-error tests passed
  ten shuffled runs with the race detector and the failing CI run's seed.
- Validation used Go 1.26.10, matching CI.

## Follow-ups

None.
