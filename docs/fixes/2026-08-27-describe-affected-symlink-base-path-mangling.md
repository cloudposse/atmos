# Fix: `describe affected` mis-computes BASE paths when the repo is under a symlinked path

**Date:** 2026-08-27

## Issue

Running `atmos describe affected` from a repository whose working directory
involves a symlink (e.g. macOS `/tmp` → `/private/tmp`, `/var` →
`/private/var`, a symlinked home or workspace directory) silently produced a
wrong affected set. Depending on filesystem depth, one of two opposite
failures occurred:

- **Everything affected**: the BASE stack scan pointed at a nonexistent
  directory, found no stack manifests, and the greenfield fallback treated
  BASE as empty — every HEAD component reported as affected, with only a
  `Warn`.
- **Nothing affected**: the mis-computed path climbed past the filesystem
  root, `filepath.Join` clamped it, and it landed back on the **HEAD**
  repository — BASE silently compared equal to HEAD and real changes were
  reported as unaffected (silent under-detection).

Discovered during an adversarial field-test pass of the merged-PR base
resolution fix: the E2E fixture lived under `/tmp` and reported every
component affected for a diff that touched none.

## Root cause

`internal/exec/describe_affected_utils.go` re-bases the config's absolute
paths onto the BASE worktree via `filepath.Rel(repoRootAbs, configPathAbs)` +
`filepath.Join(worktreePath, rel)`. The two inputs come from different
sources with different symlink treatment:

- the repo root comes from git (go-git reports the **symlink-resolved**
  path, e.g. `/private/tmp/repo`), while
- the config paths derive from the CWD as the shell reported it (`$PWD`,
  **logical/unresolved**, e.g. `/tmp/repo/stacks` — Go's `os.Getwd` honors
  `$PWD` when it points at the same directory).

`filepath.Rel` between the two forms returns a `../..`-climbing path instead
of `stacks`, and joining that onto the worktree escapes it entirely. Nothing
validated the result, so the failure surfaced only as one of the two silent
wrong answers above.

## Fix

`internal/exec/describe_affected_utils.go`:

1. **Symlink normalization**: both sides of the `filepath.Rel` computation
    are passed through `evalSymlinksBestEffort`, which resolves symlinks even
    for paths that don't fully exist (resolves the deepest existing ancestor
    and re-appends the remainder — unused helmfile/packer default dirs
    routinely don't exist).
2. **Escape guard**: if a computed relative path still starts with `..`
    after normalization (a config path genuinely outside the repository), the
    re-basing returns a hard error wrapping `errUtils.ErrGitPathEscapesWorktree`
    naming both paths — never a silent guess in either direction.

The re-basing block was extracted into testable helpers
(`rebaseConfigPathsOntoWorktree`, `rebaseOnePathOntoWorktree`,
`evalSymlinksBestEffort`).

The greenfield "treat BASE as empty" fallback was deliberately left
unchanged: gating it on path existence would break the legitimate case of a
PR introducing Atmos in a subdirectory that doesn't exist in BASE. The escape
guard removes the only demonstrated way for a *wrong* path to reach that
fallback.

## Testing

Red-first: `TestDescribeAffectedCIBaseE2E/repo_under_symlinked_path...`
(`internal/exec/describe_affected_test.go`) builds a real repo, symlinks it,
and diffs against a base that genuinely differs in one stack — the assertion
(`prod-c1`,`prod-c2` exactly) discriminates the correct result from both
failure modes (empty vs. everything). It failed with an empty affected set
before the fix. Unit tests cover `evalSymlinksBestEffort` (existing symlink,
missing-suffix, unresolvable input) and `rebaseOnePathOntoWorktree`
(plain, symlinked-mixed-forms, and the escape hard-error asserted via
`errors.Is` against the sentinel).

## Lessons

- Path arithmetic that mixes values from different provenance (git vs. CWD
  vs. config) must normalize symlinks first; `filepath.Rel` is only correct
  when both inputs share one canonical form.
- A fallback that silently absorbs "path not found" turns path bugs into
  plausible-looking wrong answers in whichever direction the arithmetic
  happens to land; validate the arithmetic before the fallback can see it.
