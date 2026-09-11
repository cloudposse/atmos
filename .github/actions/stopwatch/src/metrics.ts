export interface PullRequestReference {
  number: number;
}

export interface WorkflowRun {
  id: number;
  workflowId: number;
  name: string;
  event: string;
  headSha: string;
  status: string;
  conclusion: string | null;
  createdAt: string;
  updatedAt: string;
  htmlUrl: string;
  pullRequests: PullRequestReference[];
}

export interface WorkflowJob {
  id: number;
  runId: number;
  name: string;
  status: string;
  conclusion: string | null;
  startedAt: string | null;
  completedAt: string | null;
  htmlUrl: string;
}

export interface TimedJob extends WorkflowJob {
  durationSeconds: number;
  workflowName: string;
}

export interface WorkflowTiming {
  run: WorkflowRun;
  elapsedSeconds: number;
  runnerSeconds: number;
  jobCount: number;
}

export interface TimingSummary {
  workflowCount: number;
  jobCount: number;
  wallClockSeconds: number;
  runnerSeconds: number;
  workflows: WorkflowTiming[];
  jobs: TimedJob[];
}

/** Parses an ISO timestamp and rejects invalid input. */
function timestamp(value: string): number {
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) {
    throw new Error(`Invalid timestamp: ${value}`);
  }
  return parsed;
}

/** Calculates rounded non-negative elapsed seconds between timestamps. */
function elapsedSeconds(start: string, end: string): number {
  return Math.max(0, Math.round((timestamp(end) - timestamp(start)) / 1000));
}

/** Selects the latest relevant run for each workflow on the current PR head. */
export function selectLatestRuns(
  runs: WorkflowRun[],
  headSha: string,
  pullRequestNumber: number,
  excludedWorkflowName: string,
): WorkflowRun[] {
  const latestByWorkflow = new Map<number, WorkflowRun>();

  for (const run of runs) {
    if (run.headSha !== headSha || run.name === excludedWorkflowName) {
      continue;
    }

    // Direct pull_request runs are selected by their exact head SHA. Requiring
    // the PR association too would incorrectly omit some runs because GitHub's
    // REST response can temporarily return an empty pull_requests array.
    if (run.event !== "pull_request" && !run.pullRequests.some((pr) => pr.number === pullRequestNumber)) {
      continue;
    }

    const current = latestByWorkflow.get(run.workflowId);
    if (
      current === undefined ||
      timestamp(run.createdAt) > timestamp(current.createdAt) ||
      (run.createdAt === current.createdAt && run.id > current.id)
    ) {
      latestByWorkflow.set(run.workflowId, run);
    }
  }

  return [...latestByWorkflow.values()].sort(
    (left, right) => timestamp(left.createdAt) - timestamp(right.createdAt) || left.id - right.id,
  );
}

/** Aggregates wall-clock, workflow, and runner timings from completed jobs. */
export function calculateTimingSummary(
  runs: WorkflowRun[],
  jobsByRun: ReadonlyMap<number, WorkflowJob[]>,
): TimingSummary {
  if (runs.length === 0) {
    throw new Error("Cannot calculate timings without workflow runs");
  }

  const workflows: WorkflowTiming[] = [];
  const timedJobs: TimedJob[] = [];

  for (const run of runs) {
    const jobs = jobsByRun.get(run.id) ?? [];
    let runnerSeconds = 0;
    let workflowEnd = timestamp(run.updatedAt);

    for (const job of jobs) {
      if (job.completedAt !== null) {
        workflowEnd = Math.max(workflowEnd, timestamp(job.completedAt));
      }

      if (job.startedAt === null || job.completedAt === null) {
        continue;
      }

      const durationSeconds = elapsedSeconds(job.startedAt, job.completedAt);
      runnerSeconds += durationSeconds;
      timedJobs.push({ ...job, durationSeconds, workflowName: run.name });
    }

    workflows.push({
      run,
      elapsedSeconds: Math.max(0, Math.round((workflowEnd - timestamp(run.createdAt)) / 1000)),
      runnerSeconds,
      jobCount: jobs.length,
    });
  }

  const earliestStart = Math.min(...runs.map((run) => timestamp(run.createdAt)));
  const latestEnd = Math.max(
    ...runs.map((run) => {
      const jobs = jobsByRun.get(run.id) ?? [];
      return Math.max(
        timestamp(run.updatedAt),
        ...jobs.filter((job) => job.completedAt !== null).map((job) => timestamp(job.completedAt!)),
      );
    }),
  );

  return {
    workflowCount: runs.length,
    jobCount: [...jobsByRun.values()].reduce((total, jobs) => total + jobs.length, 0),
    wallClockSeconds: Math.max(0, Math.round((latestEnd - earliestStart) / 1000)),
    runnerSeconds: workflows.reduce((total, workflow) => total + workflow.runnerSeconds, 0),
    workflows: workflows.sort((left, right) => right.elapsedSeconds - left.elapsedSeconds),
    jobs: timedJobs.sort((left, right) => right.durationSeconds - left.durationSeconds || left.name.localeCompare(right.name)),
  };
}

/** Formats a duration as compact hours, minutes, and seconds. */
export function formatDuration(totalSeconds: number): string {
  const seconds = Math.max(0, Math.round(totalSeconds));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;

  if (hours > 0) {
    return `${hours}h ${String(minutes).padStart(2, "0")}m ${String(remainder).padStart(2, "0")}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${String(remainder).padStart(2, "0")}s`;
  }
  return `${remainder}s`;
}

/** Escapes untrusted text for safe display inside a Markdown table cell. */
function escapeCell(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("|", "\\|")
    .replace(/[\r\n]+/g, " ");
}

/** Maps a GitHub conclusion to a compact status icon. */
function conclusionIcon(conclusion: string | null): string {
  switch (conclusion) {
    case "success":
      return "✅";
    case "failure":
    case "startup_failure":
      return "❌";
    case "cancelled":
      return "⏹️";
    case "skipped":
      return "⏭️";
    case "timed_out":
      return "⏱️";
    default:
      return "⚪";
  }
}

/** Renders the sticky Markdown comment for one PR head commit. */
export function renderComment(marker: string, headSha: string, summary: TimingSummary): string {
  const workflowRows = summary.workflows.map(({ run, elapsedSeconds, runnerSeconds, jobCount }) =>
    `| ${conclusionIcon(run.conclusion)} [${escapeCell(run.name)}](${run.htmlUrl}) | ${formatDuration(elapsedSeconds)} | ${formatDuration(runnerSeconds)} | ${jobCount} |`,
  );
  const longestJobRows = summary.jobs.slice(0, 10).map((job) =>
    `| [${escapeCell(job.name)}](${job.htmlUrl}) | ${escapeCell(job.workflowName)} | ${formatDuration(job.durationSeconds)} | ${conclusionIcon(job.conclusion)} ${escapeCell(job.conclusion ?? "unknown")} |`,
  );

  const lines = [
    marker,
    "## CI timing summary",
    "",
    `Latest completed GitHub Actions runs for \`${headSha.slice(0, 12)}\`.`,
    "",
    `- **PR wall-clock time:** ${formatDuration(summary.wallClockSeconds)}`,
    `- **Aggregate runner time:** ${formatDuration(summary.runnerSeconds)}`,
    `- **Included:** ${summary.workflowCount} workflows, ${summary.jobCount} jobs (including matrix jobs)`,
    "",
    "Wall-clock time spans the earliest included workflow creation through the latest completion. Aggregate runner time adds each job's execution time, so concurrent jobs are counted separately.",
    "",
    "| Workflow | Elapsed | Runner time | Jobs |",
    "| --- | ---: | ---: | ---: |",
    ...workflowRows,
  ];

  if (longestJobRows.length > 0) {
    lines.push(
      "",
      "<details>",
      `<summary>Longest jobs (top ${longestJobRows.length})</summary>`,
      "",
      "| Job | Workflow | Duration | Conclusion |",
      "| --- | --- | ---: | --- |",
      ...longestJobRows,
      "",
      "</details>",
    );
  }

  lines.push("", "_Updated automatically when a PR workflow finishes._");
  return `${lines.join("\n")}\n`;
}
