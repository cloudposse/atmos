import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { run } from "./index";

for (const scenario of ["completed", "failed", "running workflow", "running job", "stale head", "no runs"]) {
  test(`composite handoff: ${scenario}`, async (t) => {
    const directory = await mkdtemp(join(tmpdir(), "stopwatch-"));
    const output = join(directory, "outputs");
    const environment: Record<string, string> = {
      GITHUB_OUTPUT: output,
      GITHUB_REPOSITORY: "cloudposse/atmos",
      GITHUB_API_URL: "https://api.example.test",
      "INPUT_GITHUB-TOKEN": "test-token",
      "INPUT_WORKFLOW-RUN-ID": "10",
      "INPUT_SETTLE-SECONDS": "0",
      "INPUT_COMMENT-MARKER": "<!-- atmos-stopwatch -->",
      "INPUT_COMMENT-AUTHOR": "github-actions[bot]",
      "INPUT_PUBLISH-COMMENT": "false",
    };
    const before = { ...process.env };
    Object.assign(process.env, environment);
    t.after(async () => {
      for (const key of Object.keys(environment)) {
        if (before[key] === undefined) delete process.env[key];
        else process.env[key] = before[key];
      }
      await rm(directory, { recursive: true, force: true });
    });
    const sha = "a".repeat(40);
    const workflow = {
      id: 10, workflow_id: 20, name: "Tests", event: "pull_request", head_sha: sha,
      status: scenario === "running workflow" ? "in_progress" : "completed",
      conclusion: scenario === "failed" ? "failure" : "success",
      created_at: "2026-10-13T10:00:00Z", updated_at: "2026-10-13T10:05:00Z",
      html_url: "https://github.com/cloudposse/atmos/actions/runs/10", pull_requests: [{ number: 42 }],
    };
    let prReads = 0;
    const paths: string[] = [];
    t.mock.method(globalThis, "fetch", async (input: string | URL | Request, init?: RequestInit) => {
      assert.equal(init?.method, "GET", "handoff must not create or update comments");
      const path = new URL(String(input)).pathname;
      paths.push(path);
      if (path.endsWith("/actions/runs/10")) return Response.json(workflow);
      if (path.endsWith("/pulls/42")) {
        prReads++;
        return Response.json({ number: 42, state: "open", head: { sha: scenario === "stale head" && prReads > 1 ? "b".repeat(40) : sha } });
      }
      if (path.endsWith("/actions/runs")) return Response.json({ workflow_runs: scenario === "no runs" ? [] : [workflow] });
      if (path.endsWith("/jobs")) return Response.json({ jobs: [{
        id: 1, name: "Tests", status: scenario === "running job" ? "in_progress" : "completed",
        conclusion: workflow.conclusion, started_at: workflow.created_at, completed_at: workflow.updated_at,
        html_url: workflow.html_url,
      }] });
      assert.fail(`Unexpected API request: ${path}`);
    });
    await run();
    const content = await readFile(output, "utf8");
    const ready = scenario === "completed" || scenario === "failed";
    assert.equal(/ready<<[^\n]+\ntrue\n/.test(content), ready);
    assert.equal(content.includes("## CI timing summary"), ready);
    assert.equal(content.includes(`head-sha<<`), ready);
    if (scenario === "running workflow") assert.ok(paths.every((path) => !path.endsWith("/jobs")));
    if (ready) assert.ok(content.includes("Completed timings handed to the quality reporter"));
  });
}
