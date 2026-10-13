# Stopwatch action

This repository-local JavaScript action collects timing data from every GitHub
Actions workflow run found for the PR's current head commit. It reports two deliberately different totals:

- **PR wall-clock time** spans the earliest workflow creation through the latest
  workflow or job completion.
- **Aggregate runner time** adds the execution time of every job, including every
  matrix expansion. Concurrent jobs therefore add more runner time without adding
  the same amount of wall-clock latency.

For each workflow, only its latest run is counted. This prevents reruns and
label-triggered duplicates from inflating the result. If any selected workflow or
job is still running, the action does not write; the next completed workflow run
will invoke the coordinator again.

The companion `ci-timing-summary.yml` workflow runs from the default branch on
`workflow_run`. It checks out this action from the default branch and never
executes pull-request code. It sets `publish-comment: false` and passes the
`summary` and `head-sha` outputs to the reusable Quality Gate workflow only when
`ready` is true. That trusted workflow publishes timings, quality gates, and
benchmark comparisons in one sticky quality comment. The trigger list must stay
in sync with workflows that can run for an open pull request.

`ready` is false when workflows/jobs are unfinished or the PR head changed, and
true for completed failures as well as successes. With `publish-comment: false`,
the action performs no comment reads or writes; `published` remains false.
The default `publish-comment: true` retains standalone comment behavior for other
callers. Existing standalone timing comments are not deleted during migration.

Sticky-comment lookup requires both the hidden marker and the comment author's
login. The default author is `github-actions[bot]`; set the `comment-author`
input when the action runs with a personal access token or GitHub App token.

## Permissions

The timing collector grants `actions: read`, `contents: read`, and
`pull-requests: read`. The reusable quality reporter separately grants the write
permissions needed to publish its comment and labels. Standalone action callers
using `publish-comment: true` also need `issues: write` and `pull-requests: write`.

Grant `pull-requests: write` even though PR conversation comments use the
issue-comments API. Runs with `issues: write` and `pull-requests: read` failed
to create comments with `403 Resource not accessible by integration`.

After permission changes reach the default branch, verify that an eligible
PR's completed CI cycle invokes the quality reporter and updates the composite
comment. A successful coordinator run can also mean it skipped reporting because
CI is incomplete or the PR head changed; check the action's logs and `ready`
output. For standalone callers, check `published` to confirm publication.

## Development

```shell
npm ci
npm run verify
```

`npm run build` bundles `src/main.ts` into `dist/index.js` with
`@vercel/ncc`, then normalizes generated whitespace so the repository's
`git diff --check` remains clean. The runtime uses Node's built-in `fetch` and has
no production dependencies. Commit the generated bundle whenever source changes.
