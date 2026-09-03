#!/usr/bin/env bash
# Self-test for classify-infra-failures.sh: one fixture per class, plus a paginated
# input check. Plain bash, no test framework.
# Run: bash .github/scripts/classify-infra-failures-test.sh
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
classifier="$here/classify-infra-failures.sh"

# job NAME CONCLUSION JOB_COMPLETED_AT STEPS_JSON -> one job object.
job() {
  jq -cn --arg name "$1" --arg conclusion "$2" --arg completed "$3" --argjson steps "$4" \
    '{name: $name, conclusion: $conclusion, status: "completed", completed_at: $completed, steps: $steps}'
}
# step NAME STATUS CONCLUSION COMPLETED_AT -> one step object ("null" conclusion / "" time -> JSON null).
step() {
  jq -cn --arg name "$1" --arg status "$2" --arg conclusion "$3" --arg completed "$4" \
    '{name: $name, status: $status, conclusion: (if $conclusion == "null" then null else $conclusion end),
      completed_at: (if $completed == "" then null else $completed end)}'
}
steps() { printf '%s\n' "$@" | jq -cs '.'; }

ok_steps=$(steps \
  "$(step 'Set up job' completed success 2026-09-03T16:00:00Z)" \
  "$(step 'Acceptance tests' completed success 2026-09-03T16:09:30.123Z)" \
  "$(step 'Post Harden Runner' completed skipped 2026-09-03T16:09:35Z)" \
  "$(step 'Complete job' completed success 2026-09-03T16:09:36Z)")

# Reaped 34 minutes after the last step: the runner lost DNS and never reported completion.
stuck=$(job 'Build (windows)' cancelled 2026-09-03T16:43:17Z "$ok_steps")
# Same steps, but completion landed 4 s after the last step: an ordinary cancellation.
quick_cancel=$(job 'Build (windows)' cancelled 2026-09-03T16:09:40Z "$ok_steps")
# Failure with no failed step at all.
lost=$(job 'Acceptance Tests (macos, shard 4/10)' failure 2026-09-03T16:50:00Z \
  "$(steps "$(step 'Set up job' completed success 2026-09-03T16:00:00Z)")")
# Aggregator job: its only failed step just reads other jobs' verdicts.
cascade=$(job 'Acceptance Tests (windows)' failure 2026-09-03T16:43:31Z "$(steps \
  "$(step 'Harden Runner' completed success 2026-09-03T16:43:26Z)" \
  "$(step 'Check per-OS test matrix result' completed failure 2026-09-03T16:43:28Z)" \
  "$(step 'Check terraform-registry-cache result' completed skipped 2026-09-03T16:43:28Z)" \
  "$(step 'Complete job' completed success 2026-09-03T16:43:31Z)")")
# Cancelled mid-step: a newer push replaced this run.
superseded=$(job 'Acceptance Tests (linux, shard 2/10)' cancelled 2026-09-03T16:05:00Z "$(steps \
  "$(step 'Set up job' completed success 2026-09-03T16:00:00Z)" \
  "$(step 'Acceptance tests' completed cancelled 2026-09-03T16:05:00Z)" \
  "$(step 'Complete job' completed skipped '')")")
# Cancelled before any step ran.
never_started=$(job 'Acceptance Tests (linux, shard 3/10)' cancelled 2026-09-03T16:05:00Z '[]')
# A genuinely failed test step.
real=$(job 'Acceptance Tests (linux, shard 4/10)' failure 2026-09-03T16:20:00Z "$(steps \
  "$(step 'Set up job' completed success 2026-09-03T16:00:00Z)" \
  "$(step 'Acceptance tests' completed failure 2026-09-03T16:19:00Z)" \
  "$(step 'Complete job' completed success 2026-09-03T16:20:00Z)")")
# Aggregator failed alongside a real step: not a cascade.
mixed=$(job '[k3s] demo-helmfile' failure 2026-09-03T16:20:00Z "$(steps \
  "$(step 'Run demo' completed failure 2026-09-03T16:19:00Z)" \
  "$(step 'Check k3s matrix result' completed failure 2026-09-03T16:19:30Z)")")
passed=$(job 'Build (linux)' success 2026-09-03T16:20:00Z "$ok_steps")

failures=0
# expect JOB_JSON EXPECTED_CLASS [ENV=VALUE...]
expect() {
  local input="$1" want="$2" got
  shift 2
  got=$(jq -cn --argjson j "$input" '{jobs: [$j]}' | env "$@" "$classifier" | cut -f3)
  if [ "$got" = "$want" ]; then
    printf 'PASS  %-28s %s %s\n' "$want" "$(jq -r '.name' <<<"$input")" "$*"
  else
    printf 'FAIL  expected %s, got %q for %s %s\n' "$want" "$got" "$(jq -r '.name' <<<"$input")" "$*"
    failures=$((failures + 1))
  fi
}

expect "$stuck" runner-stuck-after-complete
expect "$quick_cancel" superseded
expect "$lost" runner-lost
expect "$cascade" check-cascade
expect "$superseded" superseded
expect "$never_started" superseded
expect "$real" real-failure
expect "$mixed" real-failure
# The gap threshold is configurable (quick_cancel's gap is 4 s).
expect "$quick_cancel" superseded STUCK_GAP_SECONDS=10
expect "$quick_cancel" runner-stuck-after-complete STUCK_GAP_SECONDS=3

# Successful jobs are omitted; paginated input (two concatenated pages) flattens.
got=$(printf '{"jobs":[%s,%s]}\n{"jobs":[%s]}\n' "$passed" "$stuck" "$real" | "$classifier")
want=$(printf 'Build (windows)\tcancelled\trunner-stuck-after-complete\nAcceptance Tests (linux, shard 4/10)\tfailure\treal-failure')
if [ "$got" = "$want" ]; then
  echo "PASS  paginated input, success omitted"
else
  printf 'FAIL  paginated input:\n%s\n' "$got"
  failures=$((failures + 1))
fi

if [ "$failures" -ne 0 ]; then
  echo "$failures assertion(s) failed"
  exit 1
fi
echo "all assertions passed"
