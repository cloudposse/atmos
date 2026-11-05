# Fix: Drain script output after the interpreter exits

**Date:** 2026-10-19

## Summary

External script steps retain stdout and stderr produced by descendants after their interpreter exits. This is a bug fix for every edition.

## Context

The process-group cancellation grace also acted as an output deadline through `exec.Cmd.WaitDelay`. A script that exited successfully while a descendant printed three seconds later lost that output after two seconds, reported success, and started the next step prematurely. Cancellation after the interpreter exited also left an output-holding descendant running.

## Changes

The script runner owns its output pipes and waits for EOF independently of the interpreter's exit. Identical output destinations share one pipe, preserving ordering and avoiding concurrent writes to an unsynchronized writer. Preserve nonzero interpreter exit codes and destination errors.

Keep cancellation active through output draining, coordinate termination so it runs once, and retain the existing bounded process-group cleanup. Close the read handles after canceled cleanup so an escaped output holder cannot stall completion. Output failures cancel the producer. Successful runs wait for complete output; cancellation and configured timeouts interrupt that wait. The general shell runner's existing background-process policy is unchanged.

## Validation

Added portable helper-process tests before implementation. Successful and nonzero interpreters both lost delayed output before the fix; the cancellation test also exposed the surviving child. These tests now pass, alongside tests for shared output ordering and output-destination failure.

The `pkg/io`, `pkg/process`, and `pkg/runner/step` suites, focused race checks, and custom-command/workflow adapter tests passed. Patch lint reported zero issues. CLI reproductions compare v1.232.0 and the fixed binary twice per edition, unpinned and pinned to `2026-10-12`: both delayed streams are present and stdout precedes the following step. Cancellation controls verify that the fixed CLI terminates both child and grandchild.

Local execution records: `.context/regression-fixes/` (before/after test logs, race results, CLI wrapper, and per-invocation observations). Runtime validation was performed on macOS; process-group cancellation tests are Unix-only.

## Follow-ups

None.
