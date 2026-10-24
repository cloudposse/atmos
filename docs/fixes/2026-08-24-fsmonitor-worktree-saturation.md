# Fix: Relocate golangci-lint cache/tmp outside worktrees (git fsmonitor saturation)

**Date:** 2026-08-24

## Summary

Commits had grown so slow (tens of seconds to minutes before the signing prompt) that developers
walked away mid-commit. The dominant cause was `core.fsmonitor = true` in the shared repo config on
the affected machine, saturated by filesystem churn that repo tooling placed *inside* each worktree —
chiefly the golangci-lint cache/tmp dirs (156MB / ~37k files, rewritten on every lint run) introduced
by #2701. The fix relocates those dirs to the OS user-cache directory, keyed by a hash of the worktree
path, so no filesystem watcher observing a worktree ever sees lint-cache churn again.

## Context

Measured on the affected machine (macOS, ~91 Conductor worktrees nested under the main checkout):

- `git status --porcelain`: 2.9–10.2s with fsmonitor (one run exceeded a 2-minute timeout);
  0.04–0.15s without. `git diff --name-only`: 6.8s vs 0.02s. `git ls-files -s -z`: up to 8.6s vs
  0.014s. All wall-clock wait at ~0% CPU — git blocked on fsmonitor daemon IPC, not doing work.
- 132+ fsmonitor daemons were running for 91 registered worktrees (dozens orphaned from deleted
  workspaces). Even near-empty worktrees showed 14–68s statuses, i.e. the saturation was
  FSEvents-system-wide, not per-worktree.
- A single pre-commit run issues dozens of index-refreshing git commands (pre-commit's own
  stash/diff/status cycle plus each hook's git calls), each paying that tax. SSH commit signing
  (1Password) prompts only after all hooks pass — minutes later, when the developer was AFK.
- New-worktree creation (a full ~10k-file checkout plus a ~100k-file `pnpm install` under the same
  watched tree) stretched to 30+ minutes under the same saturation.

Why it tipped in July–August 2026, having been fine for a year prior:

- 2026-07-09 `4ee0f7ff4e` (#2701) pointed `GOLANGCI_LINT_CACHE`/`TMPDIR` at `<worktree>/.golangci-cache`
  and `<worktree>/.golangci-tmp` to isolate golangci-lint's machine-global single-instance lock per
  worktree (fixing real cross-worktree lint serialization). Correct goal, wrong location: it moved
  constant high-volume churn under the watchers.
- 2026-07-16 and 2026-07-30 added two `always_run: true` pre-commit hooks (`tracked-symlinks-check`,
  `atmos-validate-editorconfig`), each shelling out to git/atmos unconditionally per commit —
  cheap on a healthy filesystem, expensive under the multiplier.
- Steady growth to ~91 worktrees, most carrying a 675MB `website/node_modules`.

Cleared as non-causes: the 2026-08-19 Mage migration (`a134752c75`; mage startup measures ~0.4s) and
the goimports→gci formatter swap (a documented 15–20x speedup). Also corrected a prior assumption:
Conductor does not use or require git fsmonitor — its change detection is a bundled `watchexec`, and
its binary contains no fsmonitor references. Nothing in this repo sets `core.fsmonitor`; it had been
enabled manually on the machine.

## Changes

Landed in PR #2988.

- `magefiles/mage_lint_golangci_run.go`
  - `setWorktreeIsolationEnv()` now derives the default cache/tmp location from
    `os.UserCacheDir()` instead of the worktree root: `<user-cache>/atmos-lint/<hash>/cache` and
    `<user-cache>/atmos-lint/<hash>/tmp`, where `<hash>` is the first 12 hex chars of the SHA-256 of
    the absolute worktree path (new helper `worktreeLintCacheRoot()`). Per-worktree lock isolation —
    #2701's intent — is preserved; the dirs just live outside any watched tree.
  - The `ATMOS_LINT_SHARED_CACHE=1` opt-out and explicit `GOLANGCI_LINT_CACHE` override behave
    exactly as before.
  - `userCacheDirFunc` is a package var so tests can inject a fake user-cache root.
- `magefiles/mage_lint_golangci_run_test.go` — updated `TestSetWorktreeIsolationEnv` to assert the
  new location (and that it is *not* under the worktree), added `TestWorktreeLintCacheRoot`
  (determinism + cross-worktree non-collision), added a user-cache-dir resolution-failure case, and
  repointed the `runGolangciLintPrecommit` isolation-failure subtest at the new failure path.
- `.gitignore` keeps the legacy `.golangci-cache/` / `.golangci-tmp/` entries so pre-fix leftovers in
  existing worktrees stay ignored; delete those directories at leisure to reclaim disk.

Machine-level remediation applied alongside (not a repo change, recorded for operators hitting the
same wall): `git config core.fsmonitor false` + `git config core.untrackedCache true` in the shared
repo config, then stopping all `git fsmonitor--daemon` processes. `git status` dropped from
2.9s–timeout to 0.04–0.2s across every worktree checked.

## Validation

- `go vet -tags mage ./magefiles/...` and `go build ./...` pass.
- `go test -tags mage ./magefiles/...` passes (full package, including the new/updated cases).
- End-to-end cold run: staged a scratch `.go` edit, ran `go tool mage lint:precommit` — no
  `.golangci-cache`/`.golangci-tmp` created in the worktree; cache and tmp appeared under
  `~/Library/Caches/atmos-lint/<hash>/`; the run correctly flagged the scratch edit's gci violation.
- Warm-cache rerun on the real staged change: 1.5s wall (vs 9.1s cold) — cache reuse confirmed;
  `0 issues.` after fixes.
- No leakage into the real user cache dir from the test suite (fake-injected root verified).
- `gofumpt -l` clean on changed files.
- End-to-end commit: the PR's own commit — every pre-commit hook plus 1Password SSH signing —
  completed in 4.8s wall, where the pre-fix state exceeded a 2-minute `git status` timeout.
- Not validated here: Windows `TMP`/`TEMP` branch behavior (unchanged logic, exercised by the
  existing platform-conditional test on Windows CI).

## Follow-ups

None.
