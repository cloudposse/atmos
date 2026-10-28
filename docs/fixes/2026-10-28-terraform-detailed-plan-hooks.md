# Fix: Run success hooks for Terraform plans with changes

**Date:** 2026-10-28

## Summary

Detailed plan exit 2 runs success/always after-hooks once, retains external exit 2 and enforces required hook failures.

## Context

Fresh tests twice per main/reconciled revision and engine showed a plan with changes running its failure hook and skipping its default success hook. Lifecycle documentation defines after-hooks as success-only by default.

## Changes

Single-component and graph-node hook outcomes classify plan exit 2 as success, while retaining the raw exit code for CI and the external command. Policy errors remain failures. The single-component plan defer uses the result-aware success path because Cobra skips PostRunE on nonzero status; required after-hook errors propagate instead of being logged and discarded. Other nonzero command statuses remain failures. This restores the existing success-hook contract without changing configuration defaults or adding an edition transition.

## Validation

Focused command tests passed for unchanged/changed/failed plans, non-plan exit 2, policy errors, and actual required success-hook failure with a negative failed-engine control. Both real engines passed twice: before/success hooks fired once, failure hook did not fire, exit stayed 2. A required after-hook failure returned 1 on both engines; same/different plan-diff controls retained 0/2.

Live evidence and exact command records are retained under `.context/terraform-repair-20261028/contracts/`; `*-isolated-*` cases explicitly use per-fixture encrypted file keyrings.

## Follow-ups

Final integrated validation, website build and affected help-cast regeneration are coordinated in the approved Terraform field-findings repair plan. Real cloud IAM and destructive production state operations were not tested.
