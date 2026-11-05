import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { Resvg } from "@resvg/resvg-js";

// Run only when the official lockups change. Supply an installed Arial Bold TTF:
// node scripts/freeze-social-card-logos.mjs /path/to/Arial\ Bold.ttf
// Commit the outlined SVGs so normal builds need neither Arial nor system fonts.
const fontFile = process.argv[2];
if (!fontFile) throw new Error("Supply the path to the official lockups' Arial Bold TTF.");

for (const name of ["atmos-logo-gradient", "atmos-ci-lockup-gradient"]) {
  const source = new URL(`../static/img/${name}.svg`, import.meta.url);
  // Keep the assets' base gradient stops and transforms as the static frame.
  const svg = readFileSync(source, "utf8").replace(/<animate(?:Transform)?\b[^>]*\/>/g, "");
  const root = svg.match(/<svg\b[^>]*>/)[0];
  const defs = svg.match(/<defs>[\s\S]*?<\/defs>/)[0];
  // Outline only the lettering. Serializing the entire SVG through resvg alters
  // the filtered, embedded alien image and loses its base layer on re-render.
  const outlined = svg.replace(/<text\b[^>]*>[\s\S]*?<\/text>/g, (text) => {
    const converted = new Resvg(`${root}${defs}${text}</svg>`, {
      font: { fontFiles: [fontFile], loadSystemFonts: false },
    }).toString();
    return converted
      .replace(/<defs>[\s\S]*?<\/defs>/, "")
      .replace(/^<svg\b[^>]*>/, "")
      .replace(/<\/svg>\s*$/, "")
      .trim();
  });
  if (/<text\b/.test(outlined)) throw new Error(`Unresolved text in ${name}.`);
  const output = new URL(`../plugins/social-cards/assets/${name}.svg`, import.meta.url);
  writeFileSync(output, `<!-- Static, outlined export of static/img/${name}.svg. Regenerate with scripts/freeze-social-card-logos.mjs. -->\n${outlined}\n`);
  console.log(`Exported ${fileURLToPath(output)}`);
}
