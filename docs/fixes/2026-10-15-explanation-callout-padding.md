# Fix: Add vertical padding inside error explanations

**Date:** 2026-10-15

## Summary

Add one blank colored row above and below terminal error explanation content.

## Context

The shared explanation callout had horizontal padding, but its first and last
text rows touched the colored box's edges.

## Changes

- Extend the first and last line backgrounds into one row of vertical padding.
- Preserve the existing content gradient, width, nested Markdown colors, and
  uncolored output.
- Update existing rendering checks for the padded layout.

## Validation

- Error formatter and policy package tests passed, including full-width colored
  padding, content gradient progression, and unchanged uncolored output.
- The CLI build and patch-scoped lint passed.
- A real policy denial exited 1; its captured ANSI output was rendered with
  Atmos's native cast renderer and visually checked for top and bottom padding.
- Fix-document validation and `git diff --check` passed.

## Follow-ups

None.
