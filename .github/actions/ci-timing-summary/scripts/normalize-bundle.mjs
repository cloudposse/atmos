import { readFile, writeFile } from "node:fs/promises";

const bundlePath = new URL("../dist/index.js", import.meta.url);
const bundle = await readFile(bundlePath, "utf8");
const normalized = `${bundle.replace(/[\t ]+$/gm, "").trimEnd()}\n`;

await writeFile(bundlePath, normalized);
