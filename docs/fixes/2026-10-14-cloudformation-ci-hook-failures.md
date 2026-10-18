# Fix: Report CloudFormation hook failures in native CI summaries

**Date:** 2026-10-14

## Summary

CloudFormation operations now produce a failed native CI summary when hook discovery or a before hook fails, and retain after-hook failures alongside an operation failure.

## Context

The executor returned early on hook setup and before-hook errors before dispatching the CI summary. When both the operation and an after hook failed, the summary recorded only the operation error. CodeRabbit reported both gaps in PR #3100.

## Changes

The executor prepares the after-event CI payload before discovering hooks and dispatches it once when leaving the function. Early hook errors preserve their original return value and do not run user after hooks. Normal completion joins operation and after-hook errors for the CI payload while preserving the operation error as the command's return value.

## Validation

New tests failed before the fix for missing setup/before-hook summaries and the omitted after-hook failure. Focused Go 1.26.9 race tests passed for hook execution, native CI dispatch, and CI-mode detection. Regressions use real hook discovery and local missing-binary failures, verify that user after hooks do not run on early failure, and cover after-hook-only failure, combined failure, and success. The CloudFormation package builds successfully. Formatting and `git diff --check` passed.

## Follow-ups

None.
