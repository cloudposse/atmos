# Fix: Preserve MCP subprocess environment variable names

**Date:** 2026-10-22

## Summary

MCP server environment keys retain their original spelling, including distinct uppercase and lowercase names.

## Context

The local MCP field test configured `FIELDTEST_MCP_LOG` but the actual subprocess received `fieldtest_mcp_log`. Its uppercase lookup failed, preventing the initialization handshake. An argv-based control completed initialization, tool discovery, ping, and restart, isolating the failure to environment configuration.

## Changes

Extend the configuration loader's existing encoded-key preservation to `mcp.servers.<name>.env`. Restore the typed environment maps after Viper decoding. The transform operates only on dictionary keys and does not evaluate YAML functions again. Update the MCP configuration documentation to describe preserved spelling.

## Validation

The loader regression checks imports and overrides, distinct `UPPER`/`upper` keys, `!env` values, aliases, and separately included environment maps. Full tests for `pkg/config`, `cmd/mcp/client`, and `pkg/mcp/client` pass. A real local Python stdio server receives both differently cased keys, completes initialization/tools/ping, and restarts twice. All six server subprocesses exit after their CLI commands. Baseline and fixed logs are retained in the field-test ledger.

## Follow-ups

A separate preexisting loader problem remains: a main configuration containing a typed `env: !include` value can make the early full-configuration decode skip unrelated configuration imports. The field-test report records this independently; the case-preservation tests exercise imports and includes separately.
