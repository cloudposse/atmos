# Migrate from `cloudposse/github-action-atmos-component-updater`

Replace the legacy updater action with a scheduled workflow that checks out the repository, installs Atmos, and runs:

```yaml
jobs:
  vendor-update:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    steps:
      - uses: actions/checkout@v6
      - run: atmos vendor update --pull-request
```

The native invocation is:

```sh
atmos vendor update --pull-request
```

Do not retain a third-party action for updating, committing, pushing, or opening the PR. Configure update selection in `vendor.update` and PR metadata in `vendor.ci.pull_request`.

| Legacy action concern | Native Atmos replacement |
| --- | --- |
| include/exclude component globs | `vendor.update.groups.<name>.include` / `.exclude`, invoked with `--group <name>` |
| individual component selection | repeat `--component <name>` |
| update-and-pull behavior | `--pull-request` implies `--pull` |
| branch, title, body, labels, draft | `vendor.ci.pull_request` |
| GitHub token input | `ATMOS_CI_GITHUB_TOKEN`, `ATMOS_PRO_GITHUB_TOKEN`, `GITHUB_TOKEN`, or `GH_TOKEN` |
| action summary | Native GitHub step summary |

Stage the rollout: first run `atmos vendor update --check --group <name>` on a non-production group; then enable `--pull-request` manually; finally schedule it and retire the legacy action. Grant only `contents: write` and `pull-requests: write`; add `issues: write` only when using labels or assignees. The default `GITHUB_TOKEN` suppresses downstream push workflows; instead of a long-lived PAT or manually-managed GitHub App token, pair the Component Updater with the `github/sts` auth integration, whose job needs `id-token: write` for the OIDC exchange: `atmos auth exec --identity <github-sts-identity> -- atmos vendor update --pull-request` mints a real GitHub App installation token and exports it as `ATMOS_PRO_GITHUB_TOKEN`, which the Component Updater already prefers over `GITHUB_TOKEN`.

See the vendoring [Component Updater reference](../../atmos-vendoring/references/component-updater.md) for the native operating model.
