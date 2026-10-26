# Fix: focused deferred-resolution regression tests

**Date:** 2026-10-25

## Summary

Split `TestConcreteResolution` into eight focused tests while preserving its behavior coverage.

## Context

PR review identified a roughly 130-line test covering several independent resolution behaviors. Test files are exempt from the configured `funlen` linter, but separate tests make the scenarios easier to locate and maintain.

## Changes

Give surviving values, replaced values, replacement types, concrete precedence, list strategies, disabled templates, surviving errors, and scalar replacement their own test functions. Each function is at most 32 lines. Keep explicit YAML source values, all 26 assertions in the file, and all 14 mock expectations unchanged.

## Validation

The focused concrete-resolution tests passed before the refactor, and `go test ./pkg/deferred -count=1` passed afterward. Direct `custom-gcl` lint of `./pkg/deferred/...` passed with zero issues. A structural comparison confirmed the assertion and mock-expectation counts and the length of each extracted test.

The branch-wide `atmos lint --changed` command reported an unrelated complexity warning in `cmd/secret/set.go`. That code is unchanged from this PR's merge base; newer `main` has already refactored the function. No secrets code was changed for this test cleanup.

## Follow-ups

None.
