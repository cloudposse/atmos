# Fix: Restore the documented workflow directory override

**Date:** 2026-10-22

## Summary

`atmos workflow --workflows-dir` selects the directory used for explicit file selection and automatic discovery.

## Context

Skills and workflow configuration docs advertised a flag implemented by the legacy config loader but missing from the workflow command registration. The actual command rejected it.

## Changes

Register the flag through StandardParser and preserve its parsed value before initializing CLI configuration. Document its precedence over the environment and configuration.

## Validation

The live workflow fixture rejects the documented flag on two runs before the fix. Command tests execute actual shell steps from a directory containing spaces, for both explicit file selection and discovery, with conflicting config and environment defaults; both pass.

## Follow-ups

None.
