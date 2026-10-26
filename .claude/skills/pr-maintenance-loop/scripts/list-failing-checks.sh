#!/usr/bin/env bash
# List currently failing (non-pending, non-passing) CI checks on the current branch's PR, with a
# failure-log excerpt where one can be fetched, plus the diff vs origin/main — for the consuming
# agent to judge whether a failure is caused by this patch or is pre-existing/unrelated. Does no
# classification itself, same "script fetches, agent reasons" split as patch-test-coverage.sh.

set -euo pipefail

pr_number="$(gh pr view --json number -q .number)"

checks="$(gh pr checks "$pr_number" --json name,bucket,link 2>&1)"
failing="$(echo "$checks" | jq -c '[.[] | select(.bucket == "fail")]')"
fail_count="$(echo "$failing" | jq 'length')"

if [[ "$fail_count" -eq 0 ]]; then
    echo "STATUS: ALL_CHECKS_GREEN"
    echo "No failing CI checks on PR #${pr_number} (pending checks, if any, are not failures)."
    exit 0
fi

echo "STATUS: CHECKS_FAILING"
echo "${fail_count} failing check(s) on PR #${pr_number}:"
echo

echo "$failing" | jq -c '.[]' | while IFS= read -r check; do
    name="$(echo "$check" | jq -r '.name')"
    link="$(echo "$check" | jq -r '.link')"
    echo "=== ${name} ==="
    echo "LINK: ${link}"

    run_id=""
    if [[ "$link" =~ /actions/runs/([0-9]+) ]]; then
        run_id="${BASH_REMATCH[1]}"
    fi

    if [[ -n "$run_id" ]]; then
        echo "--- failure log (gh run view ${run_id} --log-failed, truncated) ---"
        gh run view "$run_id" --log-failed 2>&1 | tail -100 || echo "(could not fetch log for run ${run_id})"
    else
        echo "(no fetchable Actions run log for this check type — see LINK above)"
    fi
    echo
done

echo "== Touched files vs origin/main =="
git diff --name-only --diff-filter=ACMR origin/main...HEAD

echo
echo "DISCLAIMER: this only lists what is CURRENTLY failing; it does not judge whether a failure is"
echo "caused by this patch. Some checks (Acceptance Tests) run the full suite, wider than this"
echo "loop's own patch-scoped lint/test checks — a failure there may be in a package this patch"
echo "never touched. Verify causation before attempting any fix; when in doubt, report, don't fix."
