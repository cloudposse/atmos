# Fix: Diagnose Windows exception-stack corruption

**Date:** 2026-10-15

## Summary

Compare stock Go 1.26.9 with a diagnostic overlay that increases the Windows
exception-stack reserve from 4 KiB to 16 KiB. This is an isolated diagnostic
branch, not a change to the PR stack or release toolchain.

## Context

The Windows 2022 runner pin did not prevent the recurring free-object runtime
crash. Current jobs on #3250 and #3324 failed on Intel Xeon Platinum 8573C hosts
in different packages. Upstream [Go issue 81238](https://github.com/golang/go/issues/81238)
describes exception-context writes exceeding Go's reserve on some AMX hosts.
The dynamic-reserve fix in [CL 828724](https://go.dev/cl/828724) is still under review.
The matching hardware and crash signature are evidence to investigate, not
proof that every Atmos crash has the same cause.

## Changes

- Measure the Windows exception-context size and record CPU, OS, and toolchain.
- Compare stock and modified binaries on each host with a recovered nil-write
  stress probe; retain both successful and failed outcomes.
- Check the exact Go version and source expression before creating the overlay.
  The installed toolchain source is never overwritten.
- Run the unchanged acceptance suite with baseline, race, and checkptr settings,
  plus focused runtime regressions, using the diagnostic reserve.
- Preserve the existing runner hardening policy and bounded diagnostic artifacts.

## Validation

Workflow validation with actionlint and `git diff --check` passed locally.
The first diagnostic run exposed an oversized repetition budget: ten acceptance
repetitions exceeded the 15-minute timeout on three AMD hosts. Both stress
binaries passed on those hosts, whose measured context size was 1,663 bytes.
The corrected harness uses three repetitions per mode and also runs stock
acceptance tests on the same host. Local probe execution and a Windows cross-build
with the overlay passed. Corrected Windows execution is pending. The diagnostic
cannot establish a mitigation unless it captures a stock failure and successful
modified runs on the same host.

## Follow-ups

Inspect the Windows results before preparing any stack change. Do not count
non-AMX-only success, hide stock failures, or retry failed stack jobs blindly.
