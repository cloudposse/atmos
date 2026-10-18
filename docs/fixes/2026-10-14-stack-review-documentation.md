# Fix: Clarify PR mergeability and CloudFormation secret declarations

**Date:** 2026-10-14

## Summary

Correct the stack mergeability guidance and include `secrets` in the CloudFormation PRD's shared component properties.

## Context

GitHub checks each PR head against its configured base. The stack skill incorrectly described this as simulating a cascade onto current `main`. The PRD also omitted `secrets` from its property list despite documenting component-scoped secret declarations.

## Changes

- Distinguish GitHub's current base/head comparison from `gh stack rebase`, which updates the ancestry of every layer.
- Require matching current commit IDs before interpreting conflicting local and GitHub results, and preserve the warning that `gh stack sync` pushes branches.
- Include `secrets` in the PRD's schema contract. The implementation layer already accepts the property; its regression belongs with that implementation.

## Validation

- Inspected the current PR's configured base and head using GitHub's API.
- Inspected the CloudFormation schema definitions in the implementation layer.
- Ran `git diff --check` and the fix-document validator.

## Follow-ups

None.
