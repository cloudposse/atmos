# Native CI Integration - Generic Provider

> Related: [Overview](../overview.md) | [Interfaces](../framework/interfaces.md) | [CI Detection](../framework/ci-detection.md)

## Overview (IMPLEMENTED)

The generic CI provider is the local renderer on a workstation and the stand-in provider for unknown CI under forced CI mode. It never posts anything to a remote service. Every CI operation that a real provider would perform remotely (comments, annotations, SARIF upload, log groups, environment export) has a local rendering that is safe to run anywhere.

Its `Detect()` always returns `false` and is unchanged, so it never wins provider detection on its own. Callers reach it in two ways: as the local fallback when no provider matches, and as the detected provider when CI mode is forced.

## Selection

`Detect()` across the registered providers (GitHub, etc.) is unchanged, and the generic provider never matches it. The `ci.Reporter` (`pkg/ci/reporter.go`, `NewReporter`) selects it as follows:

1. If a registered provider is detected, the reporter uses it, with the generic provider as the local fallback for gated or unsupported writes.
2. Otherwise, if CI mode is forced (`--ci`, `ATMOS_CI` truthy, or `CI` truthy), the generic provider becomes the **detected** provider. The `ci.*` gates apply as on a real platform, the context reports `Provider=generic` with `ci.context.local` false, and the `ATMOS_CI_OUTPUT`, `ATMOS_CI_SUMMARY`, `ATMOS_CI_ENV`, and `ATMOS_CI_PATH` files are written when set.
3. Otherwise there is no detected provider and every write renders locally through the generic provider (`Local=true`, `ci.context.local` true).

`ci.ResolveProvider()` (`pkg/ci/registry_provider.go`), which returns the detected provider or the generic one, remains the helper for callers that need only a provider to run on, such as the native plugin executor under `--ci`. The Reporter does not use it.

## Context Resolution (IMPLEMENTED)

The generic provider populates `ci.Context` from environment variables. `SHA` and `Branch` fall back to the local git repository when no variable is set (`Branch` is empty on a detached HEAD):

| Field | Source |
|-------|--------|
| `Provider` | `"generic"` |
| `SHA` | `$ATMOS_CI_SHA` or `$GIT_COMMIT` or `$CI_COMMIT_SHA` or `$COMMIT_SHA` |
| `Branch` | `$ATMOS_CI_BRANCH` or `$GIT_BRANCH` or `$CI_COMMIT_REF_NAME` or `$BRANCH_NAME` |
| `Repository` | `$ATMOS_CI_REPOSITORY` or `$CI_PROJECT_PATH` |
| `Actor` | `$ATMOS_CI_ACTOR` or `$CI_COMMIT_AUTHOR` or `$USER` |
| `RepoOwner` | Parsed from `Repository` (before `/`) |
| `RepoName` | Parsed from `Repository` (after `/`) |
| `ServerURL` | `$ATMOS_CI_SERVER_URL` or `$CI_SERVER_URL` |
| `CloneURL` | `$ATMOS_CI_REPOSITORY_URL` or `$CI_REPOSITORY_URL` or `$GIT_URL` |
| `EventName` | `$ATMOS_CI_EVENT` |
| `RunID` | `$ATMOS_CI_RUN_ID` or `$CI_JOB_ID` or `$BUILD_ID` |
| `RunURL` | `$ATMOS_CI_RUN_URL` or `$CI_JOB_URL` or `$BUILD_URL` |
| `PullRequest` | `$ATMOS_CI_PR` (positive integer); `nil` when unset or invalid. `BaseRef` comes from `$ATMOS_CI_BASE_REF`, the same variable `ResolveBase` reads. `Fork` comes from `$ATMOS_CI_PR_FORK`, parsed as a boolean (`true`, `1`); unset or invalid means false. |

Fields NOT populated: `RunNumber`, `Workflow`, `Job`, `Ref`.

## Capabilities (IMPLEMENTED)

| Capability | Supported | Details |
|-----------|-----------|---------|
| **OutputWriter** | Yes | `WriteOutput` uses the shared collision-safe heredoc formatter (`provider.FormatOutputLine` in `pkg/ci/internal/provider`, delimiter `ATMOS_EOF_<KEY>`) and appends to the `$ATMOS_CI_OUTPUT` file, or renders the same formatted line locally if unset (the heredoc form for a multiline value) |
| **Summary** | Yes | Masks the content, then appends to the `$ATMOS_CI_SUMMARY` file; without a file, renders the markdown locally (see [Rendering](#rendering)) |
| **Check Runs** | Yes | Synthetic in-memory check runs with IDs from a shared `atomic.Int64` counter, logged via `ui`, with a `URL:` line when a details URL exists |
| **PostComment** | Yes (local) | Renders a masked preview and returns a synthetic `Comment` (empty `URL`, ID from a shared `atomic.Int64` counter). Never posts. An in-process ledger of rendered markers makes `behavior=update` with no earlier comment fail with `ErrCICommentNotFound`, as it does on GitHub. |
| **CommitCommenter** | Yes (local) | `PostCommitComment` renders a `commit comment preview (...)` the same way, with its own marker ledger scoped to the commit |
| **Annotate** | Yes (local) | One line per annotation: `path:line: level: message (title)`, omitting empty parts, emitted as error, warning, or info by level |
| **ReportSARIF** | Yes (local) | One info line naming the category, the file when known, and the byte count, with the reason the report was not uploaded (the switch that is off, or that no CI provider is detected) |
| **LogGrouper** | Yes (local) | `StartLogGroup` prints a heading line; `EndLogGroup` is a no-op |
| **EnvExporter** | Yes (local) | See [Environment export](#environment-export) |
| **OutputBinder** | Yes | `BindOutput(io.Writer)` routes every local rendering to the caller's writer |
| **GetStatus** | No | Returns `ErrCIOperationNotSupported` |

### Rendering

Summary and comment-preview bodies are emitted **raw** when the provider is unbound. The unbound instance is the registry instance used by the `--ci` plugin flow, and raw output keeps pipelines that parse stderr working. A provider bound with `BindOutput` renders the same bodies as terminal markdown.

### PostComment validation

The generic `PostComment` is deliberately lenient because a laptop run usually has no pull request number and often no owner or repo. It validates only that `Body` is non-empty and, when a `Marker` is set, that the marker appears in the body (otherwise future upserts would create duplicates). It does not require owner, repo, or PR number. The preview header is `PR comment preview (<behavior>, PR #<n>)`, and omits the PR number when it is 0. A commit comment reads `commit comment preview (<behavior>, commit <sha7>)`. Failures wrap `ErrCICommentPostFailed`.

### Environment export

`WriteEnv` and `AddPath` read `ATMOS_CI_ENV` and `ATMOS_CI_PATH` at call time (not at init):

| Variable set | Behavior |
|--------------|----------|
| `ATMOS_CI_ENV` | `WriteEnv(key, value)` appends a `provider.FormatOutputLine`-formatted line (`KEY=value`, heredoc for multiline) to the file |
| `ATMOS_CI_PATH` | `AddPath(dir)` appends `dir` to the file in the same format |
| Unset | `WriteEnv` prints `export KEY=value`; `AddPath` prints `export PATH=<dir>:"$PATH"` to the UI channel (stderr) or the bound writer, ready to `eval` in a shell. Values are shell-quoted only when they need quoting (`shellescape.Quote`). |

Failures wrap `ErrCIEnvWriteFailed`.

### Output and summary files

`OutputWriter()` reads `ATMOS_CI_OUTPUT` and `ATMOS_CI_SUMMARY` each time it is called, like `ATMOS_CI_ENV` and `ATMOS_CI_PATH`. `NewProvider` caches no environment, so a change made after construction applies to the next writer.

### Output binding

The generic provider implements the optional `OutputBinder` capability (`BindOutput(io.Writer) Provider`). Hosts such as script steps bind it to their own writer so local renderings keep parallel-task line prefixing and secret masking instead of going to the global UI channel. `BindOutput` returns a copy that shares the two atomic counters (check-run IDs and comment IDs), so IDs stay unique across bound copies.

## Implementation

Implemented in `pkg/ci/providers/generic/`:
- `provider.go` — Provider struct, `Context()`, `OutputWriter()`, `Detect()` (always false), and the local renderings (`PostComment`, `Annotate`, `ReportSARIF`, `StartLogGroup`/`EndLogGroup`, `WriteEnv`/`AddPath`, `BindOutput`)
- `base.go` — `ResolveBase()`
- `check.go` — `CreateCheckRun()`, `UpdateCheckRun()` with synthetic IDs
- `provider_test.go`, `check_test.go` — Tests
