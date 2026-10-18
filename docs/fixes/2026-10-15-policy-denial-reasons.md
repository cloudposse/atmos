# Fix: Display optional author guidance when a policy denies execution

**Date:** 2026-10-15

## Summary

Display the existing optional `reason` field in CLI denial errors and the policy
library, so policy authors can explain a restriction and recommend an alternative.

## Context

Policy manifests already accepted `reason` and retained it in decision findings,
but enforcement returned only the generic denial error. The library also omitted
this guidance from its policy overview and expected-decision display.

## Changes

- Attach each blocking rule's nonempty reason and rule ID to the CLI explanation.
  Preserve the denial sentinel and default error when no reason is supplied.
- Keep reasons literal, without template expansion or input interpolation; do not
  show reasons from nonmatching rules or replace evaluation errors with guidance.
- Display the selected implementation's reason on its library page and on blocked
  fixture decisions, omitting empty guidance.
- Give both `!exec` implementations an explanation and a suggested `!env`
  alternative. Document the optional field in the schema, guides, and PRD.

## Validation

- All policy package tests passed, including both engines, optional reasons,
  multiple denials, nonmatching rules, literal text, and input nondisclosure.
- A real CLI invocation exited 1, displayed the reason and alternative, and did
  not execute the blocked shell command.
- Website prebuild checks passed (14 tests).
- The Go build and production website build passed.
- Browser checks passed for both engines, omitted reasons, nonmatching cases,
  mobile layout, and no page errors; captured the overview and example views.
- Patch-scoped lint, fix-document validation, and `git diff --check` passed.

## Follow-ups

None.
