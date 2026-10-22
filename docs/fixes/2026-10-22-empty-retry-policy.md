# Fix: Run empty retry policies once

**Date:** 2026-10-22

## Summary

An empty retry policy now behaves like an omitted policy.

## Context

The broad audit reproduced retry: {} spinning on a permanent workflow failure without a delay or attempt limit. Stack retry decoding already treated an empty mapping as disabled, but the shared executor treated the zero-valued struct as unlimited.

## Changes

Normalize an entirely empty executor policy to the existing single-attempt default. Preserve unlimited attempts for explicitly configured nonempty policies. Explain empty policies and correct invented delay/time-limit defaults in the workflow skill.

## Validation

Fail-before tests reproduce repeated execution and verify error preservation after the fix. Full pkg/retry and pkg/runner/step suites pass. Actual workflows with omitted, empty, explicit one-attempt, and three-attempt policies run twice each; observed attempt counts are 1, 1, 1, and 3 and the original exit code 7 is preserved.

## Follow-ups

None.
