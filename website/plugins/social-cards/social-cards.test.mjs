import assert from "node:assert/strict";
import { createRequire } from "node:module";
import test from "node:test";

const require = createRequire(import.meta.url);
const { measureText, wrapText, cardSvg, renderCardJpeg } = require("./render-card.js");
const { Resvg } = require("@resvg/resvg-js");
const { cardForPage, isRedundantSubtitle, rewriteImageMeta } = require("./index.js");

test("wrapText uses glyph widths rather than character counts", () => {
  const narrow = "iiiiiiii";
  const wide = "WWWWWWWW";
  const width = measureText(narrow);
  assert.deepEqual(wrapText(narrow, width, 1), [narrow]);
  assert.ok(wrapText(wide, width, 1)[0].endsWith("…"));
  assert.deepEqual(wrapText("Terraform Plan and Apply Summaries", 1056, 3, { fontSize: 72 }), [
    "Terraform Plan and Apply", "Summaries",
  ]);
});

test("wrapText bounds long tokens, overflow, empty text, and escaped characters", () => {
  for (const text of ["W".repeat(200), "alpha beta gamma delta ".repeat(30), "a&b <c> ".repeat(30)]) {
    const lines = wrapText(text, 500, 3);
    assert.equal(lines.length, 3);
    assert.ok(lines.at(-1).endsWith("…"));
    assert.ok(lines.every((line) => measureText(line) <= 500));
  }
  assert.deepEqual(wrapText("", 500, 3), []);
  assert.deepEqual(wrapText("   ", 500, 3), []);
  assert.deepEqual(wrapText("Hello", 500, 0), []);
});

test("CI branding works even when the section subtitle is redundant", () => {
  const card = cardForPage('<meta property="og:title" content="Native CI | atmos">', "/ci", "atmos.tools");
  const svg = cardSvg(card);
  const logo = Buffer.from(svg.match(/base64,([^"]+)/)[1], "base64");
  assert.deepEqual([...logo.subarray(0, 8)], [137, 80, 78, 71, 13, 10, 26, 10]);
  assert.notEqual(svg, cardSvg({ ...card, brand: "atmos" }));
  assert.ok(!cardSvg({ title: "Plan Summaries", subtitle: "Atmos CI" }).includes(">Atmos CI</text>"));
});

test("long headlines and descriptions stay above the footer", () => {
  const svg = cardSvg({
    title: "Terraform Configuration and Authentication Reference for Infrastructure Deployments".repeat(3),
    subtitle: "CLI Reference",
    description: "A detailed description of infrastructure configuration and deployment workflows. ".repeat(10),
    url: `atmos.tools/${"long-path/".repeat(40)}`,
  });
  const texts = [...svg.matchAll(/<text x="72" y="([\d.]+)"[^>]*>([^<]*)<\/text>/g)];
  const footer = texts.pop();
  assert.equal(Number(footer[1]), 574);
  assert.ok(footer[2].endsWith("…"));
  assert.ok(texts.every((match) => Number(match[1]) <= 508));
  assert.ok(!svg.includes('rx="22"'));
});

test("cardForPage derives title, section label, and description", () => {
  const html =
    '<meta data-rh="true" property="og:title" content="Native CI | atmos">' +
    '<meta data-rh="true" property="og:description" content="Plans &amp; summaries">';
  assert.deepEqual(cardForPage(html, "/ci", "atmos.tools"), {
    title: "Native CI",
    brand: "ci",
    // "Atmos CI" would only restate the title, so it is dropped.
    subtitle: "",
    description: "Plans & summaries",
    url: "atmos.tools/ci",
  });
  assert.equal(cardForPage(html, "/changelog/native-ci", "atmos.tools").subtitle, "Changelog");
  assert.equal(cardForPage(html, "", "atmos.tools").url, "atmos.tools");
  assert.equal(
    cardForPage(html, "/something-else", "atmos.tools").subtitle,
    "The runtime for infrastructure",
  );
  assert.equal(cardForPage("<html></html>", "/ci", "atmos.tools"), null);
});

test("isRedundantSubtitle flags labels that restate the title", () => {
  assert.equal(isRedundantSubtitle("Native CI", "Atmos CI"), true);
  assert.equal(isRedundantSubtitle("Atmos", "Atmos"), true);
  assert.equal(isRedundantSubtitle("Terraform Plan Summaries", "Atmos CI"), false);
  assert.equal(isRedundantSubtitle("Describe Affected", "CLI Reference"), false);
});

test("rewriteImageMeta swaps og:image and twitter:image and adds dimensions", () => {
  const html =
    '<meta data-rh="true" property="og:image" content="https://atmos.tools/img/social-card.png">' +
    '<meta data-rh="true" name="twitter:image" content="https://atmos.tools/img/social-card.png">';
  const out = rewriteImageMeta(html, "https://atmos.tools/img/social/abc.jpg");
  assert.equal(out.match(/img\/social\/abc\.jpg/g).length, 2);
  assert.ok(!out.includes("social-card.png"));
  assert.ok(out.includes('property="og:image:width" content="1200"'));
  assert.equal(rewriteImageMeta(out, "https://x/y.jpg").match(/og:image:width/g).length, 1);
});

test("renderCardJpeg produces a JPEG", () => {
  const jpg = renderCardJpeg({ title: "Native CI", subtitle: "Atmos CI", description: "Hello" });
  assert.deepEqual([...jpg.subarray(0, 3)], [0xff, 0xd8, 0xff]);
  assert.ok(jpg.length > 10000, `suspiciously small: ${jpg.length}`);
});

test("the alien retains its bright base layer when embedded in a card", () => {
  for (const brand of ["atmos", "ci"]) {
    const rendered = new Resvg(cardSvg({ title: "Atmos", brand }), {
      font: { loadSystemFonts: false },
    }).render();
    // Sample the center of the forehead, away from the dark eyes and outline.
    const offset = (72 * rendered.width + 104) * 4;
    const [red, green, blue] = rendered.pixels.subarray(offset, offset + 3);
    assert.ok(red > 120 && green > 120 && blue > 120,
      `${brand} lost the alien base layer: RGB ${red}, ${green}, ${blue}`);
  }
});
