# Fix: Reject dynamic imports during static inspection with custom delimiters

**Date:** 2026-10-26

## Summary

Static stack inspection recognizes the configured template delimiters before
resolving imports, including imports marked `skip_if_missing`.

## Context

The static loader rejected `{{` expressions but missed expressions using custom
delimiters such as `[[` and `]]`. An optional dynamic import could consequently
be skipped as a missing literal path, returning incomplete backend configuration.

## Changes

Use the configured delimiters for the static import guard and reject invalid
delimiter settings. Reject dynamic imports before lookup or optional-import handling.
Also correct the vendoring example's registry comment to match its generic OCI URL.

## Validation

The custom-delimiter optional-import regression failed before the fix and passes
afterward. Focused static inspection tests cover default and custom delimiters,
optional imports, literal paths, and malformed delimiter configuration.

## Follow-ups

None.
