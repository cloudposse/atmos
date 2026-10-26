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

**Requirement**: The GitHub provider resolves the base commit from GitHub Actions environment variables and event payload. For open and closed-unmerged pull request events, the primary strategy is `git merge-base` — the only approach that is correct regardless of checkout strategy and merge method. Merged pull requests use a different strategy entirely (see "Merged pull requests: checkout classification" below), because merge-base against the target branch degenerates once the target already contains the PR.

### Why merge-base is the gold standard

The purpose of base resolution is to answer: "what is the fork point — the commit where this PR's changes diverge from the target branch?" This determines which stacks are affected by the PR. For open and closed-unmerged PRs, `git merge-base HEAD origin/<target>` answers this question correctly in all scenarios. Merged PRs are the exception — see "Merged pull requests: checkout classification" below.

### Rejected approaches

**`HEAD~1` (parent of checked-out commit)** — rejected as a *blind* strategy; retained only after positively classifying a merge-commit checkout (see "Merged pull requests: checkout classification"):
- Only correct when the workflow checks out the **merge commit** (not the PR head) AND the merge strategy is merge or squash.
- **Breaks for rebase merges with multiple commits**: `merge_commit_sha` points to the tip of the rebased commits, so `HEAD~1` is the previous rebased commit — not the target branch state.
- **Breaks entirely when the workflow checks out `head.sha`**: `HEAD~1` is the parent commit on the PR branch, which for multi-commit PRs is just the second-to-last PR commit — completely wrong.
- **Breaks the Atmos Pro upload correlation**: Atmos Pro indexes by `event.pull_request.head.sha` from the webhook. If the workflow checks out the merge commit (required for HEAD~1 to work), the upload SHA doesn't match what Atmos Pro expects. This creates conflicting requirements — you can't satisfy both HEAD~1 correctness and Atmos Pro SHA correlation with the same checkout.

**`event.pull_request.base.sha` (from webhook payload)**:
- Points to the correct branch but can be **stale**: if other PRs merge into the target branch between when the PR was created/updated and when it merges, `base.sha` points to an older commit on the target branch.
- In practice, the staleness means the diff may include changes from other PRs that merged in the interim, producing false positives in affected detection.
- For open PRs this is less of an issue (the PR is rebased/synced regularly), but for closed/merged PRs the staleness window can be significant.

### Merge-base approach

`git merge-base HEAD origin/<target_branch>` computes the common ancestor. This is correct regardless of:
- **What's checked out** — works with `head.sha`, merge commit, or any other ref.
- **Merge strategy** — merge, squash, or rebase all produce correct results.
- **Number of commits** — single or multi-commit PRs are handled identically.
- **Target branch movement** — if other PRs merged into the target, merge-base still finds the true fork point.

**Shallow CI checkouts**: merge-base requires the target branch ref (`origin/<target>`) to be available locally. `actions/checkout@v6` with the default `fetch-depth: 1` does not fetch other branches. The implementation handles this transparently:

- `pkg/git/merge_base.go` exposes `MergeBaseWithAutoFetch`, which detects a missing `origin/<target>` ref (or an out-of-reach common ancestor) and runs a targeted `git fetch origin <target>` (and, if needed, one `--deepen=200` fetch) before retrying.
- The CI provider calls the auto-fetch variant, so shallow checkouts are recovered without any change to the user's workflow.
- Only when even the auto-fetch path fails does the fallback chain proceed to `event.pull_request.base.sha` (frozen at last PR sync, never the current tip of `<target>`).

**Edge case**: if the workflow checks out the merge commit, HEAD is *on* the target branch, so `merge-base(HEAD, origin/main) == HEAD`. This is detected (merge-base == HEAD hash) and falls through to the next strategy.

**Merged PRs are excluded from this strategy entirely**: after the merge, `origin/<target>` *contains* the PR, so merge-base against its moving tip degenerates — to HEAD itself for merge-commit merges (the collapse above), and more generally to an anchor that includes the PR's own landing. Merged PRs use the checkout-classified strategy below instead.

### Merged pull requests: checkout classification

For `closed` events with `pull_request.merged == true`, no single strategy is correct for every checkout — `HEAD~1` in particular diffs only the PR's *final* commit when the PR head is checked out, so a multi-commit PR whose last commit reverts an earlier org-wide change re-reports the reverted files as affected after merge (observed in production as a wall of post-merge dispatches). The resolver therefore classifies what the workflow actually checked out, by comparing the local HEAD against the payload, and picks the strategy that is provably correct for that checkout:

| Local HEAD matches | Classification | Base | Why it is correct |
|---|---|---|---|
| `pull_request.head.sha` | `head.sha` | `merge-base(HEAD, merge_commit_sha^1)` | First parent of the merge commit is the pre-merge target tip; the merge-base against it is the true fork point for merge, squash, and rebase strategies alike, and cannot collapse to HEAD. Mirrors `merge_group.base_sha` semantics. Fetches `merge_commit_sha` by SHA if a narrow checkout did not bring it in. |
| `pull_request.head.sha`, fast-forward merge (`merge_commit_sha == head.sha`) | `head.sha` | `merge-base(HEAD, base.sha)` | For externally-merged/fast-forwarded PRs the merge commit IS the PR head, so `merge_commit_sha^1` is the PR's own previous commit — anchoring there silently drops every commit but the last (under-detection). The payload's `base.sha` is a pre-merge target-branch commit; staleness only moves the anchor along the target branch, never onto the PR branch, so the merge-base is still the fork point. |
| `pull_request.merge_commit_sha` | `merge-commit` | `HEAD^1` | First parent of the real merge/squash commit is the pre-merge target tip — the one case the old `HEAD~1` fallback was actually right for. |
| 2-parent commit whose 2nd parent is `head.sha` | `synthetic-merge` | first parent of HEAD | GitHub's `refs/pull/<n>/merge` test-merge commit: first parent is the target tip the merge was built on — the exact net-diff base. |
| anything else | `unknown` | `event.pull_request.base.sha` (Warn) | No strategy is provably correct for an unrecognized checkout; guessing silently is how wrong-base incidents happen. |

Closed-*unmerged* PRs are treated like open PRs (the branch was never folded into the target, so merge-base against `origin/<target>` remains correct).

The classification is included in the "Auto-detected CI base" log line (`checkout=...`) so wrong-base reports are diagnosable from a single line.

### Implementation: generic utility + provider-specific extraction

The merge-base computation itself is provider-agnostic and lives in `pkg/git/` as a shared utility. The GitHub provider is responsible only for extracting the target branch name from GitHub-specific sources (`event.pull_request.base.ref`, `GITHUB_BASE_REF`).

### Fallback chain for open (and closed-unmerged) pull request events

Each strategy is tried in order; the first success is used. (Merged PRs use the checkout-classified strategy above instead.)

1. **`git merge-base(HEAD, origin/<target>)`** via `MergeBaseWithAutoFetch` — gold standard. Target branch extracted from `event.pull_request.base.ref` (payload) or `GITHUB_BASE_REF` (env var). Self-heals from shallow checkouts by fetching the target branch (and deepening once) before retrying. Skipped if merge-base equals HEAD (merge commit checkout).
2. **`event.pull_request.base.sha`** — payload SHA fallback. Frozen at the last PR sync event, so it is never the current tip of `<target>`. Slightly stale on out-of-date PRs but cannot produce the "every component is affected" false positives that returning a *ref* to the current target tip does.
3. **`GITHUB_BASE_REF` ref** — last resort, only reached when the payload has no `base.sha` (hand-crafted or legacy events). Logs `Warn` because this path compares against the current tip and may include unrelated commits from `<target>`.

### Atmos Pro upload correlation

For `--upload` mode, the CLI also extracts `event.pull_request.head.sha` from the event payload. This SHA is used as the `HeadSHA` in the upload request, ensuring it matches what Atmos Pro indexed from the webhook — regardless of which commit the workflow has checked out locally.

Push events are rejected when `--upload` is set, since Atmos Pro only processes `pull_request` webhooks and cannot correlate push event uploads.

### Event Resolution Matrix

| Event | Action | Primary Strategy | Fallback | Type | Source |
|-------|--------|-----------------|----------|------|--------|
| `pull_request` | opened / synchronize | `MergeBaseWithAutoFetch(HEAD, origin/<target>)` | `event.pull_request.base.sha` → `GITHUB_BASE_REF` ref (warn) | SHA or ref | `event.pull_request.base.ref` → `git merge-base` |
| `pull_request` | closed (merged) | Checkout-classified: `merge-base(HEAD, merge_commit_sha^1)` (head.sha) / `HEAD^1` (merge-commit) / first parent (synthetic-merge) | `event.pull_request.base.sha` (also for unknown checkout, warn) → `GITHUB_BASE_REF` ref (warn) | SHA or ref | `event.pull_request.merge_commit_sha` → git |
| `pull_request` | closed (unmerged) | `MergeBaseWithAutoFetch(HEAD, origin/<target>)` | `event.pull_request.base.sha` → `GITHUB_BASE_REF` ref (warn) | SHA or ref | `event.pull_request.base.ref` → `git merge-base` |
| `pull_request_target` | any | `MergeBaseWithAutoFetch(HEAD, origin/<target>)` | `event.pull_request.base.sha` → `GITHUB_BASE_REF` ref (warn) | SHA or ref | `event.pull_request.base.ref` → `git merge-base` |
| `push` | normal | `event.before` | — | SHA | `$GITHUB_EVENT_PATH` |
| `push` | force-push (`event.forced`) | `HEAD~1` | `origin/HEAD` ref | SHA or ref | git resolution |
| `merge_group` | any | `refs/remotes/origin/$GITHUB_BASE_REF` | — | ref | `GITHUB_BASE_REF` |
| other | any | `refs/remotes/origin/HEAD` | — | ref | default |

### Environment Variables Used

- `GITHUB_EVENT_NAME` — event type routing.
- `GITHUB_BASE_REF` — PR target branch (fallback when payload extraction fails).
- `GITHUB_EVENT_PATH` — path to JSON event payload file.

### Event Payload Fields Read

- `action` — PR action (opened, synchronize, closed).
- `pull_request.base.ref` — target branch name for merge-base computation.
- `pull_request.base.sha` — payload-base fallback SHA.
- `pull_request.head.sha` — PR head commit SHA for Atmos Pro upload correlation and checkout classification.
- `pull_request.merged` — routes closed events to the merged-PR strategy (unmerged closes resolve like open PRs).
- `pull_request.merge_commit_sha` — anchor for merged-PR base resolution and checkout classification.
- `before` — previous HEAD SHA for push events.
- `forced` — whether push was a force-push.

### Validation

- PR open/sync (and closed-unmerged): `MergeBaseWithAutoFetch(HEAD, origin/<target>)` resolves to the fork-point SHA (self-healing fetch on shallow clones); falls back to `event.pull_request.base.sha`, then `refs/remotes/origin/<target>` ref (warn).
- PR closed/merged: checkout is classified against the payload; head.sha checkout resolves `merge-base(HEAD, merge_commit_sha^1)` (fork point), merge-commit checkout resolves `HEAD^1`, synthetic-merge checkout resolves the first parent, unknown checkout falls back to `event.pull_request.base.sha` with a warn. A multi-commit PR whose final commit reverts an earlier commit must resolve the fork point, never the PR's own previous commit.
- Push: resolves to `before` SHA from event payload.
- Force-push: falls back to `HEAD~1` when `forced=true`.
- Missing `$GITHUB_EVENT_PATH`: returns error (file should always exist in GitHub Actions).
- `--upload` + push event: returns error with actionable hints.

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
