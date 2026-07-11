#!/usr/bin/env bash
#
# PostToolUse hook (matcher: Bash).
#
# Detects GitHub's post-`git push` vulnerability notice ("GitHub found N
# vulnerabilities ...") and, only when the live set of open Dependabot +
# CodeQL alerts for this repo has actually changed since we last acted on
# it, injects additionalContext telling the session to run the
# security-remediate skill.
#
# This script must never fail or delay the calling `git push`: every
# external call is best-effort, bounded by run_with_timeout below, and any
# error degrades to a silent no-op (exit 0, no output). Do not use `set -e`.
#
# Fork/permission safety (important for open-source contributors): viewing
# Dependabot/code-scanning alerts requires a `gh` token with the
# security_events scope AND at least maintain-level access to the target
# repo. Most external contributors - including anyone working from a fork -
# will not have this, and `gh api` will 404. That is treated identically to
# "no alerts": the hook silently no-ops. Nothing here can block, slow down,
# or change the outcome of a push for a contributor who lacks this access.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Portable bounded-execution helper: prefers GNU coreutils timeout/gtimeout
# if present, otherwise falls back to a manual watchdog. Stock macOS ships
# neither `timeout` nor `gtimeout`, so this cannot assume either exists.
run_with_timeout() {
  local secs="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "$secs" "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$secs" "$@"
    return $?
  fi
  local tmpfile
  tmpfile="$(mktemp)"
  ("$@" >"$tmpfile" 2>/dev/null) &
  local pid=$!
  local i=0
  while kill -0 "$pid" 2>/dev/null; do
    sleep 1
    i=$((i + 1))
    if [ "$i" -ge "$secs" ]; then
      kill -TERM "$pid" 2>/dev/null
      break
    fi
  done
  wait "$pid" 2>/dev/null
  cat "$tmpfile"
  rm -f "$tmpfile"
}

payload="$(cat)"

command -v jq >/dev/null 2>&1 || exit 0

tool_name="$(printf '%s' "$payload" | jq -r '.tool_name // ""' 2>/dev/null)"
[ "$tool_name" = "Bash" ] || exit 0

cmd="$(printf '%s' "$payload" | jq -r '.tool_input.command // ""' 2>/dev/null)"

# Only real pushes: skip dry-runs, tag-only pushes, and non-push commands.
# Uses POSIX bracket classes ([[:space:]], not \s/\b) so this matches on
# stock BSD grep (e.g. minimal/busybox environments), not just GNU grep.
printf '%s' "$cmd" | grep -Eq '(^|[[:space:];&|])git[[:space:]]+push($|[[:space:]])' || exit 0
printf '%s' "$cmd" | grep -Eq -- '--dry-run' && exit 0
printf '%s' "$cmd" | grep -Eq -- '--tags($|[[:space:]])' && exit 0

stdout="$(printf '%s' "$payload" | jq -r '.tool_response.stdout // ""' 2>/dev/null)"
stderr="$(printf '%s' "$payload" | jq -r '.tool_response.stderr // ""' 2>/dev/null)"

printf '%s\n%s' "$stdout" "$stderr" | grep -Eiq 'GitHub found [0-9]+ vulnerabilit' || exit 0

command -v gh >/dev/null 2>&1 || exit 0

cd "$REPO_ROOT" 2>/dev/null || exit 0

owner_repo="$(run_with_timeout 8 gh repo view --json owner,name --jq '.owner.login + "/" + .name' 2>/dev/null)"
[ -n "$owner_repo" ] || exit 0

branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null)"
[ -n "$branch" ] && [ "$branch" != "HEAD" ] || exit 0

# Prefix by source so Dependabot and code-scanning alert numbers (independent
# numbering spaces) never collide when combined into one set. A 404/403 here
# (no security_events access - the common case for fork/external
# contributors) yields empty output, which is handled the same as "no open
# alerts" below.
dependabot_ids="$(run_with_timeout 8 gh api "repos/${owner_repo}/dependabot/alerts" --paginate -f state=open --jq '.[] | "db:" + (.number | tostring)' 2>/dev/null)"
codeql_ids="$(run_with_timeout 8 gh api "repos/${owner_repo}/code-scanning/alerts" --paginate -f state=open --jq '.[] | "cq:" + (.number | tostring)' 2>/dev/null)"

all_ids="$(printf '%s\n%s\n' "$dependabot_ids" "$codeql_ids" | grep -E '^(db|cq):[0-9]+$' | sort -u)"
[ -n "$all_ids" ] || exit 0

hash_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    shasum -a 256 | awk '{print $1}'
  fi
}

current_hash="$(printf '%s' "$all_ids" | hash_stdin)"
[ -n "$current_hash" ] || exit 0

branch_slug="$(printf '%s' "$branch" | tr -c 'A-Za-z0-9._-' '-')"
state_dir="${REPO_ROOT}/.claude/state/security-remediate"
state_file="${state_dir}/${branch_slug}.json"

last_hash=""
if [ -f "$state_file" ]; then
  last_hash="$(jq -r '.hash // ""' "$state_file" 2>/dev/null)"
fi

# Unchanged since we last surfaced this alert set on this branch: stay silent.
[ "$current_hash" = "$last_hash" ] && exit 0

mkdir -p "$state_dir" 2>/dev/null || exit 0

alert_count="$(printf '%s\n' "$all_ids" | grep -c .)"
ts="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
ids_json="$(printf '%s\n' "$all_ids" | jq -R . | jq -s .)"

jq -n --arg hash "$current_hash" --arg ts "$ts" --argjson ids "$ids_json" \
  '{hash: $hash, alert_ids: $ids, ts: $ts}' > "$state_file" 2>/dev/null || exit 0

jq -n --arg ctx "GitHub reported open security alerts after this push (${alert_count} open Dependabot/CodeQL alerts on ${owner_repo}). Run the security-remediate skill now: query the live Dependabot and CodeQL alerts for this repository, fix high-confidence findings directly on the current branch (${branch}), run make lint and targeted go test on touched packages, commit the fixes to this branch (do not open a new PR or issue), and report a summary of what was fixed vs. not auto-fixable." \
  '{hookSpecificOutput: {hookEventName: "PostToolUse", additionalContext: $ctx}}'

exit 0
