# Fix: Update the AWS Signin authorization reference link

**Date:** 2026-10-15

## Summary

Point the browser authentication PRD at the current AWS Signin service authorization reference.

## Context

The Link Check workflow returned HTTP 404 for the old `list_awssignin.html` URL. A separate request redirected that obsolete URL to the generic reference index, which did not contain the intended service documentation.

## Changes

Replace the obsolete path with `list_signin.html`, preserving the reference label and surrounding PRD. Normalize the two nested fenced examples to four-space list indentation so the touched document satisfies the existing two-space EditorConfig rule.

## Validation

- The replacement URL returned HTTP 200 without redirecting to another page.
- The response contains the AWS Signin authorization reference title and `CreateOAuth2Token` action.
- The fix-document validator and `git diff --check` passed.
- Affected-file Atmos validation and the full applicable pre-commit hooks passed, including EditorConfig.

## Follow-ups

Verify Link Check on the final published stack commits.
