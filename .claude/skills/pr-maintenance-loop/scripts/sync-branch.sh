#!/usr/bin/env bash
# Keep the current branch's PR up to date with origin/main, and keep this local checkout in sync
# with the remote PR branch.
#
# Three things happen here:
# 1. If behind origin/main, first try `gh pr update-branch` (GitHub-side merge — no local
#    rebase, no force-push, GitHub signs the merge commit itself). This is the cheap path and
#    works whenever there's no real conflict.
# 2. If that fails (a real conflict — GitHub can't auto-merge), fall back to a LOCAL merge attempt
#    so the actual conflict can be resolved instead of just reported. This script does not itself
#    decide how to resolve content conflicts — that requires understanding intent, which is a job
#    for the consuming agent, not a shell script (same "script fetches, agent reasons" split as
#    patch-test-coverage.sh). If the local merge conflicts, this script prints
#    `STATUS: MERGE_CONFLICT`, the conflicted file paths, and their full conflict-marker content,
#    then exits non-zero with the merge left in progress (MERGE_HEAD set, markers in the working
#    tree) for the caller to resolve, `git add`, and commit. If the local merge succeeds cleanly
#    with no conflicts (GitHub's mergeable check can be stale), this script commits and pushes it
#    directly — that's a purely mechanical completion of step 1, not a judgment call.
# 3. `gh pr update-branch` (or this script's own merge+push) only updates the *remote* branch —
#    the local checkout still needs an explicit fetch + fast-forward to catch up, every run,
#    regardless of whether step 1/2 did anything this time. Skipping this leaves local git state
#    stale for anything that diffs against origin/main (lint, coverage) and can get a later
#    `git push` rejected as non-fast-forward. Confirmed for real: a cycle read mergeStateStatus as
#    BLOCKED (not BEHIND), skipped the rebase check, and a later local diff against a stale
#    origin/main wrongly flagged an already-merged, unrelated commit as a new finding on this
#    patch. This final fast-forward is deliberately fail-closed: if it isn't clean, the script
#    exits non-zero rather than forcing a merge or rewrite — that needs a human.

set -euo pipefail

branch="$(git branch --show-current)"
pr_number="$(gh pr view --json number -q .number)"
merge_status="$(gh pr view --json mergeStateStatus -q .mergeStateStatus)"

echo "Branch: ${branch}  PR: #${pr_number}  mergeStateStatus: ${merge_status}"

git fetch origin main --quiet

behind=0
if [[ "$merge_status" == "BEHIND" || "$merge_status" == "DIRTY" ]]; then
    behind=1
elif ! git merge-base --is-ancestor origin/main HEAD; then
    behind=1
fi

if [[ "$behind" -eq 1 ]]; then
    echo "Behind origin/main — attempting update via gh pr update-branch..."
    update_err="$(mktemp)"
    if gh pr update-branch "$pr_number" 2>"$update_err"; then
        echo "Updated via gh pr update-branch."
        rm -f "$update_err"
    else
        echo "gh pr update-branch failed (likely a real conflict): $(cat "$update_err")"
        rm -f "$update_err"
        echo "Attempting a local merge to find the actual conflict..."
        if git merge --no-commit --no-ff origin/main --quiet; then
            git commit -m "Merge branch 'main' into ${branch}"
            git push
            echo "No real conflict — origin/main merged cleanly locally, committed, and pushed."
        else
            echo "STATUS: MERGE_CONFLICT"
            echo "Local merge of origin/main has real conflicts. The merge is left in progress"
            echo "(MERGE_HEAD set, markers in the working tree) for resolution."
            echo
            echo "Conflicted files:"
            git diff --name-only --diff-filter=U
            echo
            for f in $(git diff --name-only --diff-filter=U); do
                echo "=== ${f} ==="
                cat "$f"
                echo
            done
            exit 1
        fi
    fi
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
