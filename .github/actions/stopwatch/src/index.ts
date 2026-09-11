import { randomUUID } from "node:crypto";
import { appendFile } from "node:fs/promises";

import {
  calculateTimingSummary,
  renderComment,
  selectLatestRuns,
  type WorkflowJob,
  type WorkflowRun,
} from "./metrics";

const COORDINATOR_WORKFLOW_NAME = "CI Timing Summary";
const API_VERSION = "2022-11-28";

interface ApiPullRequest {
  number: number;
  state: string;
  head: { sha: string };
}

interface ApiWorkflowRun {
  id: number;
  workflow_id: number;
  name?: string | null;
  event: string;
  head_sha: string;
  status?: string | null;
  conclusion?: string | null;
  created_at: string;
  updated_at: string;
  html_url: string | null;
  url: string;
  pull_requests?: Array<{ number: number }> | null;
}

interface ApiWorkflowJob {
  id: number;
  name: string;
  status: string;
  conclusion: string | null;
  started_at: string | null;
  completed_at: string | null;
  html_url: string | null;
  url: string;
}

interface ApiComment {
  id: number;
  body: string | null;
  html_url: string;
  user: { login: string } | null;
}

function getInput(name: string): string {
  const key = `INPUT_${name.replaceAll(" ", "_").toUpperCase()}`;
  const value = process.env[key]?.trim();
  if (value === undefined || value === "") {
    throw new Error(`Input required and not supplied: ${name}`);
  }
  return value;
}

function parseIntegerInput(name: string, minimum: number, maximum = Number.MAX_SAFE_INTEGER): number {
  const value = Number.parseInt(getInput(name), 10);
  if (!Number.isInteger(value) || value < minimum || value > maximum) {
    throw new Error(`${name} must be an integer between ${minimum} and ${maximum}`);
  }
  return value;
}

async function setOutput(name: string, value: string | number | boolean): Promise<void> {
  const outputFile = process.env.GITHUB_OUTPUT;
  if (outputFile === undefined || outputFile === "") {
    console.log(`${name}=${value}`);
    return;
  }

  const delimiter = `ghadelimiter_${randomUUID()}`;
  await appendFile(outputFile, `${name}<<${delimiter}\n${value}\n${delimiter}\n`, "utf8");
}

async function setResult(published: boolean, reason: string): Promise<void> {
  await setOutput("published", published);
  await setOutput("reason", reason);
}

function toWorkflowRun(data: ApiWorkflowRun): WorkflowRun {
  return {
    id: data.id,
    workflowId: data.workflow_id,
    name: data.name ?? `Workflow ${data.workflow_id}`,
    event: data.event,
    headSha: data.head_sha,
    status: data.status ?? "unknown",
    conclusion: data.conclusion ?? null,
    createdAt: data.created_at,
    updatedAt: data.updated_at,
    htmlUrl: data.html_url ?? data.url,
    pullRequests: data.pull_requests?.map(({ number }) => ({ number })) ?? [],
  };
}

function toWorkflowJob(runId: number, data: ApiWorkflowJob): WorkflowJob {
  return {
    id: data.id,
    runId,
    name: data.name,
    status: data.status,
    conclusion: data.conclusion,
    startedAt: data.started_at,
    completedAt: data.completed_at,
    htmlUrl: data.html_url ?? data.url,
  };
}

class GitHubClient {
  readonly #baseUrl: string;
  readonly #token: string;

  constructor(token: string) {
    this.#token = token;
    this.#baseUrl = (process.env.GITHUB_API_URL ?? "https://api.github.com").replace(/\/$/, "");
  }

  async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const response = await fetch(`${this.#baseUrl}${path}`, {
      method,
      headers: {
        Accept: "application/vnd.github+json",
        Authorization: `Bearer ${this.#token}`,
        "Content-Type": "application/json",
        "User-Agent": "atmos-stopwatch",
        "X-GitHub-Api-Version": API_VERSION,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });

    if (!response.ok) {
      const responseBody = (await response.text()).slice(0, 2_000);
      throw new Error(`GitHub API ${method} ${path} returned ${response.status}: ${responseBody}`);
    }

    return await response.json() as T;
  }

  async paginate<T>(pathForPage: (page: number) => string, unwrap: (response: unknown) => T[]): Promise<T[]> {
    const results: T[] = [];
    for (let page = 1; ; page += 1) {
      const response = await this.request<unknown>("GET", pathForPage(page));
      const pageResults = unwrap(response);
      results.push(...pageResults);
      if (pageResults.length < 100) {
        return results;
      }
    }
  }
}

function unwrapArray<T>(response: unknown): T[] {
  if (!Array.isArray(response)) {
    throw new Error("GitHub API returned an unexpected paginated response");
  }
  return response as T[];
}

function unwrapProperty<T>(property: string): (response: unknown) => T[] {
  return (response: unknown): T[] => {
    if (typeof response !== "object" || response === null || !Array.isArray(Reflect.get(response, property))) {
      throw new Error(`GitHub API response did not contain ${property}`);
    }
    return Reflect.get(response, property) as T[];
  };
}

async function run(): Promise<void> {
  await setResult(false, "Action has not completed");

  const token = getInput("github-token");
  const workflowRunId = parseIntegerInput("workflow-run-id", 1);
  const settleSeconds = parseIntegerInput("settle-seconds", 0, 120);
  const marker = getInput("comment-marker");
  const repository = process.env.GITHUB_REPOSITORY?.split("/");
  if (repository?.length !== 2 || repository[0] === "" || repository[1] === "") {
    throw new Error("GITHUB_REPOSITORY must contain owner/repository");
  }
  const [owner, repo] = repository;
  const encodedRepo = `/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`;
  const client = new GitHubClient(token);

  if (settleSeconds > 0) {
    console.log(`Waiting ${settleSeconds}s for sibling workflow runs to be indexed`);
    await new Promise((resolve) => setTimeout(resolve, settleSeconds * 1000));
  }

  const triggeringData = await client.request<ApiWorkflowRun>("GET", `${encodedRepo}/actions/runs/${workflowRunId}`);
  const triggeringRun = toWorkflowRun(triggeringData);
  const headSha = triggeringRun.headSha;

  let pullRequestNumber: number | undefined = triggeringRun.pullRequests[0]?.number;
  if (pullRequestNumber === undefined) {
    const associated = await client.paginate<ApiPullRequest>(
      (page) => `${encodedRepo}/commits/${encodeURIComponent(headSha)}/pulls?per_page=100&page=${page}`,
      unwrapArray<ApiPullRequest>,
    );
    pullRequestNumber = associated.find((pullRequest) => pullRequest.state === "open")?.number;
  }

  if (pullRequestNumber === undefined) {
    const reason = `No open pull request is associated with ${headSha}`;
    console.log(reason);
    await setResult(false, reason);
    return;
  }

  const pullRequestPath = `${encodedRepo}/pulls/${pullRequestNumber}`;
  const pullRequest = await client.request<ApiPullRequest>("GET", pullRequestPath);
  if (pullRequest.state !== "open" || pullRequest.head.sha !== headSha) {
    const reason = `PR #${pullRequestNumber} no longer points at ${headSha}`;
    console.log(reason);
    await setResult(false, reason);
    return;
  }

  const workflowRunResponses = await client.paginate<ApiWorkflowRun>(
    (page) => `${encodedRepo}/actions/runs?head_sha=${encodeURIComponent(headSha)}&per_page=100&page=${page}`,
    unwrapProperty<ApiWorkflowRun>("workflow_runs"),
  );
  const selectedRuns = selectLatestRuns(
    workflowRunResponses.map(toWorkflowRun),
    headSha,
    pullRequestNumber,
    COORDINATOR_WORKFLOW_NAME,
  );

  if (selectedRuns.length === 0) {
    const reason = `No PR workflow runs were found for ${headSha}`;
    console.log(reason);
    await setResult(false, reason);
    return;
  }

  const incompleteRuns = selectedRuns.filter((candidate) => candidate.status !== "completed");
  if (incompleteRuns.length > 0) {
    const reason = `Waiting for ${incompleteRuns.length} workflow(s): ${incompleteRuns.map((candidate) => candidate.name).join(", ")}`;
    console.log(reason);
    await setResult(false, reason);
    return;
  }

  const jobsByRun = new Map<number, WorkflowJob[]>();
  for (const workflowRun of selectedRuns) {
    const jobs = await client.paginate<ApiWorkflowJob>(
      (page) => `${encodedRepo}/actions/runs/${workflowRun.id}/jobs?filter=latest&per_page=100&page=${page}`,
      unwrapProperty<ApiWorkflowJob>("jobs"),
    );
    jobsByRun.set(workflowRun.id, jobs.map((jobData) => toWorkflowJob(workflowRun.id, jobData)));
  }

  const incompleteJobs = [...jobsByRun.values()].flat().filter((jobData) => jobData.status !== "completed");
  if (incompleteJobs.length > 0) {
    const reason = `Waiting for ${incompleteJobs.length} job(s): ${incompleteJobs.map((jobData) => jobData.name).join(", ")}`;
    console.log(reason);
    await setResult(false, reason);
    return;
  }

  // Re-check immediately before writing so an older coordinator cannot replace
  // the sticky comment after the PR receives a newer commit.
  const currentPullRequest = await client.request<ApiPullRequest>("GET", pullRequestPath);
  if (currentPullRequest.state !== "open" || currentPullRequest.head.sha !== headSha) {
    const reason = `PR #${pullRequestNumber} changed while timings were being calculated`;
    console.log(reason);
    await setResult(false, reason);
    return;
  }

  const summary = calculateTimingSummary(selectedRuns, jobsByRun);
  const body = renderComment(marker, headSha, summary);
  const comments = await client.paginate<ApiComment>(
    (page) => `${encodedRepo}/issues/${pullRequestNumber}/comments?per_page=100&page=${page}`,
    unwrapArray<ApiComment>,
  );
  const existingComment = comments.find(
    (comment) => comment.user?.login === "github-actions[bot]" && comment.body?.includes(marker),
  );

  let comment: ApiComment;
  if (existingComment === undefined) {
    comment = await client.request<ApiComment>("POST", `${encodedRepo}/issues/${pullRequestNumber}/comments`, { body });
    console.log(`Created timing comment: ${comment.html_url}`);
  } else {
    comment = await client.request<ApiComment>("PATCH", `${encodedRepo}/issues/comments/${existingComment.id}`, { body });
    console.log(`Updated timing comment: ${comment.html_url}`);
  }

  const summaryFile = process.env.GITHUB_STEP_SUMMARY;
  if (summaryFile !== undefined && summaryFile !== "") {
    await appendFile(summaryFile, body, "utf8");
  }
  await setOutput("workflow-count", summary.workflowCount);
  await setOutput("job-count", summary.jobCount);
  await setOutput("wall-clock-seconds", summary.wallClockSeconds);
  await setOutput("runner-seconds", summary.runnerSeconds);
  await setOutput("comment-url", comment.html_url);
  await setResult(true, "Published timing summary");
}

run().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`::error::${message.replaceAll("%", "%25").replaceAll("\r", "%0D").replaceAll("\n", "%0A")}`);
  process.exitCode = 1;
});
