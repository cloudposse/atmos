import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { Parser } from "acorn";
import Ajv2020Module from "ajv/dist/2020.js";
import * as eslintScope from "eslint-scope";

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
    "article .intro, article p, article li, article dd, article td",
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
    "asc(weight.position)",
    "desc(weight.level)",
  ]);
});

// Pre-deploy guards: catch the failure modes we hit in production
// (closure refs, ES2020+ syntax, structural drift) without any secrets.

const __dirname = dirname(fileURLToPath(import.meta.url));
const schemaPath = join(__dirname, "crawler-config.schema.json");
const crawlerConfigSchema = JSON.parse(readFileSync(schemaPath, "utf8"));

// Ajv2020 supports JSON Schema draft 2020-12.
const Ajv2020 = Ajv2020Module.default || Ajv2020Module;

function getRecordExtractorSource() {
  const config = createAtmosToolsCrawlerConfig();
  return config.actions[0].recordExtractor.toString();
}

test("crawler config matches the structural JSON schema", () => {
  const config = createAtmosToolsCrawlerConfig();

  // Replace the function with a placeholder so JSON.stringify doesn't drop it;
  // the function source is validated separately by the parse/scope tests below.
  const validatable = JSON.parse(
    JSON.stringify({
      ...config,
      actions: config.actions.map((action) => ({
        ...action,
        recordExtractor: { __function__: true },
      })),
    }),
  );

  const ajv = new Ajv2020({ allErrors: true, strict: false });
  const validate = ajv.compile(crawlerConfigSchema);
  const valid = validate(validatable);

  assert.ok(
    valid,
    `Crawler config failed JSON schema validation:\n${JSON.stringify(
      validate.errors,
      null,
      2,
    )}`,
  );
});

test("recordExtractor source parses as ECMAScript 2017", () => {
  // Algolia's Crawler linter rejects ES2020+ syntax. Parsing the serialized
  // function source at ecmaVersion: 2017 catches optional chaining (?.),
  // nullish coalescing (??), binding-less catch, and other newer constructs
  // before we attempt to deploy.
  const source = getRecordExtractorSource();
  // Wrap so the bare arrow function parses as an expression.
  const wrapped = `(${source})`;

  assert.doesNotThrow(
    () => Parser.parse(wrapped, { ecmaVersion: 2017, sourceType: "script" }),
    "recordExtractor must be ES2017-compatible (no ?., ??, catch without binding, etc.)",
  );
});

// Globals available to the recordExtractor inside Algolia's sandbox. Anything
// not in this allow-list and not declared inside the function will be flagged
// as an unresolved reference (e.g., closure refs to module-scope helpers,
// which Algolia's linter reports as no-undef).
const ALLOWED_EXTRACTOR_GLOBALS = new Set([
  // Standard built-ins
  "Array",
  "Boolean",
  "Date",
  "Error",
  "Infinity",
  "JSON",
  "Math",
  "NaN",
  "Number",
  "Object",
  "RegExp",
  "String",
  "URL",
  "URLSearchParams",
  "console",
  "undefined",
]);

test("recordExtractor references only allowed runtime globals", () => {
  const source = getRecordExtractorSource();
  const wrapped = `(${source})`;
  const ast = Parser.parse(wrapped, {
    ecmaVersion: 2017,
    sourceType: "script",
    ranges: true,
    locations: true,
  });
  const scopeManager = eslintScope.analyze(ast, {
    ecmaVersion: 2017,
    sourceType: "script",
    ignoreEval: true,
  });

  // References that escape to global scope without a matching declaration.
  const unresolved = [
    ...new Set(
      scopeManager.globalScope.through
        .map((ref) => ref.identifier.name)
        .filter((name) => !ALLOWED_EXTRACTOR_GLOBALS.has(name)),
    ),
  ].sort();

  assert.deepEqual(
    unresolved,
    [],
    `recordExtractor references undefined globals: ${unresolved.join(
      ", ",
    )}. Inline the helpers inside the extractor or add the global to the allow-list.`,
  );
});
