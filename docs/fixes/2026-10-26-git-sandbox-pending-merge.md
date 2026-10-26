# Fix: Repository checks treat a pending merge as branch changes

**Date:** 2026-10-26

## Summary

Committing a merge from `main` failed the repository checks even when the branch
changed few files. `branch-file-limit` reported every file merged in as a branch
change, and `golangci-lint` reported findings in code that arrived from `main`.

## Context

`git.sandbox` runs checks in a private copy of the repository. That copy kept
`HEAD` and a literal `origin/main` but dropped `MERGE_HEAD`, so no check could tell
that the commit being made was a merge.

- `branch-file-limit` measured from `merge-base HEAD <base>`. During a merge `HEAD`
  is only the first parent, so that base was the previous merge from `main` and the
  whole incoming diff counted. A branch of 71 files measured as 949.
- `lint:precommit` already scopes findings to a base revision while a merge is in
  progress, but that safeguard reads `MERGE_HEAD`, which the sandbox had discarded.
  Code merged in from `main` was linted as if the branch had written it.
- The sandbox copied only `refs/remotes/origin/main`, whatever base the caller
  asked for with `--base-ref`, so a stacked branch could not resolve its own base.

## Changes

- Carry `MERGE_HEAD` into the sandbox so checks see the commit as a merge. Git
  refuses to write that pseudoref through `update-ref`, so the sandbox writes the
  file directly.
- Copy the comparison base the caller requested, under its full ref name, instead
  of a hardcoded `origin/main`. A base given as a commit needs no ref.
- `branch-file-limit` measures from the merge base of the base and both parents
  while a merge is pending, so only the branch's own files count.
- `golangci-lint` receives the resolved base commit rather than relying on a ref
  name existing in the sandbox.

## Validation

- Sandbox tests cover a pending merge, an ordinary commit, a non-default base, the
  default base, and commit, missing, and option-like bases.
- An end-to-end test merges 200 files into a one-file branch and expects the check
  to pass, then stages 150 of the branch's own files and expects the check to fail.
  It fails against the previous `checks.star`.
- The full pre-commit hook passes on the merge that exposed the problem.

## Follow-ups

The installed Git hook shim embeds the absolute path of the `build/atmos` from
whichever worktree last ran `atmos git hooks install`, and the hooks directory is
shared by every worktree. Each worktree therefore runs another worktree's binary,
which may predate these fixes, and a worktree whose binary is missing silently
falls back to the `atmos` on `PATH`.
