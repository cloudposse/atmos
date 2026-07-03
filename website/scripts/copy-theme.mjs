import { copyFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const websiteDir = path.resolve(scriptDir, "..");

await copyFile(
  path.resolve(websiteDir, "..", "pkg", "ui", "theme", "themes.json"),
  path.resolve(websiteDir, "static", "themes.json"),
);
