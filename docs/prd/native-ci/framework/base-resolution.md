# Native CI Integration - Base Resolution

> Related: [Overview](../overview.md) | [CI Detection](./ci-detection.md) | [Interfaces](./interfaces.md)

## Problem Statement

The `atmos describe affected` command requires `--ref` and `--sha` flags to specify the comparison base. These flags:

- Are confusingly named — both specify the base commit, but "ref" vs "sha" doesn't communicate *which side* of the comparison they represent
- Require verbose CI workflow configuration with shell expression gymnastics
- Don't leverage CI environment variables that already contain this information

Zero-config CI operation requires automatic base resolution that is provider-agnostic.

## FR-10: Provider Base Resolution

**Requirement**: The Provider interface is extended with a `ResolveBase()` method. Each CI provider implements event-aware base resolution.

**Interface Extension**:
```go
// BaseResolution contains the resolved base commit for affected detection.
type BaseResolution struct {
    Ref       string // Git reference (mutually exclusive with SHA).
    SHA       string // Git commit hash (mutually exclusive with Ref).
    Source    string // Human-readable source (for logging).
    EventType string // CI event type (e.g., "pull_request", "push").
}

// Added to Provider interface:
ResolveBase() (*BaseResolution, error)
```

**Behavior**:
- Returns `nil` when the provider cannot determine the base (caller falls through to default)
- Returns an error only for unexpected failures (e.g., malformed event payload)
- `Ref` and `SHA` are mutually exclusive — exactly one is set

**Validation**:
- Each provider returns a valid `BaseResolution` for its supported event types
- Unknown event types return `nil` (not an error)
- Malformed event payloads return descriptive errors

## FR-11: Unified `--base` Flag

**Requirement**: A single `--base` flag replaces `--ref` and `--sha`. It accepts both git references and commit SHAs.

**Behavior**:
- Auto-detects whether the value is a ref or SHA (7-40 character hex string = SHA, otherwise = ref)
- `--ref` and `--sha` become hidden, deprecated aliases
- Deprecation warning logged when old flags are used

**Precedence** (highest to lowest):
1. `--base` flag (explicit)
2. `--ref` / `--sha` flags (deprecated, backward compatible)
3. Auto-detection via `ResolveBase()` (when `ci.enabled` is true)
4. `refs/remotes/origin/HEAD` (existing default)

**Validation**:
- `--base abc123def` routes to SHA path
- `--base main` routes to ref path as `refs/remotes/origin/main`
- `--base refs/heads/main` routes to ref path as-is
- `--ref` and `--sha` still work with deprecation warning
- `--base` conflicts with `--repo-path` (same as `--ref`/`--sha`)

## FR-12: GitHub Actions Base Resolution

**Requirement**: The GitHub provider resolves the base commit from GitHub Actions environment variables and event payload.

**Event Resolution Matrix**:

| Event | Action | Base | Type | Source |
|-------|--------|------|------|--------|
| `pull_request` | opened / synchronize | `refs/remotes/origin/$GITHUB_BASE_REF` | ref | `GITHUB_BASE_REF` |
| `pull_request` | closed (merged) | `event.pull_request.base.sha` | SHA | `$GITHUB_EVENT_PATH` |
| `pull_request_target` | any | `refs/remotes/origin/$GITHUB_BASE_REF` | ref | `GITHUB_BASE_REF` |
| `push` | normal | `event.before` | SHA | `$GITHUB_EVENT_PATH` |
| `push` | force-push (`event.forced`) | `HEAD~1` | SHA | git resolution |
| `merge_group` | any | `refs/remotes/origin/$GITHUB_BASE_REF` | ref | `GITHUB_BASE_REF` |
| other | any | `refs/remotes/origin/HEAD` | ref | default |

**Environment Variables Used**:
- `GITHUB_EVENT_NAME` — event type routing
- `GITHUB_BASE_REF` — PR target branch
- `GITHUB_EVENT_PATH` — path to JSON event payload file

**Event Payload Fields Read**:
- `action` — PR action (opened, synchronize, closed)
- `pull_request.base.sha` — base commit SHA for closed PRs
- `before` — previous HEAD SHA for push events
- `forced` — whether push was a force-push

**Validation**:
- PR open/sync: resolves to `refs/remotes/origin/main` when `GITHUB_BASE_REF=main`
- PR closed/merged: resolves to the base SHA from event payload
- Push: resolves to `before` SHA from event payload
- Force-push: falls back to `HEAD~1` when `forced=true`
- Missing `$GITHUB_EVENT_PATH`: returns error (file should always exist in GitHub Actions)

## FR-13: Generic Provider Base Resolution

**Requirement**: The generic provider supports explicit base override via `ATMOS_CI_BASE_REF` environment variable for local testing.

**Behavior**:
- Reads `ATMOS_CI_BASE_REF` environment variable
- Auto-detects ref vs SHA format
- Returns `nil` when env var is not set (falls through to default)
- GitLab and Jenkins are separate dedicated providers (out of scope)

**Validation**:
- `ATMOS_CI_BASE_REF=main` → ref `refs/remotes/origin/main`
- `ATMOS_CI_BASE_REF=abc123def` → SHA `abc123def`
- No env var → returns `nil`

## FR-14: CI Auto-Detection Gating

**Requirement**: Base auto-detection only activates when `ci.enabled` is `true` in `atmos.yaml`.

**Behavior**:
- `ci.enabled` is the hard kill switch (consistent with existing CI detection in `ci-detection.md`)
- When `ci.enabled` is `false` (or unset), auto-detection is skipped entirely
- Explicit flags (`--base`, `--ref`, `--sha`) always work regardless of `ci.enabled`

| `ci.enabled` | Flags provided | Provider detected | Result |
|:------------:|:--------------:|:-----------------:|--------|
| false/unset | none | yes | **No auto-detect** (use default `refs/remotes/origin/HEAD`) |
| false/unset | `--base main` | any | **Use explicit flag** |
| true | none | yes | **Auto-detect via provider** |
| true | none | no | **No auto-detect** (use default) |
| true | `--base main` | any | **Use explicit flag** (flags win) |

**Logging**:
When auto-detection resolves a base, log at Info level:
```
Auto-detected CI base: refs/remotes/origin/main (provider=github-actions, event=pull_request, source=GITHUB_BASE_REF)
```

**Validation**:
- `ci.enabled: false` + GitHub Actions → no auto-detection
- `ci.enabled: true` + GitHub Actions PR → auto-detects base
- `ci.enabled: true` + `--base main` → uses explicit flag
- `ci.enabled: true` + no provider → falls through to default
