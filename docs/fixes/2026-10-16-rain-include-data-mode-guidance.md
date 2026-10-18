# Fix: Clarify Rain include data mode

**Date:** 2026-10-16

## Summary

Distinguish Atmos YAML-function evaluation from CloudFormation intrinsic rewriting.

## Context

The generic warning that tags were dropped contradicted the preceding intrinsic guidance.

## Changes

Scoped the warning to Atmos YAML functions and stated that CloudFormation intrinsic rewriting does not require eval.

## Validation

Verified the referenced documentation and behavior against the current source.
The fix-record validator and applicable pre-commit hooks passed.

## Follow-ups

None.
