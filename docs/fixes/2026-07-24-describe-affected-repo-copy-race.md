# Fix: flaky repo-copy race in describe-affected tests

**Date:** 2026-07-24

## Summary

The Linux Acceptance Tests job failed with `TestDescribeAffectedDeletedComponentWithDependents`
in `internal/exec`, panicking on an unexpected error:

```text
stat ../../.git/objects/pack/tmp_rev_G500qY: no such file or directory
```

Fixed by retrying the whole-repository copy used to set up these tests when it fails
because a source file vanished mid-copy.

## Context

Several `describe affected` tests (`setupDescribeAffectedTest` and
`setupDescribeAffectedTestWithFixture` in `internal/exec/describe_affected_test.go`)
copy the live, currently-checked-out repository (including `.git`) into a temp dir via
`github.com/otiai10/copy`, to simulate comparing HEAD against a modified BASE.

`otiai10/copy`'s directory walk lists a directory with `os.ReadDir`, then calls
`DirEntry.Info()` (which internally `Lstat`s) on every listed entry before copying any
of them. Git's own background housekeeping (an automatic repack triggered by other
tests/processes running concurrently against the same live repo) writes and then
renames/removes transient `tmp_pack_*`/`tmp_idx_*`/`tmp_rev_*` files under
`.git/objects/pack/` within milliseconds. If one of those files is listed but vanishes
before `.Info()` reads it, the library returns a bare `stat: no such file or directory`
error for that entire directory batch (this specific `.Info()` call path isn't covered
by the library's own `os.IsNotExist` tolerance, which only applies to the initial
`os.ReadDir` and to `os.Open` inside `fcopy`) — failing the whole test, non-deterministically,
depending on whether a repack happens to be mid-flight at copy time.

GitHub Job: Acceptance Tests (linux), Job ID 89514178007. The GitHub Actions log
attachment for this job only captured the final ~1000 lines of a run with heavy
Terraform output interleaved, which cut off the actual `--- FAIL:` detail; the full
detail was fetched via `gh api repos/cloudposse/atmos/actions/jobs/89514178007/logs`.

## Changes

- `internal/exec/describe_affected_test.go`: added `copyRepoWithRetry`, which retries
  the whole-repo `cp.Copy` call up to 5 times (with a short sleep and a clean
  `RemoveAll` of the partial destination between attempts) when the copy fails with
  `os.IsNotExist`. Both call sites that copy the live repository (in
  `setupDescribeAffectedTest` and `setupDescribeAffectedTestWithFixture`) now use it
  instead of calling `cp.Copy` directly.
- Left untouched: the second `cp.Copy` call in each setup function (copying fixture
  stacks over the temp copy) doesn't touch `.git` and isn't subject to this race.
  Audited other `otiai10/copy` call sites in the repo
  (`tests/describe_affected_include_test.go`, `tests/describe_affected_greenfield_test.go`,
  vendoring/copy-glob code, `tests/cli_test.go`) — none of them copy the live `.git`
  directory (they either copy specific fixture subpaths and `git.PlainInit` a fresh
  repo, or copy unrelated component/vendor sources), so none share this race.

## Validation

- `go build ./...` — passed.
- `go test ./internal/exec/... -run 'TestDescribeAffected' -v -count=1` — all
  `TestDescribeAffected*` subtests pass, including the previously-failing
  `TestDescribeAffectedDeletedComponentWithDependents`.
- `go test ./internal/exec/... -count=1` (full package, matching the package that
  failed on Linux CI) — passed twice in a row, `~200-300s` each.
- `atmos lint --changed` (patch-scoped, this repo's real PR gate) — passed, 0 issues.
- Not reproduced as a hard failure locally (the race requires a concurrent repack
  mid-copy, which is timing-dependent); the fix directly targets the documented
  `os.IsNotExist` failure mode from the CI log.

## Follow-ups

None.
