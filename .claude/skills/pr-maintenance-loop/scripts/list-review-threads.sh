#!/usr/bin/env bash
# List unresolved, non-outdated CodeRabbit review threads on the current branch's PR.
#
# Read-only (a plain GraphQL query, no mutation) — prints each thread's node ID (the value
# `atmos fix comments --thread-id` needs), file path, comment URL, and a one-line preview of the
# comment body, so a human or agent can see what's outstanding before acting on a specific thread.

set -euo pipefail

owner="$(gh repo view --json owner -q .owner.login)"
repo="$(gh repo view --json name -q .name)"
pr_number="$(gh pr view --json number -q .number)"

# reviewThreads has no orderBy and returns oldest-first, so `first: N` silently drops the newest
# threads once a long-running PR passes N total (confirmed for real: this PR reached 65 threads,
# and `first: 50` made the 5 newest -- an entire fresh CodeRabbit review round -- invisible here
# while still genuinely unresolved on GitHub). `last: 100` pages backward from the end of the
# connection instead, mirroring the identical fix already applied to the fix-all skill's own
# step-9 CodeRabbit-approval query for the same underlying GraphQL pagination behavior.
#
# comments' authorship check reads the THREAD-STARTING comment (`first: 1`), not the latest reply
# (`last: 1`): a thread CodeRabbit opened stays a CodeRabbit thread even after a human or agent
# posts a reply without resolving it (the documented "skip as invalid, leave open" path in
# fix-all's step 5) -- filtering on the last comment's author would make that reply itself un-list
# the thread, hiding it from every future cycle.
result="$(gh api graphql -f query='
query($owner: String!, $repo: String!, $number: Int!) {
	repository(owner: $owner, name: $repo) {
		pullRequest(number: $number) {
			reviewThreads(last: 100) {
				nodes {
					id
					isResolved
					isOutdated
					path
					comments(first: 1) {
						nodes { author { login } body url }
					}
				}
			}
		}
	}
}' -f owner="$owner" -f repo="$repo" -F number="$pr_number" \
	--jq '[.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false and .isOutdated == false and .comments.nodes[0].author.login == "coderabbitai")]')"

echo "$result" | jq -r '.[] | "THREAD  \(.id)\nPATH    \(.path // "(no path)")\nURL     \(.comments.nodes[0].url)\nPREVIEW \(.comments.nodes[0].body | split("\n")[0] | .[0:120])\n"'

count="$(echo "$result" | jq 'length')"
echo "${count} unresolved CodeRabbit thread(s) on PR #${pr_number}."
