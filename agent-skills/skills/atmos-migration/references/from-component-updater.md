# Migrate from `cloudposse/github-action-atmos-component-updater`

Load [atmos-vendoring](../../atmos-vendoring/SKILL.md) for update selection and PR configuration,
and [atmos-auth](../../atmos-auth/SKILL.md) when migrating GitHub App authentication to `github/sts`.
Replace the legacy updater action with a workflow that checks out the repository and runs Atmos
from a pinned container. Start with manual dispatch; carry over the existing schedule after validation:

```yaml
name: Component updater
on:
  workflow_dispatch:

jobs:
  vendor-update:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    env:
      GITHUB_TOKEN: ${{ github.token }}
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

Preserve the existing component selection, base branch, schedule, PR metadata, and private-source
access. Native updates batch the selected scope into one branch/PR; do not promise the same PR
grouping as a legacy per-component workflow without explicitly mapping its groups/invocations.

Stage the rollout: first run `atmos vendor update --check --group <name>` on a non-production
group; then enable `--pull-request` manually; finally restore the schedule and retire the legacy
action. Validate no-update behavior, intended changed files, PR metadata, repeat-run PR reuse,
and downstream CI before disabling the old workflow. Avoid overlapping scheduled writers.

## Regular Users: GITHUB_TOKEN

Atmos Pro is not required. Use `GITHUB_TOKEN: ${{ github.token }}` as in the workflow above.
Grant `contents: write` and `pull-requests: write`; add `issues: write` for labels or assignees,
and ensure repository policy allows Actions to create PRs.

Explain the downstream CI limitation before cutover: PRs created with the default `GITHUB_TOKEN`
do not run their CI automatically. GitHub currently creates approval-required runs for PR
`opened`, `synchronize`, and `reopened` events; a user with write access must approve them.
Push events from this token do not trigger workflows, and other PR activity types are suppressed.
This is GitHub's recursion protection, not an Atmos failure. See
[GitHub's token-trigger rules](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs).

## Atmos Pro Users: Short-Lived STS Tokens

For Atmos Pro users who want downstream PR CI to run automatically, recommend `github/sts` to
mint short-lived GitHub App installation tokens dynamically instead of maintaining long-lived
PATs or manually managed App tokens. These tokens avoid the default workflow token's recursion
restriction; normal workflow filters and repository policies still apply.

Configure the `atmos/pro` provider/identity and `github/sts` integration following
[atmos-auth](../../atmos-auth/SKILL.md), leave `auto_provision` enabled, and add `id-token: write`
for the OIDC exchange. Activate the profile containing that configuration if needed. Keep the native invocation:

```shell
atmos vendor update --pull-request
```

In CI, the updater invokes the credential broker before checking source versions. The broker
authenticates the identity bound by the integration's `via` configuration and exports the
installation token as `ATMOS_PRO_GITHUB_TOKEN`; no routine `atmos auth login` or nested
`atmos auth exec -- atmos ...` is needed. `--identity` is a global flag, but this broker currently
selects its identity from the integration binding, not that flag. Treat any need to wrap Atmos
to select a different identity as an auth-integration gap, not the standard migration pattern.

Ensure the App has access to the target repository and the necessary contents/PR/optional issue grants; workflow
permissions do not grant rights to that separate token. Remove any inherited
`ATMOS_CI_GITHUB_TOKEN` override pointing at the default token when using STS for the updater:
it takes precedence over `ATMOS_PRO_GITHUB_TOKEN` and would defeat this token choice.

See the vendoring [Component Updater reference](../../atmos-vendoring/references/component-updater.md) for the native operating model.
