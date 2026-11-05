# Fix: Display interactive step prompts with masking enabled

**Date:** 2026-10-19

## Summary

Custom-command and workflow shell steps display prompts without trailing newlines when their input is a terminal. This is a bug fix for every edition.

## Context

The step output wrappers and shell handler constructed streaming maskers without the operation's input. Default regex masking held the unfinished line, so a command printing `PROMPT> ` before reading input appeared blocked. Disabling masking worked around the problem but removed secret protection.

## Changes

Carry the actual input through raw, scoped, and viewport output wrappers and the shell handler. Reuse the existing terminal-aware masking options: interactive output does not hold unfinished regex lines, while registered literal secrets remain protected across writes. Commands with no input or piped input retain line buffering, including external script interpreters. Explicit line-oriented log output keeps its existing behavior.

## Validation

Added real-terminal tests before implementation. All four prompt cases failed before the fix and passed afterward. Additional tests cover split literal secrets on both streams and strict regex masking with nonterminal input under a terminal parent.

The `pkg/io`, `pkg/process`, and `pkg/runner/step` suites passed, as did focused race checks and custom-command/workflow adapter tests. Patch lint reported zero issues. CLI reproductions compare the fixed binary with v1.232.0 using isolated fixtures, both unpinned and pinned to edition `2026-10-12`, repeated twice. Default masking, configured patterns, structured steps, and the masking-disabled control all display the prompt and finish after input.

Local execution records: `.context/regression-fixes/` (before/after test logs, race results, CLI wrapper, and per-invocation observations).

## Follow-ups

None.
