# Stopwatch action

This repository-local JavaScript action updates one sticky pull-request comment
with timing data from every GitHub Actions workflow run found for the PR's current
head commit. It reports two deliberately different totals:

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
`workflow_run`. It checks out only this action from the default branch and never
checks out or executes pull-request code. The workflow's trigger list must stay in
sync with workflows that can run for an open pull request.

## Development

```shell
npm ci
npm run verify
```

`npm run build` bundles the TypeScript entrypoint into `dist/index.js` with
`@vercel/ncc`, then normalizes generated whitespace so the repository's
`git diff --check` remains clean. The runtime uses Node's built-in `fetch` and has
no production dependencies. Commit the generated bundle whenever source changes.
