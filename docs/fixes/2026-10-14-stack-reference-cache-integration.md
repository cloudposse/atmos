# Fix: Preserve component cache behavior when integrating reference caching

**Date:** 2026-10-14

## Summary

Combine upstream reference-cache partitioning with the stack's target and delivery authentication handling.

## Context

The upstream cache update conflicted with CloudFormation's requirement to resolve target credentials before reading cached outputs. Using a newly allocated auth-manager wrapper as part of the cache scope also prevented repeated references with identical credentials from reusing results.

## Changes

- Retain configuration-root, invocation, and inspection partitions from the shared reference-cache key.
- Scope component and delivery output caches by resolved credentials rather than transient manager wrappers.
- Keep delivery authentication ahead of CloudFormation cache access, and keep deferred CloudFormation entries separate from other component entries.
- Pass the existing operation context in newly inherited cache tests and use the shared key in the cache-logging fixture.

## Validation

- Reproduced repeated-provider-call failures in the existing identity-cache and AWS-file isolation regressions before the credential-scope correction.
- Focused `internal/exec` race tests passed on Go 1.26.9, covering component output reuse, credential isolation, reference-cache secret isolation, deferred authentication, and resolution contexts.
- Ran `git diff --check` and the fix-document validator.

## Follow-ups

None.
