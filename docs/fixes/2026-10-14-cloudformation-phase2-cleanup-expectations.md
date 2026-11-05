# Fix: Place preview cleanup expectations in the phase 2 layer

**Date:** 2026-10-14

## Summary

Place preview cleanup expectations in the phase 2 layer.

## Context

CI in #3000 reported unexpected preview deletion and missing deletion calls for retained manual changesets. The corrections already existed in #3001, leaving #3000 independently broken.

## Changes

Move the preview mock expectations into #3000. Diff expects deletion and stub inspection after its final poll; explicit changeset creation retains its review artifact.

## Validation

Focused race tests for operation dispatch and preview template packaging passed.

## Follow-ups

Verify the published commit in CI before considering the stack complete.
