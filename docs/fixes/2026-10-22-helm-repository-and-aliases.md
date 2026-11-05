# Fix: Honor explicit Helm repositories and documented command aliases

**Date:** 2026-10-22

## Summary

An explicit Helm repository can render a chart when an unrelated configured repository is unavailable. Native Helm supports `render` as an alias for `template`; Helmfile supports `deploy` as an alias for `apply`.

## Context

The Helm executor fetched all configured repository indexes before resolving a chart, even when the component supplied its own repository URL. Both documented aliases were absent from command registration.

## Changes

Skip unrelated repository setup for explicit repository URLs unless dependency updating was requested. Register aliases on the canonical commands so flags and execution behavior stay shared.

## Validation

A regression verifies explicit repository execution does not call unrelated repository setup. Existing repository/dependency-update tests pass. Live HTTP chart rendering succeeds twice with an unrelated repository pointing at a closed local port, and the render alias produces the same manifests. Helmfile alias checks execute the native Helmfile binary against an empty local release file; matching empty-selection behavior verifies dispatch without claiming cluster deployment coverage.

## Follow-ups

Dependency updates still initialize declared repositories because chart dependencies may reference them.
