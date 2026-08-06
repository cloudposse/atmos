#!/usr/bin/env bash
# Reply to (and optionally resolve) a GitHub PR review thread.
#
# This is a deliberately narrow wrapper around exactly two GraphQL mutations —
# addPullRequestReviewThreadReply and resolveReviewThread. It never accepts
# arbitrary GraphQL query text: --body only ever becomes the *value* of a
# variable, passed via `gh api graphql -f`, so it cannot be interpolated into
# the mutation selection even if fully attacker-controlled. This lets
# .claude/settings.json allowlist this script specifically without touching
# the blanket "deny any gh api graphql mutation" rule that exists to stop a
# prompt-injected PR comment from driving mergePullRequest/closePullRequest.
#
# Usage:
#   gh-resolve-review-thread.sh --thread-id <node-id> --body <text> [--resolve]
#
# Default is reply-only (safe default). --resolve additionally marks the
# thread resolved — callers must only pass --resolve when there is a concrete
# commit SHA that fixes the thread's finding.

set -euo pipefail

THREAD_ID=""
BODY=""
RESOLVE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --thread-id)
            THREAD_ID="${2:-}"
            shift 2
            ;;
        --body)
            BODY="${2:-}"
            shift 2
            ;;
        --resolve)
            RESOLVE=1
            shift
            ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

if [[ -z "$THREAD_ID" || -z "$BODY" ]]; then
    echo "Usage: $0 --thread-id <node-id> --body <text> [--resolve]" >&2
    exit 1
fi

gh api graphql -f query='
mutation($id: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $id, body: $body}) {
    comment { id url }
  }
}' -f id="$THREAD_ID" -f body="$BODY"

if [[ "$RESOLVE" -eq 1 ]]; then
    gh api graphql -f query='
mutation($id: ID!) {
  resolveReviewThread(input: {threadId: $id}) {
    thread { id isResolved }
  }
}' -f id="$THREAD_ID"
fi
