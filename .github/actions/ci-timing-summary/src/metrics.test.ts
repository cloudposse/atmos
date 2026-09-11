import assert from "node:assert/strict";
import test from "node:test";

import {
  calculateTimingSummary,
  formatDuration,
  renderComment,
  selectLatestRuns,
  type WorkflowJob,
  type WorkflowRun,
} from "./metrics";

function run(overrides: Partial<WorkflowRun> = {}): WorkflowRun {
  return {
    id: 1,
    workflowId: 10,
    name: "Tests",
    event: "pull_request",
    headSha: "abc123",
    status: "completed",
    conclusion: "success",
    createdAt: "2026-09-11T10:00:00Z",
    updatedAt: "2026-09-11T10:05:00Z",
    htmlUrl: "https://github.com/cloudposse/atmos/actions/runs/1",
    pullRequests: [{ number: 42 }],
    ...overrides,
  };
}

function job(overrides: Partial<WorkflowJob> = {}): WorkflowJob {
  return {
    id: 100,
    runId: 1,
    name: "Race tests (1)",
    status: "completed",
    conclusion: "success",
    startedAt: "2026-09-11T10:01:00Z",
    completedAt: "2026-09-11T10:04:00Z",
    htmlUrl: "https://github.com/cloudposse/atmos/actions/runs/1/job/100",
    ...overrides,
  };
}

test("selectLatestRuns keeps the newest run for each workflow", () => {
  const selected = selectLatestRuns(
    [
      run({ id: 1 }),
      run({ id: 2, createdAt: "2026-09-11T10:01:00Z" }),
      run({ id: 3, workflowId: 11, name: "CodeQL" }),
      run({ id: 4, workflowId: 12, name: "CI Timing Summary" }),
      run({ id: 5, workflowId: 13, headSha: "old-sha" }),
    ],
    "abc123",
    42,
    "CI Timing Summary",
  );

  assert.deepEqual(selected.map((item) => item.id), [3, 2]);
});

test("selectLatestRuns includes associated non-pull_request workflows", () => {
  const selected = selectLatestRuns(
    [
      run({ id: 8, workflowId: 18, event: "workflow_run", pullRequests: [{ number: 42 }] }),
      run({ id: 9, workflowId: 19, event: "push", pullRequests: [] }),
    ],
    "abc123",
    42,
    "CI Timing Summary",
  );

  assert.deepEqual(selected.map((item) => item.id), [8]);
});

test("calculateTimingSummary distinguishes elapsed and aggregate runner time", () => {
  const runs = [
    run(),
    run({
      id: 2,
      workflowId: 11,
      name: "CodeQL",
      createdAt: "2026-09-11T10:02:00Z",
      updatedAt: "2026-09-11T10:07:00Z",
    }),
  ];
  const jobs = new Map<number, WorkflowJob[]>([
    [
      1,
      [
        job(),
        job({ id: 101, name: "Race tests (2)", startedAt: "2026-09-11T10:01:30Z", completedAt: "2026-09-11T10:05:00Z" }),
      ],
    ],
    [
      2,
      [job({ id: 200, runId: 2, name: "Analyze", startedAt: "2026-09-11T10:03:00Z", completedAt: "2026-09-11T10:07:00Z" })],
    ],
  ]);

  const summary = calculateTimingSummary(runs, jobs);

  assert.equal(summary.wallClockSeconds, 7 * 60);
  assert.equal(summary.runnerSeconds, 10.5 * 60);
  assert.equal(summary.jobCount, 3);
  assert.equal(summary.jobs[0].name, "Analyze");
});

test("formatDuration and renderComment produce a sticky summary", () => {
  assert.equal(formatDuration(3723), "1h 02m 03s");
  assert.equal(formatDuration(83), "1m 23s");

  const runs = [run({ name: "Tests | Linux" })];
  const summary = calculateTimingSummary(runs, new Map([[1, [job()]]]));
  const body = renderComment("<!-- marker -->", "abc1234567890", summary);

  assert.match(body, /^<!-- marker -->/);
  assert.match(body, /PR wall-clock time/);
  assert.match(body, /Aggregate runner time/);
  assert.match(body, /Tests \\| Linux/);
  assert.match(body, /Longest jobs/);
});
