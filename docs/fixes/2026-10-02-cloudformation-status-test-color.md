# Fix: CloudFormation status validation tests under forced color

**Date:** 2026-10-02

## Summary

Assert the structured valid-status hint instead of terminal-rendered error text.

## Context

The previously published CloudFormation hardening tip failed its acceptance shard
on Linux, macOS and Windows. Forced-color formatting inserted ANSI sequences into
status names, breaking substring assertions despite the correct error and hint.

## Changes

Read the error's structured hints for the known-status assertions. Retain the
`ErrInvalidFlag` assertion, invalid-input cases and normalization checks.

## Validation

The failure was reproduced with forced colors. The focused status-validation test
passes with forced colors after the change, without changing application behavior.

## Follow-ups

None.
