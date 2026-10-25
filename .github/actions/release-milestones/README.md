# Release milestones

Runs the same tested Mage operations used locally. Checkout trusted default-branch
code first; never checkout PR code in a job with status- or issue-writing access.
This action receives no release credentials and cannot publish releases.

| Operation | Mage target | Token permissions |
| --- | --- | --- |
| `check` | `milestones:check` | Contents, issues and PRs read; statuses write |
| `ensure` | `milestones:ensure` | Contents read; issues write |
| `create` | `milestones:create` | Issues write |

`version` is required for `create`. The repository comes from `GITHUB_REPOSITORY`.
`ensure` maintains the next three minor versions after the highest published
stable release, reusing existing milestones and reopening unpublished slots.
It never creates patch or major milestones or deletes unrelated milestones.

`check` adds a GitHub check named **Release milestone** to every open PR targeting
the default branch. It also checks PRs waiting in GitHub's merge queue.
The check passes only when the rules below pass. To make GitHub block merging
when it fails or is missing, add **Release milestone** as a required status check
in the default branch's repository ruleset. Displaying a check alone does not block merging.
Exactly one of `major`, `minor`, `patch`, or `no-release` is required. `no-release`
is exempt; other PRs must target the lowest open versioned milestone and cannot
require a larger bump than that milestone supplies. Non-versioned planning
milestones do not participate. Equivalent duplicate versions fail closed.

Milestone/label changes refresh the statuses, including queued PRs. Unchanged
results reuse the existing status to avoid GitHub's per-commit status limit.
Evidence failures revoke known heads; interrupted runs or failed API writes
require a later reconciliation. Scheduled reconciliation recovers missed events; GitHub event delivery and merge decisions
are not atomic. Administrators retaining bypass privileges remain trusted.

Regression tests: `go test -tags=mage ./magefiles/release/...`. Workflow wiring is
covered by tests reading the production YAML; API tests use offline fixtures.
