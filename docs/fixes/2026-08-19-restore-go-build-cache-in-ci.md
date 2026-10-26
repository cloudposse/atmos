# Fix: Restore `actions/setup-go`'s Go build cache in `.github/workflows/test.yml`

**Date:** 2026-08-19

## Summary

`.github/workflows/test.yml` had `cache: false` set on all 7 of its `actions/setup-go` steps
(`build`, `terraform-registry-cache`, `test`, `magefiles`, `coverage`, `floci-go`,
`kubernetes-e2e`), two of them with a comment claiming this avoided "restoring or saving mutable
cache entries from PR-controlled keys." Researched GitHub's actual documented cache access model:
that rationale doesn't hold — GitHub Actions already isolates PR-branch caches (a `pull_request`
run gets read/write access only to its own branch's cache scope, read-only access to the default
branch's cache, and cannot restore caches from sibling PRs; only `push`/`workflow_dispatch`/
`schedule`/etc. can write to the default branch's cache scope at all). Restored `cache: true`
explicitly on all 7 steps, replacing the inaccurate comment with one describing the actual
isolation model.

## Context

While auditing PR #2955's CI runtime (~40 minutes end to end, dominated by `Build` + the 30-way
sharded `Acceptance Tests` matrix), the user asked why Go module/build caching was disabled there
while `codeql.yml` and `pre-commit.yml` both use `cache: true`. My first answer speculated that a
malicious PR could get its cache promoted into `main`'s cache lineage via the `merge_group` trigger
or the post-merge `push` run — this was wrong, and the user correctly pushed back ("i thought the
way it works, is it caches from main, never across PRs").

Researched GitHub's own docs directly (not from memory) and confirmed:

- A workflow run gets read/write access only to its **own current branch's** cache scope.
- Every branch gets **read-only** access to the **default branch's** (`main`'s) cache — this
  cascades downward to PRs, not upward from them.
- "Workflow runs cannot restore caches created for child branches or sibling branches" — a PR's
  cache, scoped to its merge ref (`refs/pull/N/merge`), is isolated from every other PR and from
  `main`.
- Only `push`, `workflow_dispatch`, `repository_dispatch`, `delete`, `registry_package`,
  `page_build`, and `schedule` can **write** to the default branch's cache scope. Every other
  trigger — including `pull_request` and `merge_group` — gets **read-only** access to it.

So there is no cross-PR or PR→main cache-poisoning path here for GitHub Actions cache to guard
against; the "PR-controlled keys" comment overstated the actual attack surface.

Separately investigated the one *real*, historically-documented Windows caching incident in this
repo: PR #2713 (2026-07-13, `fix(io): migrate fmt.Fprintf/Println anti-patterns...` — unrelated in
subject, the fix was bundled into that branch's CI-stabilization commits). At the time, caching
*was* enabled, and a much larger dependency tree (AI/MCP/LSP/k8s/OPA/Helm v4) pushed a cold-cache
Windows `build` job to the edge of its then-30-minute timeout; the job was killed mid cache-save
(observed at 30m10s), which cascaded into false failures in downstream jobs depending on `build`
(e.g. the `k3s` `demo-helmfile` gate). The fix at the time was to bump the `build` job's timeout to
45 minutes for headroom — it was **not** to disable caching. `cache: false` was introduced later
and separately, on 2026-08-02 in commit `9343caf702` (a large squashed `refactor(store)` PR whose
own commit message never mentions caching at all — the change and its "PR-controlled keys" comment
appear to have been swept in incidentally, with no documented rationale beyond the inline comment
itself).

Since the `build` job's 45-minute timeout headroom (the actual fix for the 2026-07-13 incident) is
still in place, and none of these 7 jobs' current `timeout-minutes` (`build`: 45, `test`: 40–65,
`terraform-registry-cache`: 20, `magefiles`/`coverage`: 15, `floci-go`: 30, `kubernetes-e2e`: 20)
were reduced since then, restoring the cache should not reintroduce that specific historical
failure mode.

## Changes

- `.github/workflows/test.yml`: `cache: false` → `cache: true` (explicit, not just the implicit
  default, so there's a concrete line for a future comment/change to anchor to) on all 7
  `actions/setup-go` steps: `build`, `terraform-registry-cache`, `test`, `magefiles`, `coverage`,
  `floci-go`, `kubernetes-e2e`.
- Replaced the "Disable setup-go's module cache to avoid restoring or saving mutable cache entries
  from PR-controlled keys" comment (present on the `build` and `test` jobs' steps) with one
  correctly describing GitHub's actual branch-scoped cache isolation model, kept once on the
  `build` job's step rather than duplicated on all 7.
- One `actions/setup-go` step in this same file (`test.yml`'s "Set up Go for the Buildx cache
  integration test") already had no `cache:` override (implicit default `true`) and was left
  untouched — nothing to restore there.

## Validation

- `python3 -c "import yaml; yaml.safe_load(...)"` — YAML parses cleanly.
- `atmos validate --affected --exclude 'tests/fixtures/**' --exclude '**/*.go' --format rich` —
  passes, including the GitHub Actions workflow validation this repo's own CI runs
  ("✓ No GitHub Actions workflow validation findings").
- Not locally testable beyond that: `actions/setup-go`'s cache restore/save behavior only executes
  on GitHub's runners. The actual CI run on this PR after pushing is the integration test for this
  change — watch `Build (windows)` in particular (the job with the one historically-documented
  timing incident) to confirm it stays comfortably inside its 45-minute budget with caching back
  on.

## Follow-ups

None.
