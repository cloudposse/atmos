import assert from "node:assert/strict";
import test from "node:test";

import crawler from "./atmos-tools.crawler.js";

const {
  createAtmosToolsCrawlerConfig,
  getLvl0ForPath,
  getPageRankForPath,
  getPathname,
  normalizePathname,
} = crawler;

test("normalizes slash and non-slash paths to the same path", () => {
  assert.equal(
    normalizePathname("/cli/configuration/auth/"),
    "/cli/configuration/auth",
  );
  assert.equal(
    normalizePathname("/cli/configuration/auth"),
    "/cli/configuration/auth",
  );
  assert.equal(
    getPathname(
      "https://atmos.tools/cli/configuration/auth/#environment-variables",
    ),
    "/cli/configuration/auth",
  );
});

test("uses URL taxonomy instead of active sidebar labels", () => {
  assert.equal(getLvl0ForPath("/cli/commands/auth/usage"), "CLI Commands");
  assert.equal(
    getLvl0ForPath("/cli/configuration/auth/required"),
    "CLI Configuration",
  );
  assert.equal(
    getLvl0ForPath("/changelog/introducing-atmos-auth"),
    "Changelog",
  );
  assert.equal(getLvl0ForPath("/tutorials/ecr-authentication"), "Tutorials");
});

test("boosts command pages above configuration and reference pages", () => {
  assert.ok(
    getPageRankForPath("/cli/commands/auth/usage") >
      getPageRankForPath("/cli/configuration/auth"),
  );
  assert.ok(
    getPageRankForPath("/cli/commands/auth/login") >
      getPageRankForPath("/reference/yaml"),
  );
  assert.ok(
    getPageRankForPath("/tutorials/ecr-authentication") >
      getPageRankForPath("/cli/configuration/auth"),
  );
});

test("crawler config keeps definition terms searchable at lower hierarchy", () => {
  const config = createAtmosToolsCrawlerConfig();
  const action = config.actions[0];
  const recordProps = {};

  action.recordExtractor({
    url: "https://atmos.tools/cli/configuration/auth/",
    helpers: {
      docsearch: (options) => {
        Object.assign(recordProps, options.recordProps);
        return [];
      },
    },
  });

  assert.equal(recordProps.lvl5, "article h5");
  assert.equal(recordProps.lvl6, "article h6, article dt");
  assert.equal(
    recordProps.content,
    "article p, article li, article dd, article td",
  );
  assert.equal(recordProps.lvl0.defaultValue, "CLI Configuration");
  assert.equal(recordProps.pageRank, 35);
});

test("crawler config uses pageRank and respects canonical URLs", () => {
  const config = createAtmosToolsCrawlerConfig();
  const indexName = config.actions[0].indexName;

  assert.equal(config.ignoreCanonicalTo, false);
  assert.deepEqual(config.initialIndexSettings[indexName].customRanking, [
    "desc(weight.pageRank)",
    "desc(weight.level)",
    "asc(weight.position)",
  ]);
});
