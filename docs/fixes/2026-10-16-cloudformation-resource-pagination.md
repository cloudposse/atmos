# Fix: Stop resource pagination at an empty next token

**Date:** 2026-10-16

## Summary

Stop resource pagination at an empty next token.

## Context

CloudFormation tree and log discovery could repeat the first resource page when AWS returned an empty pagination token.

## Changes

Treat nil and empty NextToken values as terminal. Extend pagination tests to assert page inputs, ordered resources, and termination for single and multiple pages.

## Validation

Regression reproduced against the original code. Focused race tests, the full Go build, and all applicable pre-commit checks passed with Go 1.26.9.

## Follow-ups

None.
