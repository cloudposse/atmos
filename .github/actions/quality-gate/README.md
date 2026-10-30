# Quality Gate

Runs the Mage quality reporter from trusted default-branch code.

- `operation: report` (`quality:report`) refreshes only the PRs the event names:
  a release label or milestone change, or a CI completion for their head. It skips
  repository-wide events and never falls back to every open PR.
- `operation: reconcile` (`quality:reconcile`) trues up PRs after a schedule tick,
  a merge, a milestone or release change, or a manual dispatch.

The two operations run in separate workflows (`quality-report.yml` and
`quality-reconcile.yml`) with separate concurrency groups, so a long true-up never
cancels a queued per-PR report. The reporter ignores both workflows' own runs as CI
evidence.

Tests: `go test -tags=mage ./magefiles/qualitygate/...`.
