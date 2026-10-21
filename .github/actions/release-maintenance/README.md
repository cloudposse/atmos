# Release maintenance

Runs downstream branch maintenance and the existing pull-request release
notifications after the published release has passed verification. Stable
releases update release branches; both stable releases and prereleases notify
included pull requests. Moving the major-version tag is handled separately by
the guarded `release-major-tag.yml` workflow, avoiding two competing tag writers.

| Input | Required | Purpose |
| --- | --- | --- |
| `app-id` | Yes | Release bot GitHub App ID. |
| `app-private-key` | Yes | Release bot App private key. |

```yaml
- uses: $/.github/actions/release-maintenance
  with:
    app-id: ${{ vars.BOT_GITHUB_APP_ID }}
    app-private-key: ${{ secrets.BOT_GITHUB_APP_PRIVATE_KEY }}
```

Use on a `release: published` event, after the verification job succeeds. The
caller hardens the runner first, uses the `release` environment, and grants
`contents: read` plus `pull-requests: write`. The separate App token requests
Contents and Workflows write access for branch maintenance; notifications use
the job token. The action has no outputs.

`comment.cjs` contains the notification logic formerly embedded in the shared
workflow. It runs through `actions/github-script`. Branch management remains
delegated to the pinned Cloud Posse action. This action intentionally performs
external branch writes and PR comments only when invoked by the workflow.

Tests cover release selection, deduplication, empty histories, and API failures.
They mock GitHub and never post live comments:

```sh
node --test .github/actions/release-maintenance/comment.test.cjs
```
