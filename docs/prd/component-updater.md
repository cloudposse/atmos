# PRD: Native Component Updater PR Workflow

## Status

Implemented design for native, opt-in pull-request publishing from `atmos vendor update`. It replaces the capability of `cloudposse/github-action-atmos-component-updater` without third-party update, commit, push, or PR actions.

## Configuration and invocation

```yaml
vendor:
  update:
    execution: { mode: current } # current | worktree
    batching: { mode: scope } # scope | component; component requires worktree
    groups:
      platform:
        include: ["terraform/vpc", "terraform/eks/*"]
        exclude: ["terraform/eks/legacy"]
  ci:
    pull_request:
      provider: github
      base_branch: main # otherwise the remote default branch
      branch_prefix: atmos/component-updater
      title: "chore(components): update {{ .scope.name }}"
      body: "{{ .updates | markdownTable }}"
      labels: [component-update]
      draft: false
      reviewers: []
      assignees: []
    summary: { enabled: true }
```

Use `atmos vendor update --pull-request`, optionally with repeatable `--component <name>` or `--group <name>`. No selector means all discoverable sources. `--pull-request` implies `--pull`; `--check` is mutation-free. `--format table|json` controls output. Group patterns match canonical component names and exclusions win.

## Architecture and safety

`cmd/vendor` binds flags and renders results. `pkg/vendoring` owns discovery, version edits, and pulling. `pkg/vendoring/updater` owns structured results and pure Markdown summaries. `pkg/git` owns branch safety and a provider-neutral `PullRequestPublisher` registry. The CLI provider performs local Git; `pkg/git/providers/github` uses `pkg/ci/providers/github.Client` and `go-github`. GitLab and Bitbucket can add publishers without changing the command.

Atmos discovers before branch creation: no updates means no branch, commit, push, or PR. The current checkout must be clean, the configured or remote-default base is fetched, and that base is never written. Scope names are deterministic: `<prefix>/all`, `<prefix>/group-<name>`, and `<prefix>/components-<stable-selection-hash>`; component batching reserves `<prefix>/component/<name>` in isolated linked worktrees. Existing remote feature branches are reused and pushed fast-forward only. PR reconciliation updates title/body and additively requests labels, reviewers, and assignees while preserving draft state after creation. Provisioner and lifecycle-hook PR support remain unsupported.

## Auth, permissions, and summaries

Token precedence is `ATMOS_CI_GITHUB_TOKEN`, `GITHUB_TOKEN`, then `GH_TOKEN`; PATs and GitHub App installation tokens work through these variables. Grant `contents: write`, `pull-requests: write`, and `issues: write` where labels/assignees are used. A default [`GITHUB_TOKEN`](https://docs.github.com/en/actions/concepts/security/github_token) does not trigger downstream `push` workflows; use a PAT or GitHub App token when downstream automation must run. GitHub may require approval for PR-triggered workflows; see [workflow permissions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax).

In GitHub Actions Atmos appends a Component Updater summary for every `vendor update`, independent of PR requests, updates, or token availability. It shows scope, counts, dry-run state, branch/commit, failures, and a PR link. `vendor.ci.summary.enabled: false` disables it; summary write failures never mask operation failures.

## Workflow and migration

```yaml
on:
  schedule: [{ cron: "17 3 * * 1" }]
permissions:
  contents: write
  pull-requests: write
  issues: write
jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - name: Install Atmos
        run: |
          curl -fsSL https://atmos.tools/install.sh | bash
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"
      - run: atmos vendor update --pull-request
        env: { GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }} }
```

The workflow uses checkout plus native Atmos installation and Atmos itself; no third-party action performs updating, committing, pushing, or PR publishing. Migrate legacy options and roll out groups progressively using [from-component-updater.md](../../agent-skills/skills/atmos-migration/references/from-component-updater.md).

## Test plan

Unit tests cover schema/flags, selectors, names, templates, no-op, JSON, and summary rendering. Fake GitHub HTTP clients cover PR list/create/edit/enrichment and auth errors. Local bare remotes cover dirty-tree refusal, protected-base safety, commit/push, no-op, and worktree cleanup. Summary tests use temporary `GITHUB_STEP_SUMMARY` files for no-update, dry-run, PR, failure, disabled, and local cases.
