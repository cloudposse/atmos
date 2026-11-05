# Fix: Clarify interactive source command arguments

**Date:** 2026-10-16

## Summary

Clarify when CloudFormation source commands require explicit component and stack values.

## Context

The source describe and pull reference pages called both arguments unconditionally required, although the shared source commands prompt for missing values in interactive mode.

## Changes

Describe component and stack selection when interactive candidates are available. Keep unattended requirements explicit and preserve the `ATMOS_STACK` alternative.

## Validation

Compared both pages with `pkg/provisioner/source/cmd/describe.go`, `pull.go`, and the parent source guide. Documentation checks are recorded in the workspace validation evidence.

## Follow-ups

None.
