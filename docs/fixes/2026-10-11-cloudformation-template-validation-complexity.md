# Fix: Separate CloudFormation template validation

**Date:** 2026-10-11

## Summary

Reduce function complexity through a behavior-preserving helper extraction.

## Context

Component validation combined template selection, inline-template validation, and stack-name validation. Later layers add more checks, causing the function to exceed the updated complexity limit.

## Changes

Extracted validateComponentTemplate at the earliest layer with file-template detection. Template errors still precede missing stack-name errors; abstract and nil components still bypass validation.

## Validation

Using Go 1.26.9 at this owning layer:

- `go test -race ./pkg/component/aws/cloudformation -count=1 -run '^TestValidateComponent|^TestIsTemplatePresent|^TestSanityCheck|^TestHasFileTemplate'`: passed (5.313s). The coverage-enabled run gives both the component and extracted template validation functions 100% statement coverage.
- `go build ./pkg/component/aws/cloudformation/...`: passed.
- Custom golangci-lint with `--new-from-rev=HEAD ./pkg/component/aws/cloudformation/...`: zero issues.
- Formatting, `git diff --check`, and fix-record validation passed.

## Follow-ups

None.
