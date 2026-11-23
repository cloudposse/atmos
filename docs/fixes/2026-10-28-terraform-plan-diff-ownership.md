# Fix: Preserve caller plans during plan-diff

**Date:** 2026-10-28

## Summary

`plan-diff` preserves its caller-owned saved plans and cleans only temporary files it created.

## Context

Twice per engine and revision, a known-good default plan and a byte-identical copy were compared. Prerequisite init deleted the default plan, making comparison fail. The external-plan copy helper could also overwrite a same-named file in the component directory.

## Changes

Plan-diff marks the canonical caller plan as supplied during its nested init, preventing unrelated cleanup from deleting it. External plans use uniquely created temporary files with cleanup on copy/close failure; directory membership uses path-relative boundaries rather than a string prefix. Different plans now return the existing detailed exit 2 instead of exiting within the library, allowing deferred cleanup. No configuration defaults or edition interpretation changed.

## Validation

Focused component tests passed for caller-plan preservation, sibling-prefix paths, relative/absolute inputs, same-name collisions, owned-copy cleanup, same/different comparison status and unchanged source bytes. The existing plan-diff/deployment-verification policy matrix also passes with explicit returned difference-sentinel and exit-2 assertions. Real Terraform 1.15.6 and OpenTofu 1.12.5 passed identical comparison twice with exact saved-plan hashes retained; additional distinct-plan comparisons returned 2 and retained both files.

Live evidence and exact command records are retained under `.context/terraform-repair-20261028/contracts/`; `*-isolated-*` cases explicitly use per-fixture encrypted file keyrings.

## Follow-ups

Final integrated validation, website build and affected help-cast regeneration are coordinated in the approved Terraform field-findings repair plan. Real cloud IAM and destructive production state operations were not tested.
