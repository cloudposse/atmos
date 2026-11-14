# Fix: Cursor marketplace link check

**Date:** 2026-11-11

## Summary

Keep the Markdown link check reliable when the Cursor marketplace rejects the CI HTTP/2 client.

## Context

Two consecutive CI attempts failed with an HTTP/2 protocol error for the same marketplace URL; a local HTTP/2 request returned 200.

## Changes

Exclude only the verified marketplace URL from lychee checks; other Cursor URLs remain checked.

## Validation

The endpoint returned HTTP 200 locally. The lychee configuration and the affected PRD were checked locally.

## Follow-ups

None.
