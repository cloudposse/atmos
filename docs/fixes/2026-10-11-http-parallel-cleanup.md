# Fix: Keep HTTP fixtures alive through parallel subtests

**Date:** 2026-10-11

## Summary

Register HTTP fixture shutdown with testing cleanup so fixtures remain available until parallel descendants complete.

## Context

Rebasing the stack onto current main enables stricter lint checks for the changed code.

## Changes

Replace the parent and nested test server deferred closes with t.Cleanup callbacks.

## Validation

Go 1.26.9 targeted HTTP mismatch/retry race tests passed (2.399 seconds).

## Follow-ups

None.
