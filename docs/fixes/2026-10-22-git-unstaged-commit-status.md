# Fix: Distinguish unstaged Git changes from a clean commit no-op

**Date:** 2026-10-22

## Summary

A commit with an empty index and dirty working tree fails with guidance to stage changes or specify managed paths.

## Context

The Git provider checked only the index, returned the same result as a clean repository, and the CLI incorrectly printed “working tree is clean.”

## Changes

Check working-tree status when no staged changes exist. Return a distinct error for unstaged/untracked changes, preserving explicit staging and managed-path safety. A genuinely clean tree remains a successful no-op.

## Validation

Provider tests cover unstaged and untracked changes and verify no implicit staging occurs. Live disposable-repository checks repeat the unstaged failure, commit an explicit path, and verify a clean no-op afterward.

## Follow-ups

The command continues to commit the index when no managed paths are specified.
