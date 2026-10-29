# Fix: Validate local paths at the plugin archive boundary

**Date:** 2026-10-28

## Summary

Check archive entry names with `filepath.IsLocal` before exposing them through plugin bundle inventories, and reject Windows drive paths and alternate data streams on every host.

## Context

CodeQL reported archive entry names flowing through the bundle inventory to filesystem operations. The existing custom validator already rejected parent traversal and absolute slash paths, but its validation was not recognized along this flow. It also allowed colon-containing names on Unix, including Windows drive-qualified paths.

## Changes

Add an explicit local-path guard at the archive reader boundary. Keep the stricter canonical-path, hidden-file, entry-type, and credential-material checks. Reject colon-containing paths in the shared validator so packing and inspection agree across platforms. Preserve legal filenames containing consecutive dots.

## Validation

- Full `pkg/plugin/oci` tests pass with the race detector, including hostile archive rejection before extraction.
- Added cases cover drive-absolute paths, drive-relative paths, alternate data streams, and legal consecutive dots within filenames.
- CodeQL's Go path-traversal library models `filepath.IsLocal` as a sanitizer guard. Hosted CodeQL analysis must confirm the reported flow is eliminated; no alert suppression was added.

## Follow-ups

Confirm hosted CodeQL analysis passes on the published stack head.
