import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./generate-social-card.mjs", import.meta.url));
const output = fileURLToPath(new URL("../static/img/social-card.png", import.meta.url));

test("generates a 1200x630 PNG social card", () => {
  execFileSync(process.execPath, [script], { stdio: "pipe" });
  const png = readFileSync(output);

  // PNG signature followed by the IHDR chunk (width/height at bytes 16 and 20).
  assert.deepEqual([...png.subarray(0, 8)], [137, 80, 78, 71, 13, 10, 26, 10]);
  assert.equal(png.readUInt32BE(16), 1200);
  assert.equal(png.readUInt32BE(20), 630);
  assert.ok(png.length > 5000, `card is suspiciously small: ${png.length} bytes`);
});
