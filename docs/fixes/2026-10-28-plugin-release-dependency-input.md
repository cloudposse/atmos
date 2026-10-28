# Fix: Advance plugin metadata for changed dependency inputs

**Date:** 2026-10-28

## Summary

Advance auth-secrets to 1.0.2 when the runtime fixes change the declared root dependency manifest.

## Context

The official plugin release planner treats root `go.mod` as a shared build input. The runtime changes correctly classify `github.com/pelletier/go-toml/v2` as a direct dependency, but that changes an input relative to the preceding stack layer. The planner therefore rejects an unchanged plugin version, even though this edit does not change the dependency version.

## Changes

Advance the authoritative plugin version from 1.0.1 to 1.0.2 and update the description test and package README pin. Keep the direct dependency declaration and release input policy intact. The preceding layer's 1.0.1 bump covers its OCI path validation change; this layer needs its own increase against that parent.

## Validation

The old committed comparison reproduced the release planning error; the preceding layer was a passing control. Run the package tests and committed-head release plans for each final stack layer before publication. Full cross-platform package checking remains part of CI.

## Follow-ups

Verify the official plugin package check against the published head.
