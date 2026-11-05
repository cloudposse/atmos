# Fix: Describe MCP export formats and literal environment values accurately

**Date:** 2026-10-22

## Summary

The AI skill describes output-extension format selection and distinguishes explicitly configured server environment values from credentials resolved through Atmos Auth.

## Context

The skill referred to a nonexistent export `--format` flag and claimed exported client configurations could not contain secrets. Export intentionally preserves server environment values, so that blanket claim was incorrect.

## Changes

Document `.toml` output as Codex format and other output extensions as JSON. Explain that explicitly configured environment values are preserved, while identity-based credentials resolve at runtime through the auth wrapper.

## Validation

Actual JSON and TOML export controls verify their structure and output permissions. Two fresh exports preserve a harmless dummy token marker configured in the server environment. No real credential is used.

## Follow-ups

None.
