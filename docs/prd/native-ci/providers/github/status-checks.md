# Native CI Integration - Status Checks

> Related: [Overview](../../overview.md) | [GitHub Provider](./provider.md) | [Configuration](../../framework/configuration.md)

## FR-4: Status Checks (IMPLEMENTED)

**Requirement**: Post commit status checks showing operation progress.

**Implementation**: The executor's `executeCheckAction()` creates check runs on "before" events (`executeCheckCreate`) and updates them on "after" events (`executeCheckUpdate`). Check run IDs are correlated via `sync.Map` keyed by `stack/component/command`. If no stored ID is found on "after", a new completed check run is created. Uses `provider.CreateCheckRun()` and `provider.UpdateCheckRun()` — GitHub implementation in `pkg/ci/providers/github/checks.go`.

**Behavior**:
- Create check run when operation starts ("Plan in progress")
- Update check run when operation completes with result summary
- Include component and stack in check name
- Support configuration via `ci.checks.enabled` (disabled by default)

**Check States**:
| State | Description |
|-------|-------------|
| `in_progress` | Operation started, not yet complete |
| `success` | Operation completed successfully |
| `failure` | Operation failed with errors |

**Validation**:
- Check runs appear in GitHub PR checks section
- Check description shows resource change summary
- Disabled by default (requires `checks: write` permission)

## FR-9: CI Status Command

**Requirement**: Show PR/commit status similar to `gh pr status`.

**Behavior**:
- Display current branch status with check results
- Show PRs created by user
- Show PRs requesting review from user
- Use familiar status icons (checkmark success, x failure, circle pending)

**Validation**:
- Works in CI and locally (with `GITHUB_TOKEN`)
- Shows all check runs for current commit
- Matches `gh pr status` UX patterns

## Live Status Checks

Atmos can post GitHub status checks when operations start and complete—just like CodeRabbit:

```
2 pending checks

* Atmos  Plan in progress — vpc in plat-ue2-dev
* Atmos  Plan in progress — eks in plat-ue2-dev
```

When complete:

```
2 checks passed

+ Atmos  Plan complete — 3 to add, 1 to change, 2 to destroy
+ Atmos  Plan complete — No changes
```

Status checks require the `checks: write` permission and are enabled via configuration:

```yaml
ci:
  checks:
    enabled: true
    context_prefix: "atmos"  # Check name prefix: "atmos/plan — vpc in plat-ue2-dev"
```

The `context_prefix` is wired from configuration now (not deferred). Check names follow the pattern: `{context_prefix}/{command} — {component} in {stack}`. For example: `atmos/plan — vpc in plat-ue2-dev`.

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
