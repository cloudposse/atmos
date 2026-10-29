---
name: lint
description: "Run golangci-lint and fix findings. Patch-aware by default (only lines changed vs origin/main, matching CI's real gate) — pass full-repo to lint everything instead. Invoke on explicit requests like \"lint this\" / \"run a full lint\", or from within the fix-all skill's cycle."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.1.0"
---

# Lint (patch-aware by default)

Runs this repo's real lint gate — the custom `golangci-lint` binary with the `lintroller` plugin
(`.custom-gcl.yml`), the same one `.github/workflows/codeql.yml`'s `lint-golangci` job runs — and
fixes what it finds.

## Default mode: patch-aware

Run:

```
atmos fix lint
```

This delegates to the existing `atmos lint --changed` (`.atmos.d/lint.yaml`), which builds
`go.mod` compatibility checks, then builds `./custom-gcl` — **staleness-guarded**: it only
rebuilds when the binary is missing or older than its build inputs (`.custom-gcl.yml` or the
`lintroller` plugin sources), so this is cheap in the common case, not a fresh clone-and-compile
every cycle. No separate "is it built" precondition needed; `atmos fix lint` handles that safely
on its own.

The underlying lint run is scoped to `--new-from-rev=origin/main` — only findings on lines changed
vs `origin/main`, exactly matching CI's real gate (`.github/workflows/codeql.yml`'s
`lint-golangci` job).

Zero findings → one-line no-op summary, done.

## Repository checks and commit hooks

For the complete pre-commit suite, use `atmos check staged` (the index),
`atmos check pr` (changes since `origin/main`), or `atmos check all` (all tracked
files). These commands are read-only, including checks that normally fix files.
Use `--base-ref <branch-or-SHA>` for another comparison base, including a parent
branch in a PR stack. Missing refs must be resolved explicitly; checks never fetch.

The catalog is [`.atmos-git-hooks.yaml`](../../../.atmos-git-hooks.yaml).
It declares each check's handler, file patterns, exclusions, and whether it fixes
files. The handlers in `checks.star` run sequentially through Atmos's test UI.
Preserve each check's exclusions when editing the catalog; snapshots and fixtures
have intentional formatting. `branch-file-limit` allows at most 150 changed files
against the merge base, including staged changes; split larger diffs into PRs.

Use `atmos format staged|pr|all` to apply formatting and module cleanup. Review the
diff, then stage only intended changes. A failed commit may already have returned
fixes as **unstaged edits**; inspect those before retrying. Hooks never change the
index. If a fix overlaps unstaged or concurrent edits, the original file is kept
and the diagnostic names a recovery directory with a proposed patch and
before/after files. Review and reconcile it manually. Never blindly stage all
changes or bypass the hook with `--no-verify`.

Run `atmos dev setup` to build Atmos, prepare pinned check tools and custom-gcl,
and install the commit hook. For stale native APIs, run `atmos dev checks-build`;
for a missing/stale custom linter, run `go tool mage lint:customGCL`. Builds and
tool installation happen outside hooks. Preserve custom hooks and `core.hooksPath`
conflicts, and account for hook directories shared by worktrees. See
[development setup and recovery](../../../docs/development.md#setup-and-recovery).
Do not install or invoke the pre-commit Python framework.

## Full-repo mode (explicit only)

Only when a human explicitly asks for a full lint (e.g. "run a full lint", "lint the whole repo"
— never inferred, never run from the automated loop). Run the same staleness-guarded build the
patch-aware mode uses (`atmos lint custom-gcl` — only rebuilds `./custom-gcl` when missing or
older than its build inputs) before invoking the binary directly, so a fresh checkout or a stale
binary after `.custom-gcl.yml`/`lintroller` changes doesn't fail outright or silently report wrong
findings:

```bash
atmos lint custom-gcl
./custom-gcl run --config=.golangci.yml
```

(no `--new-from-rev`, so it reports every existing finding, not just new ones — expect this to
surface pre-existing issues unrelated to any current patch).

## Fixing findings

Delegate to `Agent subagent_type: "lint-fix"`, passing the raw `custom-gcl` output. The agent
fixes what it can, re-runs the same lint command to confirm clean, and reports anything it
skipped (with a reason) rather than forcing a fix — e.g. a finding that requires a broader
refactor than patch scope. For a skipped finding when called from the automated loop, invoke the
[`say` skill](../say/SKILL.md) with a short message like `"PR <number> has a lint finding needing
your input."`

Follow the loop's existing git-hygiene discipline when committing a fix: verify the commit is
signed (`git log --show-signature -1`), `git add` only the files actually touched, plain
(never force) `git push`.

## Related

- **[`lint-fix` agent](../../agents/lint-fix.md)** — does the actual fixing.
- **[`fix-all` skill](../fix-all/SKILL.md)** — invokes this skill's default (patch-aware) mode at
  step 6 (and reuses it directly at step 2 for CI-sourced lint findings). Scheduled hourly by
  [`pr-maintenance-loop`](../pr-maintenance-loop/SKILL.md), or run this directly for a one-shot.
- **[`say` skill](../say/SKILL.md)** — invoked when a finding is skipped and needs human input.
- `atmos fix lint` (`.atmos.d/fix.yaml`) — the custom command this skill's default mode runs;
  delegates to the existing `atmos lint --changed` (`.atmos.d/lint.yaml`), also used by the
  Atmos-managed pre-commit hook via `go tool mage lint:precommit` (`magefiles/`).
