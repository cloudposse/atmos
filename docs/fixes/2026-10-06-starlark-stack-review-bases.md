# Fix: Review every Starlark stack layer automatically

**Date:** 2026-10-06

## Summary

Enable CodeRabbit automatic reviews for each base branch in the Starlark PR stack.

## Context

CodeRabbit's automatic-review configuration allowed `main` at the bottom of the
stack and only some intermediate bases in later layers. Updates targeting the
remaining branches therefore needed manual review requests.

## Changes

List each actual stack base explicitly in the review configuration. Keep the
existing review rules, required change-request workflow, and draft policy.

## Validation

- Compared the allowlist with the base branches reported by GitHub for all ten PRs.
- Checked the patch for whitespace errors.
- Automatic review results will be verified after the configuration reaches
  each layer.

## Follow-ups

None.
