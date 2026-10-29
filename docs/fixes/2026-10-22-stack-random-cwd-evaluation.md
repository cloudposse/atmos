# Fix: Evaluate `!random` and `!cwd` in stacks

**Date:** 2026-10-22

## Summary

Stack manifests now evaluate `!random` to an integer and `!cwd` to the invocation's working directory, including an optional relative suffix.

## Context

Issue #3477 reported that both functions silently reached component variables as strings. The stack loader deferred them, but the component evaluator had no handlers. Configuration and scaffold loaders already supported both functions.

## Changes

The component evaluator delegates to the existing random and working-directory implementations. Both functions obey the existing skip flags and complete-tag matching rules. The shared string-form catalog now includes them, keeping doctor classification aligned with runtime behavior.

## Validation

- Reproduced the original issue with the unmodified baseline binary: `describe component` returned `"!random 49152 65535"` and `"!cwd"`.
- Added a regression test that failed before the fix and checks numeric range/type, cwd and suffix resolution in nested lists, skip behavior, and the existing invalid-range error sentinel.
- Ran focused Go tests covering random/cwd handlers, the string-form catalog, literal processing, and generic config/workflow loaders.

## Follow-ups

None.
