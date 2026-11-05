# Fix: Preserve template cancellation in CloudFormation output reads

**Date:** 2026-10-10

## Summary

CloudFormation output reads made by `atmos.Component` now inherit the template caller's cancellation and deadline while retaining the five-minute request timeout.

## Context

`AtmosFuncs.Component` dropped its existing context before resolving a component. The CloudFormation output getter then created a timeout from `context.Background()`, so a canceled template or an expired caller deadline could still start a fresh five-minute output request.

## Changes

- Pass the caller context through the private component helper and CloudFormation output resolver.
- Derive the request timeout from that context, preserving earlier deadlines and cancellation during the provider call.
- Keep the context-free YAML evaluator's existing bounded request behavior.
- Update private-helper test callers and add deterministic template-path regressions for cancellation, deadlines, timeout bounds, and child-context cleanup.

## Validation

- Confirmed the new template-path regression fails before the fix: canceled and expired callers reach the getter with an uncanceled five-minute context.
- Go 1.26.9 affected `internal/exec` race tests passed in 35.137 seconds.
- Go 1.26.9 builds of `internal/exec` and `pkg/aws/cloudformation` passed.
- Changed-code lint passed with zero findings against the prepared #3000 head.
- All four changed executable production lines were covered by the affected tests.
- Formatting, whitespace, and fix-record checks passed.

## Follow-ups

None.
