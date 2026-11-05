# Fix: Make the !exec policy easy to find

**Date:** 2026-10-15

## Summary

Name the existing YAML shell-execution policy "Deny !exec" so its purpose is clear
in the library catalog and navigation.

## Context

The previous title, "Disable YAML shell execution", obscured that the library
already includes a rule denying `!exec` in both CEL and Starlark.

## Changes

- Update the policy title and heading without changing its URL or enforcement.
- Name the blocked fixture explicitly after `!exec` and clarify that the `!env`
  fixture does not match this rule, rather than implying a global allow decision.

## Validation

- The catalog fixtures passed for both CEL and Starlark.
- All 14 website prebuild checks and the production site build passed.
- Browser checks passed for the title, desktop/mobile navigation, engine
  switching, and blocked/nonmatching fixture decisions, with no page errors.
- Patch-scoped lint, fix-document validation, and `git diff --check` passed.

## Follow-ups

None.
