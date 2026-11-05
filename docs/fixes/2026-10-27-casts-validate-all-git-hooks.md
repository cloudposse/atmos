# Fix: register the repository git-hooks cast validator in `casts validate all`

**Date:** 2026-10-27

## Summary

The `Validation (casts)` CI job failed on `main` and on every branch rebased onto it because the
`casts validate repository git-hooks` validator was never reachable from the `casts validate all`
aggregate. Adding the one missing step restores the job.

## Context

Two independent changes landed on `main` without meeting: the Git hooks work added
`casts validate repository git-hooks` under `demo/casts/atmos.d/repository/git-hooks.yaml`, and the
Starlark cast recorder work added a coverage guard to `casts validate all` that fails when any leaf
validator is not reachable from the aggregate. Once both were on `main`, the guard reported:

```text
cast validators never run from 'casts validate all'; add them to an aggregate:
  - casts validate repository git-hooks
```

The committed cast at `website/static/casts/demo/repository/git-hooks.cast` passes its validator; it
was only unregistered.

## Changes

- `demo/casts/atmos.d/all.yaml`: add a `repository-git-hooks` shell step running
  `atmos casts validate repository git-hooks` to the `casts validate all` command, immediately before
  the coverage guard. The hunk matches the same fix carried by open PRs #3509 and #3442 so the three
  merge cleanly.

`casts generate all` is unchanged: the git-hooks recording drives the real repository hook and is
regenerated on its own.

## Validation

- `atmos --chdir=demo/casts casts validate repository git-hooks` with the worktree build: passes.
- The aggregate's coverage guard (`assert_all_validators_reachable("casts validate all")` and
  `assert_no_dumb_term_recordings`) run locally against the edited file: passes.
- The full `casts validate all` run is left to CI, where it previously reached the coverage guard with
  every other validator passing.

## Follow-ups

None.
