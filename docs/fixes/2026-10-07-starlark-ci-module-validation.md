# Fix: Validate CI module docs against committed capabilities

**Date:** 2026-10-07

## Summary

Correct the formatting and documentation dependencies that blocked CI for the Starlark CI module.

## Context

The fix-log skill used three-space continuation indentation where EditorConfig requires multiples of two. The ci.path reference linked to and used fs.resolve before that function and its reference page were committed.

## Changes

- Use four-space continuation indentation in the fix-log instruction.
- Give ci.path an absolute directory directly or through ctx.args, and link to the committed argument reference.
- Validate in an isolated checkout so unrelated, uncommitted filesystem changes cannot hide missing dependencies.

## Validation

- Affected-file schema and EditorConfig validation passed using a fresh Atmos build and the PR base.
- Both revised examples ran successfully; the build example produced a binary and rejected a missing directory argument.
- The production website build, including broken-link checking, passed in the isolated checkout.
- Skill structure, fix-record structure, and whitespace checks passed.

## Follow-ups

None.
