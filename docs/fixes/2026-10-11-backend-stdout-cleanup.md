# Fix: Restore backend test output after callback failure

**Date:** 2026-10-11

## Summary

Restore backend test output after callback failure.

## Context

A failed callback could leave global stdout pointing at a closed pipe.

## Changes

Defer restoring stdout and closing both pipe ends before invoking the callback. Add a panic regression that verifies restoration and subsequent capture.

## Validation

The full backend package race suite passed with Go 1.26.9 (2.898 seconds), including the callback-panic regression. Changed-code lint reported zero issues.

## Follow-ups

None.
