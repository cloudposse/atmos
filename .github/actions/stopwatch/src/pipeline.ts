import { GitHubClient } from "./github";

export interface PipelineRun {
  id: number;
  path: string;
  event: string;
  head_sha: string;
  display_title: string;
  run_attempt: number;
  status?: string | null;
  conclusion?: string | null;
  created_at: string;
  pull_requests?: Array<{ number: number }> | null;
}

const file = (run: PipelineRun): string => run.path.split("@")[0].split("/").at(-1) ?? "";
const children: Record<string, string[]> = {
  "ci-build-linux.yml": ["ci-test-linux.yml", "ci-test-floci.yml", "ci-test-k3s-linux.yml", "ci-test-integrations.yml"],
  "ci-build-macos.yml": ["ci-test-macos.yml", "ci-test-k3s-macos.yml"],
  "ci-build-windows.yml": ["ci-test-windows.yml"],
};

/** Resolve child and coverage events back to the PR run instead of main's SHA. */
export async function timingRoot<T extends PipelineRun>(client: GitHubClient, repo: string, run: T): Promise<T> {
  let current = run;
  for (let depth = 0; depth < 3 && current.event === "workflow_run"; depth++) {
    const match = /^CI source ([1-9]\d*) attempt ([1-9]\d*)$/.exec(current.display_title);
    if (match === null) throw new Error("Missing CI timing parent identity");
    const parent = await client.request<T>("GET", `${repo}/actions/runs/${match[1]}`);
    const allowed = children[file(parent)]?.includes(file(current)) ||
      (file(current) === "ci-coverage.yml" && ["ci-test-linux.yml", "ci-test-go.yml"].includes(file(parent)));
    if (!allowed || parent.run_attempt !== Number(match[2])) throw new Error("Superseded or unexpected CI timing parent");
    current = parent;
  }
  if (current.event === "workflow_run") throw new Error("CI timing chain too deep");
  return current;
}

/** Add exact-attempt child runs to the ordinary SHA-filtered workflow list. */
export async function includePipelineRuns<T extends PipelineRun>(
  client: GitHubClient, repo: string, runs: T[], event?: string,
): Promise<{ runs: T[]; pending: boolean }> {
  const latest = new Map<string, T>();
  for (const run of runs) {
    if (event !== undefined && run.event !== event) continue;
    if (file(run) === "test.yml") continue; // inactive bootstrap/rollback workflow
    const previous = latest.get(file(run));
    if (previous === undefined || run.id > previous.id) latest.set(file(run), run);
  }
  const result = [...latest.values()];
  let pending = false;
  async function include(parent: T, child: string): Promise<T | undefined> {
    const matches = await client.paginate<T>(
      page => `${repo}/actions/workflows/${child}/runs?event=workflow_run&created=${encodeURIComponent(">=" + parent.created_at)}&per_page=100&page=${page}`,
      data => (data as { workflow_runs: T[] }).workflow_runs,
    );
    const title = `CI source ${parent.id} attempt ${parent.run_attempt}`;
    const run = matches.filter(candidate => candidate.display_title === title && file(candidate) === child)
      .sort((a, b) => b.id - a.id)[0];
    if (run !== undefined) {
      const normalized = { ...run, head_sha: parent.head_sha, event: parent.event, pull_requests: parent.pull_requests };
      result.push(normalized);
      return normalized;
    }
    if (parent.status === "completed" && parent.conclusion === "success") pending = true;
    return undefined;
  }
  for (const [parentFile, childFiles] of Object.entries(children)) {
    const parent = latest.get(parentFile);
    if (parent === undefined) continue;
    for (const child of childFiles) await include(parent, child);
  }
  // Coverage can be triggered by either input. It is not needed after a failed
  // Linux suite; otherwise include the newest event-driven upload run.
  const linux = result.find(run => file(run) === "ci-test-linux.yml");
  const go = latest.get("ci-test-go.yml");
  if (linux?.conclusion === "success" && go !== undefined) {
    const coverage = await client.paginate<T>(
      page => `${repo}/actions/workflows/ci-coverage.yml/runs?event=workflow_run&created=${encodeURIComponent(">=" + go.created_at)}&per_page=100&page=${page}`,
      data => (data as { workflow_runs: T[] }).workflow_runs,
    );
    const titles = [linux, go].map(parent => `CI source ${parent.id} attempt ${parent.run_attempt}`);
    const newest = coverage.filter(candidate => titles.includes(candidate.display_title))
      .sort((a, b) => b.id - a.id)[0];
    if (newest === undefined) pending = true;
    else result.push({ ...newest, head_sha: linux.head_sha, event: linux.event, pull_requests: linux.pull_requests });
  }
  return { runs: result, pending };
}
