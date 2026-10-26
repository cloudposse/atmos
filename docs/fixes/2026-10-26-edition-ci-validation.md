# Fix: edition website tests and merged skill size

**Date:** 2026-10-26

## Summary

Fix the website preview dependency error and the agent-skill size failure.

## Context

The edition generator tests resolved `@docusaurus/mdx-loader` from the website root even though it was only a transitive dependency. A clean pnpm install could not resolve it. Separately, GitHub tests the PR merged with main: additional Starlark guidance made that merged skill 20,542 bytes, exceeding the 20,480-byte limit even though the branch alone passed.

## Changes

Declare `@mdx-js/mdx` as a development dependency at the version already present in the lockfile and import its compiler directly. Preserve the full hook guidance in `references/hook-context.md`, with a summary and link in the Starlark skill. The merged skill is now 19,137 bytes.

## Validation

An isolated frozen-lockfile pnpm install reproduces the missing undeclared loader, while all four updated edition tests pass. All validation shell steps from the failing skill workflow pass against its exact merged checkout with the fix applied. The production website build passed, including all 117 prebuild tests. Affected EditorConfig validation and `git diff --check` passed. The required branch lint run reports the existing complexity finding in the unchanged `cmd/secret/set.go` function `findGlobalSetContext`.

## Follow-ups

None.
