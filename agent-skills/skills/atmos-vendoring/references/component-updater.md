# Native Component Updater

Use `atmos vendor update --pull-request` for scheduled, reviewable component updates. It is opt-in: ordinary `atmos vendor update` remains a local update command.

Configure source selection under `vendor.update`. A group has `include` and optional `exclude` glob lists; exclusions win. Invoke one group with `--group platform`, select components with repeatable `--component`, or omit both for all sources. Updates batch as a single scope (one branch/PR for the whole run) on the current checkout by default, or in an isolated linked worktree with `execution.mode: worktree`.

Put PR behavior under `vendor.ci.pull_request`: `provider: github`, optional `base_branch`, `branch_prefix`, title/body templates, labels, draft, reviewers, and assignees. Branches are deterministic and never force-pushed. Atmos discovers updates before branch creation, so no update makes no branch, commit, push, or PR. `--pull-request` implies `--pull`; `--check` never writes.

Supply `ATMOS_CI_GITHUB_TOKEN`, `ATMOS_PRO_GITHUB_TOKEN`, `GITHUB_TOKEN`, or `GH_TOKEN`, in that precedence order. Grant the job `contents: write` and `pull-requests: write`; add `issues: write` only when using labels or assignees.

**Regular users:** use `GITHUB_TOKEN: ${{ github.token }}`; Atmos Pro is not required. Explain that
downstream PR CI does not run automatically with this token: GitHub currently requires approval
for runs from PR `opened`, `synchronize`, and `reopened` events, and suppresses push-triggered
workflows and other PR activity types. See
[GitHub's token-trigger rules](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs).

**Atmos Pro users:** recommend dynamically minted, short-lived `github/sts` App tokens when
downstream CI must run automatically, avoiding the default token's recursion restriction and
long-lived PATs or manually managed App tokens. Normal workflow filters and repository policies
still apply. Load [atmos-auth](../../atmos-auth/SKILL.md) and configure `atmos/pro` plus `github/sts`, with
`auto_provision` enabled and job permission `id-token: write`. Run
`atmos vendor update --pull-request` directly: in CI, the updater invokes the credential broker
before checking versions, and the integration exports `ATMOS_PRO_GITHUB_TOKEN` for Git and PR
operations. Do not prescribe a nested `atmos auth exec -- atmos ...` or a routine login step.
The broker selects the identity through the integration's `via` binding; although `--identity`
is global, it does not currently select that broker identity. Verify the App's repository access
and permissions, and avoid an `ATMOS_CI_GITHUB_TOKEN` override that would shadow its token.

GitHub Actions gets a Component Updater step summary on every vendor-update invocation when `GITHUB_STEP_SUMMARY` is available. It includes any PR link. Set `vendor.ci.summary.enabled: false` only when summaries must be suppressed. See `docs/prd/component-updater.md` for the full contract.
