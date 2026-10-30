import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../docs/cli/commands/", import.meta.url));

function commandPages(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) return entry.name === "_partials" ? [] : commandPages(file);
    return /\.mdx?$/.test(entry.name) ? [file] : [];
  });
}

test("every command page opens with one nonempty Intro and imports the component", () => {
  const pages = commandPages(root);
  assert.ok(pages.length > 0, "No command documentation found");
  const failures = [];

  for (const file of pages) {
    // Ignore frontmatter and code examples when checking rendered page content.
    const source = readFileSync(file, "utf8")
      .replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, "")
      .replace(/^(```|~~~)[\s\S]*?^\1[^\n]*$/gm, "");
    const name = path.relative(root, file);
    const openings = [...source.matchAll(/<Intro\b[^>]*>/g)];
    const intros = [...source.matchAll(/<Intro\b[^>]*>([\s\S]*?)<\/Intro>/g)];
    if (openings.length !== 1 || intros.length !== 1 || !intros[0][1].trim()) {
      failures.push(`${name}: expected one nonempty <Intro>...</Intro>`);
      continue;
    }
    if (!/import\s+Intro\s+from\s+["']@site\/src\/components\/Intro["']/.test(source)) {
      failures.push(`${name}: missing Intro import`);
    }
    // A page may supply its own H1 title before the introduction.
    const firstSection = source.search(/^#{2,6}\s|<CastPlayer\b/m);
    if (firstSection !== -1 && firstSection < intros[0].index) {
      failures.push(`${name}: Intro must precede sections and casts`);
    }
  }

  assert.deepEqual(failures, [], failures.join("\n"));
});
