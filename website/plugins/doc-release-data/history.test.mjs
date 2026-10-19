import assert from "node:assert/strict";
import test from "node:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { historicalPathFor, changedLinesAtHistoricalPath } from "./history.js";

const mappings = [
  {
    from: "website/docs/steps",
    to: "website/docs/workflows/workflows/workflow/steps",
  },
];

test("historical paths apply only to the migrated directory and retain nested paths", () => {
  assert.equal(
    historicalPathFor("website/docs/steps/type/http.mdx", mappings),
    "website/docs/workflows/workflows/workflow/steps/type/http.mdx",
  );
  assert.equal(
    historicalPathFor("website/docs/steps-other/index.mdx", mappings),
    null,
  );
  assert.equal(
    historicalPathFor("website/docs/stacks/hooks.mdx", mappings),
    null,
  );
});

test("release comparison handles untracked moves, changed content, and genuinely new pages", (t) => {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "atmos-doc-history-test-"),
  );
  t.after(() => fs.rmSync(repo, { recursive: true, force: true }));
  const git = (...args) =>
    execFileSync("git", args, { cwd: repo, stdio: "pipe" });
  git("init", "-q");
  git("config", "user.name", "Test");
  git("config", "user.email", "test@example.invalid");
  const previous = "title\nold route\nexample\n";
  fs.writeFileSync(path.join(repo, "old.mdx"), previous);
  git("add", "old.mdx");
  git("-c", "commit.gpgsign=false", "commit", "-qm", "Baseline");
  git("tag", "v1.0.0");
  const current = path.join(repo, "new.mdx");
  fs.writeFileSync(current, previous);
  assert.equal(
    changedLinesAtHistoricalPath(repo, "v1.0.0", "old.mdx", current),
    0,
  );
  fs.writeFileSync(current, previous.replace("old route", "new route"));
  assert.equal(
    changedLinesAtHistoricalPath(repo, "v1.0.0", "old.mdx", current),
    2,
  );
  fs.writeFileSync(current, "Different content\n");
  assert.equal(
    changedLinesAtHistoricalPath(repo, "v1.0.0", "old.mdx", current),
    4,
  );
  assert.equal(
    changedLinesAtHistoricalPath(repo, "v1.0.0", "never-existed.mdx", current),
    null,
  );
});
