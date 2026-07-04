# Native CI Integration - Status Checks

> Related: [Overview](../../overview.md) | [GitHub Provider](./provider.md) | [Configuration](../../framework/configuration.md)

## FR-4: Status Checks (IMPLEMENTED)

**Requirement**: Post commit status checks showing operation progress.

**Implementation**: Uses the GitHub **Commit Status API** (`POST /repos/{owner}/{repo}/statuses/{sha}`) via `Repositories.CreateStatus()`. The plugin's `createCheckRun()` handler sets a `pending` status on "before" events and `updateCheckRun()` handler sets a final status (`success`/`failure`) on "after" events. The Status API is idempotent by `context` string — no ID tracking or fallback logic needed. Additionally, per-operation statuses (`/add`, `/change`, `/destroy`) are created when resource counts > 0.

**Behavior**:
- Set commit status to `pending` when operation starts
- Set commit status to `success`/`failure` when operation completes with resource change summary
- Include component and stack in the status context string
- Create per-operation statuses (add/change/destroy) when corresponding resource counts > 0
- Support configuration via `ci.checks.enabled` (disabled by default)
- Per-operation statuses individually toggleable via `ci.checks.statuses.*`

**Status States**:
| State | Description |
|-------|-------------|
| `pending` | Operation started, not yet complete |
| `success` | Operation completed successfully |
| `failure` | Operation failed with errors |
| `error` | Internal error or cancelled |

**Per-Operation Statuses**:

For each component, additional informational statuses are created when resource counts > 0:

| Context Pattern | When Created | State | Description |
|----------------|-------------|-------|-------------|
| `{prefix}/{command}/{stack}/{component}` | Always | `pending` → `success`/`failure` | Resource change summary |
| `{prefix}/{command}/{stack}/{component}/add` | Only if `to_add > 0` | `success` | `"N resources"` |
| `{prefix}/{command}/{stack}/{component}/change` | Only if `to_change > 0` | `success` | `"N resources"` |
| `{prefix}/{command}/{stack}/{component}/destroy` | Only if `to_destroy > 0` | `success` | `"N resources"` |

Per-operation statuses are purely informational (always `success` state). Their **presence** signals the condition (e.g., "there are resources to destroy"). If there are no resources of that type, the status is not created.

**Validation**:
- Commit statuses appear in GitHub PR status section
- Status description shows resource change summary (e.g., "3 to add, 1 to change, 2 to destroy")
- Per-operation statuses appear only when counts > 0
- Disabled by default (requires `statuses: write` permission)

## FR-9: CI Status Command

**Requirement**: Show PR/commit status similar to `gh pr status`.

**Behavior**:
- Display current branch status with check results
- Show PRs created by user
- Show PRs requesting review from user
- Use familiar status icons (checkmark success, x failure, circle pending)
- Reads both Checks API and Commit Status API results for display

**Validation**:
- Works in CI and locally (with `GITHUB_TOKEN`)
- Shows all check runs and commit statuses for current commit
- Matches `gh pr status` UX patterns

## Live Status Checks

Atmos posts commit statuses when operations start and complete:

```
2 pending statuses

* atmos/plan/plat-ue2-dev/vpc  — pending — Plan in progress...
* atmos/plan/plat-ue2-dev/eks  — pending — Plan in progress...
```

When complete:

```
5 statuses

+ atmos/plan/plat-ue2-dev/vpc       — success — 3 to add, 1 to change, 2 to destroy
+ atmos/plan/plat-ue2-dev/vpc/add   — success — 3 resources
+ atmos/plan/plat-ue2-dev/vpc/change  — success — 1 resource
+ atmos/plan/plat-ue2-dev/vpc/destroy — success — 2 resources
+ atmos/plan/plat-ue2-dev/eks       — success — No changes
```

Status checks require the `statuses: write` permission and are enabled via configuration:

```yaml
ci:
  checks:
    enabled: true
    context_prefix: "atmos"  # Status context prefix
    statuses:
      component: true        # atmos/{command}/{stack}/{component}
      add: true              # atmos/{command}/{stack}/{component}/add
      change: true           # atmos/{command}/{stack}/{component}/change
      destroy: true          # atmos/{command}/{stack}/{component}/destroy
```

The `context_prefix` is read from `ci.checks.context_prefix` config (defaults to `"atmos"`). Status context strings follow the pattern: `{prefix}/{command}/{stack}/{component}`. For example: `atmos/plan/plat-ue2-dev/vpc`.

## `atmos ci status` Command

```bash
# Show CI status for current commit (like gh pr status, but using GitHub API directly)
atmos ci status
```

**Example Output (when on a PR branch):**

```
Relevant pull requests in cloudposse/infra-live

Current branch
  #123  Add VPC module [feature-branch]
    - + terraform-plan (success)
    - + terraform-validate (success)
    - o terraform-apply (pending)
    - x security-scan (failure)

Created by you
  #120  Update EKS cluster [eks-upgrade]
    - + All checks passing

Requesting a code review from you
  #118  Refactor networking [net-refactor]
    - + All checks passing
```

**Example Output (when not on a PR branch):**

```
Commit status for abc123d in cloudposse/infra-live

  + terraform-validate (success)
  o terraform-plan (pending)
  x lint (failure)

No open pull request for current branch.
```
