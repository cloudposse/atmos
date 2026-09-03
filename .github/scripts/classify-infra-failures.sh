#!/usr/bin/env bash
# Classify every non-success job of a workflow-run attempt as an infrastructure
# failure or a real one.
#
# Input (stdin): the JSON returned by
#   gh api "repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{n}/jobs?per_page=100" --paginate
# i.e. one or more `{"jobs":[...]}` pages concatenated, or a bare `[...]` array of jobs.
#
# Output (stdout): TSV `job<TAB>conclusion<TAB>class`, one line per job whose conclusion
# is not success/skipped/null. Classes:
#
#   runner-stuck-after-complete  cancelled, every step succeeded (or was skipped), and the
#                                job-level completion landed >= STUCK_GAP_SECONDS after the
#                                last step. The runner finished all its work but never
#                                reported completion, so GitHub reaped it (see
#                                docs/fixes/2026-09-03-rerun-infra-cancelled-ci-runs.md).
#   runner-lost                  failure with no failed step: the runner disappeared.
#   check-cascade                failure whose only failed steps are `Check ... result`
#                                aggregator steps (test.yml `test-required`, k3s and
#                                terraform-registry-cache verdict jobs). Those steps only
#                                inspect other jobs' conclusions and never run tests, so
#                                they fail as a consequence of a zombie upstream job.
#                                Coupled to test.yml's `name: Check <x> result` convention.
#   superseded                   cancelled for any other reason (a step was interrupted, or
#                                the job never started) - a newer run replaced it, or a
#                                human cancelled it.
#   real-failure                 anything else: a step genuinely failed.
#
# Only runner-stuck-after-complete and runner-lost justify a rerun; check-cascade is
# tolerated alongside them; superseded and real-failure veto a rerun.
set -euo pipefail

STUCK_GAP_SECONDS="${STUCK_GAP_SECONDS:-300}"

jq -r -s --argjson gap "$STUCK_GAP_SECONDS" '
  # GitHub timestamps may carry fractional seconds, which `fromdate` rejects.
  def ts: sub("\\.[0-9]+Z$"; "Z") | fromdate;

  # Flatten paginated `{"jobs":[...]}` pages and bare arrays into one job list.
  def all_jobs: [ .[] | if type == "object" then .jobs[] else .[] end ];

  def step_ok: .status == "completed" and (.conclusion == "success" or .conclusion == "skipped");

  def gap_after_last_step:
    (.steps // []) as $s
    | if ($s | length) == 0 or .completed_at == null or ($s | last | .completed_at) == null then null
      else (.completed_at | ts) - ($s | last | .completed_at | ts) end;

  def classify:
    .conclusion as $c
    | (.steps // []) as $s
    | ($s | length > 0 and all(step_ok)) as $all_steps_ok
    | ($s | map(select(.conclusion == "failure"))) as $failed_steps
    | if $c == "cancelled" and $all_steps_ok and (gap_after_last_step // -1) >= $gap then "runner-stuck-after-complete"
      elif $c == "cancelled" then "superseded"
      elif $c == "failure" and ($failed_steps | length) == 0 then "runner-lost"
      elif $c == "failure" and ($failed_steps | all(.name | test("^Check .* result$"))) then "check-cascade"
      else "real-failure"
      end;

  all_jobs[]
  | select(.conclusion != null and .conclusion != "success" and .conclusion != "skipped")
  | [.name, .conclusion, classify]
  | @tsv
'
