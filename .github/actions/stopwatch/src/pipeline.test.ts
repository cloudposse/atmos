import test from "node:test";
import assert from "node:assert/strict";
import { GitHubClient } from "./github";
import { includePipelineRuns, timingRoot, type PipelineRun } from "./pipeline";

function run(id: number, path: string, overrides: Partial<PipelineRun> = {}): PipelineRun {
  return { id, path: `.github/workflows/${path}`, event: "pull_request", head_sha: "pr-head",
    display_title: "Root", run_attempt: 1, status: "completed", conclusion: "success",
    created_at: "2026-09-17T10:00:00Z", ...overrides };
}

function client(runs: PipelineRun[]): GitHubClient {
  return new GitHubClient("test", { fetch: async input => {
    const url = String(input);
    if (/\/actions\/runs\/\d+$/.test(url)) return Response.json(runs.find(run => url.endsWith(`/${run.id}`)));
    const file = /\/workflows\/([^/]+)\/runs/.exec(url)?.[1];
    return Response.json({ workflow_runs: runs.filter(run => run.path.endsWith(`/${file}`)) });
  } });
}

test("timing follows a child back to the PR head rather than main", async () => {
  const root = run(1, "ci-build-linux.yml");
  const child = run(2, "ci-test-linux.yml", { event: "workflow_run", head_sha: "main", display_title: "CI source 1 attempt 1" });
  assert.equal((await timingRoot(client([root, child]), "/repos/o/r", child)).head_sha, "pr-head");
  root.run_attempt = 2;
  await assert.rejects(timingRoot(client([root, child]), "/repos/o/r", child), /Superseded/);
});

test("timing waits for downstream workflows and excludes the bootstrap fallback", async () => {
  const roots = [run(1, "ci-build-windows.yml"), run(2, "test.yml")];
  const result = await includePipelineRuns(client(roots), "/repos/o/r", roots);
  assert.equal(result.pending, true);
  assert.deepEqual(result.runs.map(run => run.id), [1]);
});

test("timing includes only the latest child of the current build attempt", async () => {
  const root = run(1, "ci-build-windows.yml", { run_attempt: 2 });
  const old = run(2, "ci-test-windows.yml", { event: "workflow_run", display_title: "CI source 1 attempt 1" });
  const current = run(3, "ci-test-windows.yml", { event: "workflow_run", head_sha: "main", display_title: "CI source 1 attempt 2" });
  const result = await includePipelineRuns(client([root, old, current]), "/repos/o/r", [root]);
  assert.equal(result.pending, false);
  assert.deepEqual(result.runs.map(run => run.id), [1, 3]);
  assert.equal(result.runs[1].head_sha, "pr-head");
});

test("coverage timings accept either input's completion and choose the newest upload", async () => {
  const build = run(1, "ci-build-linux.yml");
  const go = run(2, "ci-test-go.yml");
  const files = ["ci-test-linux.yml", "ci-test-floci.yml", "ci-test-k3s-linux.yml", "ci-test-integrations.yml"];
  const tests = files.map((file, i) => run(10 + i, file, { event: "workflow_run", head_sha: "main", display_title: "CI source 1 attempt 1" }));
  const earlier = run(20, "ci-coverage.yml", { event: "workflow_run", head_sha: "main", display_title: "CI source 10 attempt 1" });
  const later = run(21, "ci-coverage.yml", { event: "workflow_run", head_sha: "main", display_title: "CI source 2 attempt 1" });
  const result = await includePipelineRuns(client([build, go, ...tests, earlier, later]), "/repos/o/r", [build, go]);
  assert.equal(result.pending, false);
  assert.equal(result.runs.at(-1)?.id, 21);
  assert.equal(result.runs.at(-1)?.head_sha, "pr-head");
});

test("a newer push cannot replace the PR root on the same SHA", async () => {
  const pr = run(1, "ci-build-windows.yml");
  const push = run(5, "ci-build-windows.yml", { event: "push" });
  const child = run(3, "ci-test-windows.yml", { event: "workflow_run", display_title: "CI source 1 attempt 1" });
  const pushChild = run(6, "ci-test-windows.yml", { event: "workflow_run", display_title: "CI source 5 attempt 1" });
  const result = await includePipelineRuns(client([pr, push, child, pushChild]), "/repos/o/r", [pr, push], "pull_request");
  assert.equal(result.pending, false);
  assert.deepEqual(result.runs.map(run => run.id), [1, 3]);
  assert.equal(result.runs[1].event, "pull_request");
});
