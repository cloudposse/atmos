# Native CI Integration - Generic Provider

> Related: [Overview](../overview.md) | [Interfaces](../framework/interfaces.md) | [CI Detection](../framework/ci-detection.md)

## Overview (IMPLEMENTED)

The generic CI provider serves as the fallback when `--ci` flag is used but no specific provider (GitHub, GitLab, etc.) is detected. It is **never auto-detected** — `Detect()` always returns `false`. It is only activated explicitly by the executor when `forceCIMode` is true and no other provider matches.

## Detection

The generic provider does NOT auto-detect. The executor uses it as a last resort when:
1. `--ci` flag is set (or `CI`/`ATMOS_CI` env var is true)
2. No specific provider (GitHub, etc.) is detected via `Detect()`
3. The executor falls back to `Get("generic")`

## Context Resolution (IMPLEMENTED)

The generic provider populates `ci.Context` from environment variables (no git fallback):

| Field | Source |
|-------|--------|
| `Provider` | `"generic"` |
| `SHA` | `$ATMOS_CI_SHA` or `$GIT_COMMIT` or `$CI_COMMIT_SHA` or `$COMMIT_SHA` |
| `Branch` | `$ATMOS_CI_BRANCH` or `$GIT_BRANCH` or `$CI_COMMIT_REF_NAME` or `$BRANCH_NAME` |
| `Repository` | `$ATMOS_CI_REPOSITORY` or `$CI_PROJECT_PATH` |
| `Actor` | `$ATMOS_CI_ACTOR` or `$CI_COMMIT_AUTHOR` or `$USER` |
| `RepoOwner` | Parsed from `Repository` (before `/`) |
| `RepoName` | Parsed from `Repository` (after `/`) |

Fields NOT populated: `RunID`, `RunNumber`, `Workflow`, `Job`, `EventName`, `Ref`, `PullRequest`.

## Capabilities (IMPLEMENTED)

| Capability | Supported | Details |
|-----------|-----------|---------|
| **OutputWriter** | Yes | Writes to `$ATMOS_CI_OUTPUT` file (KEY=VALUE, heredoc for multiline) or stderr if unset |
| **Summary** | Yes | Writes to `$ATMOS_CI_SUMMARY` file or stderr if unset |
| **Check Runs** | Yes | Synthetic in-memory check runs with `atomic.Int64` IDs, logged via `ui.Infof`/`ui.Successf`/`ui.Errorf` |
| **GetStatus** | No | Returns `ErrCIOperationNotSupported` |
| **PR Comments** | No | Not implemented |

## Implementation

Implemented in `pkg/ci/providers/generic/`:
- `provider.go` — Provider struct, `Context()`, `OutputWriter()`, `Detect()` (always false)
- `check.go` — `CreateCheckRun()`, `UpdateCheckRun()` with synthetic IDs
- `provider_test.go`, `check_test.go` — Tests
