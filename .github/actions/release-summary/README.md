# Release stage summaries

Every stage of `release-pipeline.yml` calls this action with `if: always()` after
its work. It shows the requested version, actual reserved tag, source commit,
milestone, checkpoint outcomes, and links to the run and uploaded artifacts.
Publication success includes a release link. Failures identify failed checkpoints
and credential failures describe the environment configuration to inspect.

Pass only checkpoint names and `steps.<id>.outcome` in `checks`. Never pass
`toJSON(steps)` or token outputs. All displayed input is escaped. The action
requires no App credentials and does not fetch or modify release state.

Run `node --test .github/actions/release-summary/summary.test.cjs` to test rendering.
