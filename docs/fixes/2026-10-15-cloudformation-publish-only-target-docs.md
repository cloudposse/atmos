# Fix: Distinguish template uploads from direct deployment

**Date:** 2026-10-15

## Summary

Clarify that selecting the example S3 target uploads a template without deploying a stack.

## Context

CodeRabbit identified ambiguous delivery-target guidance in PR #3156. The example described a publish-only S3 target as useful for large templates without distinguishing upload from deployment.

## Changes

Separate selecting `--target packaged` for an upload from automatic S3 packaging during a direct deployment. Explain how to select direct deployment.

## Validation

The updated MDX compiled successfully. All 32 command-intro, example-documentation, and sidebar tests passed after linking the existing website dependencies into the isolated worktree. The fix-document validator and `git diff --check` passed. A full website build remains to be run after integration into the final stack.

## Follow-ups

Commit once Git signing is available and propagate through the stack, preserving the later expanded delivery-target documentation.
