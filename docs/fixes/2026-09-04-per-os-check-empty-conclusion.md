# Fix: per-OS acceptance check failed on a shard whose conclusion the API had not written yet

**Date:** 2026-09-04

## Summary

`Acceptance Tests (macos)` (the per-OS aggregate in `test.yml`) failed on PR #3049's run
33907449514 with:

```text
the following 'macos' shard jobs did not succeed:
Acceptance Tests (macos, shard 9/10)
```

The conclusion column is empty. Every macOS shard had succeeded; shard 9 had completed at
19:13:13, seven minutes before the aggregate ran at 19:20:28, yet the jobs API returned it with
`conclusion: null`. The check compared that empty string against `success` and failed the run.

## Fix

The check moved out of shell into Go, next to the classify step that already lives there:
`internal/ci/acceptance.CheckShardResults` (with `TargetFromCheckName`), exposed as
`go tool mage ci:checkShardResults <repo> <run-id> <attempt> <check-name>`. It lists the attempt's
jobs through the same go-gh `RESTClient` and `rerun.FetchJobs` the classify step uses, keeps the
"Acceptance Tests (<os>, shard N/M)" ones, and treats an empty conclusion as *not settled yet*:
it re-lists (up to 20 times, 15 s apart, context-aware) until every shard has a conclusion, then
judges. A shard that truly failed still fails the check with the offending shards listed; a shard
count other than `TEST_SHARD_COUNT` fails at once (every job of an attempt exists from the start,
so a mismatch means the workflow changed shape); a listing that never settles fails loudly with
the last listing printed. Unit-tested with the generated `MockRESTClient` (settles immediately,
settles after retries, never settles, failed shards, count mismatch, cancelled context).

The `test-required` job gains a checkout and a `setup-go` (with `cache: false`, so this tiny job
never writes a thin archive under the Linux setup-go key every other Linux job shares), `contents:
read`, and the Go download hosts in its egress allowlist. The workflow step is one line.

## Validation

- `actionlint .github/workflows/test.yml`, pre-commit hooks clean.
- The PR's own `Tests` run exercises all three per-OS aggregates.
