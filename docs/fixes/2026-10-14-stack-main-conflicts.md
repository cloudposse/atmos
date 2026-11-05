# Fix: Stack integration after main advanced

**Date:** 2026-10-14

## Summary

Reconcile the CloudFormation stack with the shared CI and Starlark changes on main.

## Context

The stack and main independently changed loopback fixtures, bootstrap installation, SHA verification, and README generation.

## Changes

Keep main’s loopback fixtures, checksum-verifying bootstrap installer, file-based SHA diagnostics, and tokenless README template download. Preserve bounded SHA quota recovery and service restoration after runner hardening. Remove a duplicated fixture environment key and retain the comment step’s independent retry policy.

## Validation

The strict CLI fixture loader passes. All 23 SHA verifier regressions, 15 bootstrap regressions, and eight service-restoration tests pass. Changed test/native-CI workflows pass actionlint. The pinned README generator succeeds without a token.

## Follow-ups

Finish signed publication and verify CI on each final head under the existing serial publication schedule.
