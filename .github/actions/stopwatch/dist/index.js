import { createRequire as __WEBPACK_EXTERNAL_createRequire } from "module";
var __webpack_exports__ = {};

;// CONCATENATED MODULE: external "node:crypto"
const external_node_crypto_namespaceObject = __WEBPACK_EXTERNAL_createRequire(import.meta.url)("node:crypto");
;// CONCATENATED MODULE: external "node:fs/promises"
const promises_namespaceObject = __WEBPACK_EXTERNAL_createRequire(import.meta.url)("node:fs/promises");
;// CONCATENATED MODULE: ./src/github.ts
const API_VERSION = "2022-11-28";
const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;
const DEFAULT_MAX_GET_ATTEMPTS = 3;
const MAX_RETRY_DELAY_MS = 30_000;
/** Returns whether an unsuccessful GET response is safe and useful to retry. */
function isRetryableResponse(response) {
    if (response.status >= 500 || response.status === 429) {
        return true;
    }
    return response.status === 403 && (response.headers.has("retry-after") || response.headers.get("x-ratelimit-remaining") === "0");
}
/** Calculates a bounded retry delay from GitHub headers or exponential backoff. */
function retryDelayMilliseconds(response, attempt) {
    const fallback = 1_000 * 2 ** (attempt - 1);
    const retryAfter = response?.headers.get("retry-after");
    if (retryAfter !== null && retryAfter !== undefined) {
        const seconds = Number(retryAfter);
        if (Number.isFinite(seconds)) {
            return Math.min(MAX_RETRY_DELAY_MS, Math.max(0, seconds * 1_000));
        }
        const retryAt = Date.parse(retryAfter);
        if (Number.isFinite(retryAt)) {
            return Math.min(MAX_RETRY_DELAY_MS, Math.max(0, retryAt - Date.now()));
        }
    }
    if (response?.headers.get("x-ratelimit-remaining") === "0") {
        const resetAt = Number(response.headers.get("x-ratelimit-reset"));
        if (Number.isFinite(resetAt)) {
            return Math.min(MAX_RETRY_DELAY_MS, Math.max(0, resetAt * 1_000 - Date.now()));
        }
    }
    return Math.min(MAX_RETRY_DELAY_MS, fallback);
}
/** Waits for a retry delay. */
function sleep(milliseconds) {
    return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
/** Minimal GitHub REST client with pagination and bounded retries for safe GETs. */
class GitHubClient {
    #baseUrl;
    #fetch;
    #maxGetAttempts;
    #requestTimeoutMs;
    #sleep;
    #token;
    /** Creates a client whose transport options can be replaced by unit tests. */
    constructor(token, options = {}) {
        const configuredBaseUrl = options.baseUrl ?? process.env.GITHUB_API_URL ?? "https://api.github.com";
        const baseUrl = new URL(configuredBaseUrl);
        if (baseUrl.protocol !== "https:") {
            throw new Error("GitHub API base URL must use HTTPS");
        }
        this.#token = token;
        this.#baseUrl = baseUrl.toString().replace(/\/$/, "");
        this.#fetch = options.fetch ?? fetch;
        this.#sleep = options.sleep ?? sleep;
        this.#requestTimeoutMs = options.requestTimeoutMs ?? DEFAULT_REQUEST_TIMEOUT_MS;
        this.#maxGetAttempts = options.maxGetAttempts ?? DEFAULT_MAX_GET_ATTEMPTS;
    }
    /** Sends one GitHub REST request, retrying only idempotent GET failures. */
    async request(method, path, body) {
        const maxAttempts = method === "GET" ? this.#maxGetAttempts : 1;
        for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
            let response;
            try {
                response = await this.#fetch(`${this.#baseUrl}${path}`, {
                    method,
                    headers: {
                        Accept: "application/vnd.github+json",
                        Authorization: `Bearer ${this.#token}`,
                        "Content-Type": "application/json",
                        "User-Agent": "atmos-stopwatch",
                        "X-GitHub-Api-Version": API_VERSION,
                    },
                    body: body === undefined ? undefined : JSON.stringify(body),
                    signal: AbortSignal.timeout(this.#requestTimeoutMs),
                });
            }
            catch (error) {
                if (method !== "GET" || attempt === maxAttempts) {
                    throw error;
                }
                const delay = retryDelayMilliseconds(undefined, attempt);
                console.log(`GitHub API GET ${path} failed; retrying in ${delay}ms (${attempt + 1}/${maxAttempts})`);
                await this.#sleep(delay);
                continue;
            }
            if (response.ok) {
                try {
                    return await response.json();
                }
                catch (error) {
                    if (method !== "GET" || attempt === maxAttempts) {
                        throw error;
                    }
                    const delay = retryDelayMilliseconds(undefined, attempt);
                    console.log(`GitHub API GET ${path} response body failed; retrying in ${delay}ms (${attempt + 1}/${maxAttempts})`);
                    await this.#sleep(delay);
                    continue;
                }
            }
            if (method === "GET" && attempt < maxAttempts && isRetryableResponse(response)) {
                const delay = retryDelayMilliseconds(response, attempt);
                console.log(`GitHub API GET ${path} returned ${response.status}; retrying in ${delay}ms (${attempt + 1}/${maxAttempts})`);
                await this.#sleep(delay);
                continue;
            }
            const responseBody = (await response.text()).slice(0, 2_000);
            throw new Error(`GitHub API ${method} ${path} returned ${response.status}: ${responseBody}`);
        }
        throw new Error(`GitHub API ${method} ${path} exhausted its retry attempts`);
    }
    /** Fetches every page from a GitHub REST collection. */
    async paginate(pathForPage, unwrap) {
        const results = [];
        for (let page = 1;; page += 1) {
            const response = await this.request("GET", pathForPage(page));
            const pageResults = unwrap(response);
            results.push(...pageResults);
            if (pageResults.length < 100) {
                return results;
            }
        }
    }
}

;// CONCATENATED MODULE: ./src/metrics.ts
/** Parses an ISO timestamp and rejects invalid input. */
function timestamp(value) {
    const parsed = Date.parse(value);
    if (!Number.isFinite(parsed)) {
        throw new Error(`Invalid timestamp: ${value}`);
    }
    return parsed;
}
/** Calculates rounded non-negative elapsed seconds between timestamps. */
function elapsedSeconds(start, end) {
    return Math.max(0, Math.round((timestamp(end) - timestamp(start)) / 1000));
}
/** Selects the latest relevant run for each workflow on the current PR head. */
function selectLatestRuns(runs, headSha, pullRequestNumber, excludedWorkflowName) {
    const latestByWorkflow = new Map();
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
        if (current === undefined ||
            timestamp(run.createdAt) > timestamp(current.createdAt) ||
            (run.createdAt === current.createdAt && run.id > current.id)) {
            latestByWorkflow.set(run.workflowId, run);
        }
    }
    return [...latestByWorkflow.values()].sort((left, right) => timestamp(left.createdAt) - timestamp(right.createdAt) || left.id - right.id);
}
/** Aggregates wall-clock, workflow, and runner timings from completed jobs. */
function calculateTimingSummary(runs, jobsByRun) {
    if (runs.length === 0) {
        throw new Error("Cannot calculate timings without workflow runs");
    }
    const workflows = [];
    const timedJobs = [];
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
    const latestEnd = Math.max(...runs.map((run) => {
        const jobs = jobsByRun.get(run.id) ?? [];
        return Math.max(timestamp(run.updatedAt), ...jobs.filter((job) => job.completedAt !== null).map((job) => timestamp(job.completedAt)));
    }));
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
function formatDuration(totalSeconds) {
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
function escapeCell(value) {
    return value
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll("|", "\\|")
        .replace(/[\r\n]+/g, " ");
}
/** Maps a GitHub conclusion to a compact status icon. */
function conclusionIcon(conclusion) {
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
function renderComment(marker, headSha, summary) {
    const workflowRows = summary.workflows.map(({ run, elapsedSeconds, runnerSeconds, jobCount }) => `| ${conclusionIcon(run.conclusion)} [${escapeCell(run.name)}](${run.htmlUrl}) | ${formatDuration(elapsedSeconds)} | ${formatDuration(runnerSeconds)} | ${jobCount} |`);
    const longestJobRows = summary.jobs.slice(0, 10).map((job) => `| [${escapeCell(job.name)}](${job.htmlUrl}) | ${escapeCell(job.workflowName)} | ${formatDuration(job.durationSeconds)} | ${conclusionIcon(job.conclusion)} ${escapeCell(job.conclusion ?? "unknown")} |`);
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
        lines.push("", "<details>", `<summary>Longest jobs (top ${longestJobRows.length})</summary>`, "", "| Job | Workflow | Duration | Conclusion |", "| --- | --- | ---: | --- |", ...longestJobRows, "", "</details>");
    }
    lines.push("", "_Updated automatically when a PR workflow finishes._");
    return `${lines.join("\n")}\n`;
}

;// CONCATENATED MODULE: ./src/index.ts




const COORDINATOR_WORKFLOW_NAME = "CI Timing Summary";
/** Reads a required action input from the environment. */
function getInput(name) {
    const key = `INPUT_${name.replaceAll(" ", "_").toUpperCase()}`;
    const value = process.env[key]?.trim();
    if (value === undefined || value === "") {
        throw new Error(`Input required and not supplied: ${name}`);
    }
    return value;
}
/** Parses and validates an integer action input. */
function parseIntegerInput(name, minimum, maximum = Number.MAX_SAFE_INTEGER) {
    const value = Number.parseInt(getInput(name), 10);
    if (!Number.isInteger(value) || value < minimum || value > maximum) {
        throw new Error(`${name} must be an integer between ${minimum} and ${maximum}`);
    }
    return value;
}
/** Writes a multiline-safe GitHub Actions output. */
async function setOutput(name, value) {
    const outputFile = process.env.GITHUB_OUTPUT;
    if (outputFile === undefined || outputFile === "") {
        console.log(`${name}=${value}`);
        return;
    }
    const delimiter = `ghadelimiter_${(0,external_node_crypto_namespaceObject.randomUUID)()}`;
    await (0,promises_namespaceObject.appendFile)(outputFile, `${name}<<${delimiter}\n${value}\n${delimiter}\n`, "utf8");
}
/** Records whether the action published a timing summary and why. */
async function setResult(published, reason) {
    await setOutput("published", published);
    await setOutput("reason", reason);
}
/** Converts a GitHub workflow-run response to the metrics model. */
function toWorkflowRun(data) {
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
/** Converts a GitHub workflow-job response to the metrics model. */
function toWorkflowJob(runId, data) {
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
/** Validates and unwraps a top-level paginated array. */
function unwrapArray(response) {
    if (!Array.isArray(response)) {
        throw new Error("GitHub API returned an unexpected paginated response");
    }
    return response;
}
/** Builds an unwrapping function for a paginated response property. */
function unwrapProperty(property) {
    return (response) => {
        if (typeof response !== "object" || response === null || !Array.isArray(Reflect.get(response, property))) {
            throw new Error(`GitHub API response did not contain ${property}`);
        }
        return Reflect.get(response, property);
    };
}
/** Collects completed PR workflow timings and publishes the sticky comment. */
async function run() {
    await setResult(false, "Action has not completed");
    const token = getInput("github-token");
    const workflowRunId = parseIntegerInput("workflow-run-id", 1);
    const settleSeconds = parseIntegerInput("settle-seconds", 0, 120);
    const marker = getInput("comment-marker");
    const commentAuthor = getInput("comment-author");
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
    const triggeringData = await client.request("GET", `${encodedRepo}/actions/runs/${workflowRunId}`);
    const triggeringRun = toWorkflowRun(triggeringData);
    const headSha = triggeringRun.headSha;
    let pullRequestNumber = triggeringRun.pullRequests[0]?.number;
    if (pullRequestNumber === undefined) {
        const associated = await client.paginate((page) => `${encodedRepo}/commits/${encodeURIComponent(headSha)}/pulls?per_page=100&page=${page}`, (unwrapArray));
        pullRequestNumber = associated.find((pullRequest) => pullRequest.state === "open")?.number;
    }
    if (pullRequestNumber === undefined) {
        const reason = `No open pull request is associated with ${headSha}`;
        console.log(reason);
        await setResult(false, reason);
        return;
    }
    const pullRequestPath = `${encodedRepo}/pulls/${pullRequestNumber}`;
    const pullRequest = await client.request("GET", pullRequestPath);
    if (pullRequest.state !== "open" || pullRequest.head.sha !== headSha) {
        const reason = `PR #${pullRequestNumber} no longer points at ${headSha}`;
        console.log(reason);
        await setResult(false, reason);
        return;
    }
    const workflowRunResponses = await client.paginate((page) => `${encodedRepo}/actions/runs?head_sha=${encodeURIComponent(headSha)}&per_page=100&page=${page}`, unwrapProperty("workflow_runs"));
    const selectedRuns = selectLatestRuns(workflowRunResponses.map(toWorkflowRun), headSha, pullRequestNumber, COORDINATOR_WORKFLOW_NAME);
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
    const jobsByRun = new Map();
    for (const workflowRun of selectedRuns) {
        const jobs = await client.paginate((page) => `${encodedRepo}/actions/runs/${workflowRun.id}/jobs?filter=latest&per_page=100&page=${page}`, unwrapProperty("jobs"));
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
    const currentPullRequest = await client.request("GET", pullRequestPath);
    if (currentPullRequest.state !== "open" || currentPullRequest.head.sha !== headSha) {
        const reason = `PR #${pullRequestNumber} changed while timings were being calculated`;
        console.log(reason);
        await setResult(false, reason);
        return;
    }
    const summary = calculateTimingSummary(selectedRuns, jobsByRun);
    const body = renderComment(marker, headSha, summary);
    const comments = await client.paginate((page) => `${encodedRepo}/issues/${pullRequestNumber}/comments?per_page=100&page=${page}`, (unwrapArray));
    const existingComment = comments.find((comment) => comment.user?.login === commentAuthor && comment.body?.includes(marker));
    let comment;
    if (existingComment === undefined) {
        comment = await client.request("POST", `${encodedRepo}/issues/${pullRequestNumber}/comments`, { body });
        console.log(`Created timing comment: ${comment.html_url}`);
    }
    else {
        comment = await client.request("PATCH", `${encodedRepo}/issues/comments/${existingComment.id}`, { body });
        console.log(`Updated timing comment: ${comment.html_url}`);
    }
    const summaryFile = process.env.GITHUB_STEP_SUMMARY;
    if (summaryFile !== undefined && summaryFile !== "") {
        await (0,promises_namespaceObject.appendFile)(summaryFile, body, "utf8");
    }
    await setOutput("workflow-count", summary.workflowCount);
    await setOutput("job-count", summary.jobCount);
    await setOutput("wall-clock-seconds", summary.wallClockSeconds);
    await setOutput("runner-seconds", summary.runnerSeconds);
    await setOutput("comment-url", comment.html_url);
    await setResult(true, "Published timing summary");
}
run().catch((error) => {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`::error::${message.replaceAll("%", "%25").replaceAll("\r", "%0D").replaceAll("\n", "%0A")}`);
    process.exitCode = 1;
});
