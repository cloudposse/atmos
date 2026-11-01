# Fix: Validate local paths at the plugin archive boundary

**Date:** 2026-10-28

## Summary

Check archive entry names with `filepath.IsLocal` before exposing them through plugin bundle inventories.

## Context

CodeQL reported archive entry names flowing through the bundle inventory to filesystem operations. The existing custom validator already rejected parent traversal and absolute slash paths, but its validation was not recognized along this flow. Colon-containing names remain ordinary file names on Unix and are rejected on Windows.

## Changes

Add an explicit local-path guard at the archive reader boundary. Keep the stricter canonical-path, hidden-file, entry-type, and credential-material checks. Preserve legal filenames containing consecutive dots.

## Validation

- Full `pkg/plugin/oci` tests pass with the race detector, including hostile archive rejection before extraction.
- Added cases cover legal consecutive dots within filenames.
- CodeQL's Go path-traversal library models `filepath.IsLocal` as a sanitizer guard. Hosted CodeQL analysis must confirm the reported flow is eliminated; no alert suppression was added.

## Follow-ups

Confirm hosted CodeQL analysis passes on the published stack head.
