# Fix: Keep skill sizes valid in every PR layer

**Date:** 2026-10-26

## Summary

Apply the Starlark skill size correction at the bottom of the PR stack so each
intermediate PR satisfies the repository's skill validation checks.

## Context

Main added Starlark guidance while the skills audit was under review. The combined
skill exceeded the 20 KB limit. The correction initially lived only in the final
skills PR, leaving the earlier PRs' independent CI checks failing.

## Changes

Move the concise toolchain-dependency explanation into the first layer, retaining
installation policy, PATH behavior, pin conflicts, and tool-name requirements.
All dependent layers inherit the correction.

## Validation

The repository's skill validation steps pass in the first-layer worktree, including
frontmatter, directory structure, line counts, file sizes, and code fences.

## Follow-ups

None.
