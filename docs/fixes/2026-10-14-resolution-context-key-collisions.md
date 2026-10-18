# Fix: Distinguish hyphenated resolution context nodes

**Date:** 2026-10-14

## Summary

Distinct stack/component pairs no longer produce a false circular dependency error.

## Context

The visited-node key joined stack and component with a hyphen. A dependency from component `c` in stack `a-b` to component `b-c` in stack `a` therefore collided even though it was not a cycle. CodeRabbit identified the ambiguity in PR #3000.

## Changes

Push and Pop now encode the two names as a pair of quoted strings. The exported `Visited` map retains its existing type. A regression checks distinct pairs, real cycles, clone isolation, and removal on Pop. Related review nits consolidate the auth-disabled propagation and source command CLI-name cases into named table tests while preserving their assertions.

## Validation

The new collision regression failed before the fix with a false circular dependency error. Go 1.26.9 focused race tests passed for `internal/exec` and `pkg/provisioner/source/cmd`, covering resolution contexts, auth-disabled propagation, and CLI-name selection. Both affected packages build successfully. Formatting and `git diff --check` passed.

## Follow-ups

None.
