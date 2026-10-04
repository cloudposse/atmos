const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

function historicalPathFor(relativePath, mappings = []) {
  const normalized = relativePath.split(path.sep).join("/");
  const mapping = mappings.find(({ from }) =>
    normalized.startsWith(`${from}/`),
  );
  return mapping
    ? `${mapping.to}${normalized.slice(mapping.from.length)}`
    : null;
}

/** Compare a moved file with its previous location at a release, without
 * depending on rename detection or adding the new file to the Git index. */
function changedLinesAtHistoricalPath(
  repoRoot,
  tag,
  historicalPath,
  currentFile,
) {
  let previous;
  try {
    previous = execFileSync("git", ["show", `${tag}:${historicalPath}`], {
      cwd: repoRoot,
      encoding: "utf8",
      stdio: ["pipe", "pipe", "pipe"],
    });
  } catch {
    return null;
  }
  const temporary = fs.mkdtempSync(
    path.join(os.tmpdir(), "atmos-doc-history-"),
  );
  try {
    const baseline = path.join(temporary, "previous.mdx");
    fs.writeFileSync(baseline, previous);
    let stat;
    try {
      stat = execFileSync(
        "git",
        ["diff", "--no-index", "--numstat", "--", baseline, currentFile],
        {
          cwd: repoRoot,
          encoding: "utf8",
          stdio: ["pipe", "pipe", "pipe"],
        },
      );
    } catch (error) {
      // git diff exits 1 when files differ, 0 when they are identical.
      if (error.status !== 1) return null;
      stat = error.stdout;
    }
    if (!stat.trim()) return 0;
    const match = stat.match(/^(\d+)\t(\d+)\t/);
    return match ? Number(match[1]) + Number(match[2]) : null;
  } finally {
    fs.rmSync(temporary, { recursive: true, force: true });
  }
}

module.exports = { historicalPathFor, changedLinesAtHistoricalPath };
