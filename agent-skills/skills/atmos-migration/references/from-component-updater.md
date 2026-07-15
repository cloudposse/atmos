# Migrate from `cloudposse/github-action-atmos-component-updater`

Replace the legacy updater action with a scheduled workflow that checks out the repository and runs:

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
| GitHub token input | `ATMOS_CI_GITHUB_TOKEN`, `GITHUB_TOKEN`, or `GH_TOKEN` |
| action summary | Native GitHub step summary |

Stage the rollout: first run `atmos vendor update --check --group <name>` on a non-production group; then enable `--pull-request` manually; finally schedule it and retire the legacy action. Grant only `contents: write`, `pull-requests: write`, and `issues: write` where labels/assignees are used. Use a PAT or GitHub App token if downstream push workflows must run; the default `GITHUB_TOKEN` suppresses them.

See the vendoring [Component Updater reference](../../atmos-vendoring/references/component-updater.md) for the native operating model.
