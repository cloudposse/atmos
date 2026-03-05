# Native CI Integration - Generic Provider

> Related: [Overview](../overview.md) | [Interfaces](../framework/interfaces.md) | [CI Detection](../framework/ci-detection.md)

## Overview

The generic CI provider serves as the fallback when no specific provider (GitHub, GitLab, etc.) is detected. It activates when `CI=true` is set in the environment.

## Detection

The generic provider checks for the `CI` environment variable. If `CI=true` and no specific provider is detected, the generic provider is used.

## Context Resolution

The generic provider populates `ci.Context` from environment variables with git fallback:

| Field | Source |
|-------|--------|
| `Provider` | `"generic"` |
| `SHA` | `$CI_COMMIT_SHA` or `git rev-parse HEAD` |
| `Ref` | `$CI_COMMIT_REF` or `git symbolic-ref HEAD` |
| `Repository` | `$CI_REPOSITORY` or git remote origin |
| `Actor` | `$CI_ACTOR` or `$USER` |

## Implementation

Implemented in `pkg/ci/generic.go`. Satisfies the `ci.Provider` interface with minimal feature set — no status checks, PR comments, or job summaries.
