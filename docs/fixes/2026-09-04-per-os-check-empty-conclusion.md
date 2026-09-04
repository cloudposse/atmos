# Fix: per-OS acceptance check failed on a shard whose conclusion the API had not written yet

**Date:** 2026-09-04

## Summary

`Acceptance Tests (macos)` (the per-OS aggregate in `test.yml`) failed on PR #3049's run
33907449514 with:

```
the following 'macos' shard jobs did not succeed:
Acceptance Tests (macos, shard 9/10)
```

The conclusion column is empty. Every macOS shard had succeeded; shard 9 had completed at
19:13:13, seven minutes before the aggregate ran at 19:20:28, yet the jobs API returned it with
`conclusion: null`. The check compared that empty string against `success` and failed the run.

## Fix

"Check per-OS test matrix result" now treats an empty conclusion as *not settled yet*: it re-lists
the shards (up to 20 times, 15 s apart) until every shard has a conclusion and the count matches
`TEST_SHARD_COUNT`, and only then judges. A shard that truly failed still fails the check; a
listing that never settles fails loudly with the last listing printed.

## Validation

- `actionlint .github/workflows/test.yml`, pre-commit hooks clean.
- The PR's own `Tests` run exercises all three per-OS aggregates.
