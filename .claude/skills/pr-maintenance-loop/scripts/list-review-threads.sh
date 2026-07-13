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

result="$(gh api graphql -f query='
query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 50) {
        nodes {
          id
          isResolved
          isOutdated
          path
          comments(last: 1) {
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
