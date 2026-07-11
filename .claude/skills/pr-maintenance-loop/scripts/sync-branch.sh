#!/usr/bin/env bash
# Keep the current branch's PR up to date with origin/main, and keep this local checkout in sync
# with the remote PR branch.
#
# Two distinct things, both necessary:
# 1. If behind origin/main, update via `gh pr update-branch` (GitHub-side merge — no local
#    rebase, no force-push, GitHub signs the merge commit itself).
# 2. `gh pr update-branch` only updates the *remote* branch via GitHub's API — it never touches
#    this local checkout. So every run also fetches and fast-forwards local HEAD to match the
#    remote branch, whether or not step 1 just ran. Skipping this leaves local git state stale
#    for anything that diffs against origin/main (lint, coverage) and can get a later `git push`
#    rejected as non-fast-forward. Confirmed for real: a cycle read mergeStateStatus as BLOCKED
#    (not BEHIND), skipped the rebase check, and a later local diff against a stale origin/main
#    wrongly flagged an already-merged, unrelated commit as a new finding on this patch.
#
# The fast-forward is deliberately fail-closed: if it isn't a clean fast-forward, this script
# exits non-zero rather than forcing a merge or rewrite — that needs a human.

set -euo pipefail

branch="$(git branch --show-current)"
pr_number="$(gh pr view --json number -q .number)"
merge_status="$(gh pr view --json mergeStateStatus -q .mergeStateStatus)"

echo "Branch: ${branch}  PR: #${pr_number}  mergeStateStatus: ${merge_status}"

git fetch origin main --quiet

behind=0
if [[ "$merge_status" == "BEHIND" ]]; then
    behind=1
elif ! git merge-base --is-ancestor origin/main HEAD; then
    behind=1
fi

if [[ "$behind" -eq 1 ]]; then
    echo "Behind origin/main — updating via gh pr update-branch..."
    gh pr update-branch "$pr_number"
else
    echo "Not behind origin/main."
fi

git fetch origin "$branch" --quiet
if git merge --ff-only "origin/${branch}"; then
    echo "Local checkout is in sync with origin/${branch}."
else
    echo "ERROR: local checkout could not fast-forward to origin/${branch} — this needs human attention, not a forced merge." >&2
    exit 1
fi

if [[ "$merge_status" == "DIRTY" ]]; then
    echo "WARNING: PR has a real merge conflict (DIRTY). Not auto-resolved — needs human attention." >&2
fi
