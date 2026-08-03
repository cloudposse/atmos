# Native Component Updater

Use `atmos vendor update --pull-request` for scheduled, reviewable component updates. It is opt-in: ordinary `atmos vendor update` remains a local update command.

Configure source selection under `vendor.update`. A group has `include` and optional `exclude` glob lists; exclusions win. Invoke one group with `--group platform`, select components with repeatable `--component`, or omit both for all sources. Updates batch as a single scope (one branch/PR for the whole run) on the current checkout by default, or in an isolated linked worktree with `execution.mode: worktree`.

Put PR behavior under `vendor.ci.pull_request`: `provider: github`, optional `base_branch`, `branch_prefix`, title/body templates, labels, draft, reviewers, and assignees. Branches are deterministic and never force-pushed. Atmos discovers updates before branch creation, so no update makes no branch, commit, push, or PR. `--pull-request` implies `--pull`; `--check` never writes.

Supply `ATMOS_CI_GITHUB_TOKEN`, `ATMOS_PRO_GITHUB_TOKEN`, `GITHUB_TOKEN`, or `GH_TOKEN`, in that precedence order. Use `contents: write`, `pull-requests: write`, and `issues: write` as needed. A default `GITHUB_TOKEN` does not trigger downstream push/pull_request workflows; pair the Component Updater with the `github/sts` auth integration for a token that does — `atmos auth exec --identity <github-sts-identity> -- atmos vendor update --pull-request` mints a real GitHub App installation token and exports it as `ATMOS_PRO_GITHUB_TOKEN`, which the Component Updater already prefers over `GITHUB_TOKEN`.

GitHub Actions gets a Component Updater step summary on every vendor-update invocation when `GITHUB_STEP_SUMMARY` is available. It includes any PR link. Set `vendor.ci.summary.enabled: false` only when summaries must be suppressed. See `docs/prd/component-updater.md` for the full contract.
