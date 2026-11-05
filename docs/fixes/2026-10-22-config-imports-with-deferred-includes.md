# Fix: Preserve configuration imports alongside deferred YAML includes

**Date:** 2026-10-22

## Summary

Configuration imports load even when another section contains an unresolved `!include`.
Required import failures and invalid import conditions stop configuration loading.

## Context

Import discovery decoded the entire CLI configuration before YAML functions were evaluated.
A scalar include destined for a map, such as an MCP server's environment, caused decoding to fail.
The caller discarded that error and continued without imported configuration, allowing auth
validation to incorrectly succeed when an invalid identity existed only in an imported file.

## Changes

Decode only import-resolution inputs before processing imports. Preserve authentication,
downloader settings, remote-cache options, and stack paths needed by the import adapters.
Propagate resolver errors; optional missing imports retain their existing resolver behavior.
YAML functions still run in their normal configuration-loading phase.
The temporary resolver context preserves native function tags and defers unresolved typed fields,
including whole-section and nested includes in auth, settings, and import options. It retains
static sibling settings without modifying the original configuration or evaluating functions early.

## Validation

Regression tests cover native and quoted includes, inline controls, required and optional missing
imports, invalid conditions, resolver context, whole-section and nested includes, static type
errors, and CLI auth validation. The rebuilt CLI passed
18 isolated runtime invocations covering imported values, invalid auth, required import errors,
provisioned identities, manual override precedence, and remote-cache expiration.

The hook fixture now uses the supported scalar `settings.terminal.pager: false` setting.
Its former map-shaped setting was exposed by strict import-context decoding; the three
component hook regressions are included in validation.

## Follow-ups

None.
