# Native Component Updater

Use `atmos vendor update --pull-request` for scheduled, reviewable component updates. It is opt-in: ordinary `atmos vendor update` remains a local update command.

Configure source selection under `vendor.update`. A group has `include` and optional `exclude` glob lists; exclusions win. Invoke one group with `--group platform`, select components with repeatable `--component`, or omit both for all sources. Use scope batching on the current checkout by default. Component batching requires an isolated linked-worktree execution mode.

Put PR behavior under `vendor.ci.pull_request`: `provider: github`, optional `base_branch`, `branch_prefix`, title/body templates, labels, draft, reviewers, and assignees. Branches are deterministic and never force-pushed. Atmos discovers updates before branch creation, so no update makes no branch, commit, push, or PR. `--pull-request` implies `--pull`; `--check` never writes.

Supply `ATMOS_CI_GITHUB_TOKEN`, `GITHUB_TOKEN`, or `GH_TOKEN`. Use `contents: write`, `pull-requests: write`, and `issues: write` as needed. A default GitHub token does not trigger downstream push workflows; use a PAT or GitHub App installation token when that is required.

GitHub Actions gets a Component Updater step summary on every vendor-update invocation when `GITHUB_STEP_SUMMARY` is available. It includes any PR link. Set `vendor.ci.summary.enabled: false` only when summaries must be suppressed. See `docs/prd/component-updater.md` for the full contract.
