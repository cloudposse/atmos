# Fix: Export usable MCP client configuration files

**Date:** 2026-10-22

## Summary

MCP export creates missing client directories and writes Codex TOML when the output filename ends in `.toml`.

## Context

The documented Cursor/Gemini paths failed when their parent directories did not exist. Exporting to `config.toml` wrote JSON, which Codex could not parse.

## Changes

Create output parents with private permissions and preserve the existing private file mode. Select TOML serialization for `.toml` output, with Codex's `mcp_servers` and `http_headers` keys. JSON output remains the default. Both formats use the shared server builder, preserving auth wrappers and environment handling.

## Validation

Tests cover directory creation, an unwritable parent, valid TOML parsing, local commands, remote HTTP headers, case-sensitive environment keys, and existing JSON behavior. Live export checks parse both formats and assert the output file mode is 0600.

## Follow-ups

Export writes a standalone configuration; merging with an existing client configuration remains the install command's responsibility.
