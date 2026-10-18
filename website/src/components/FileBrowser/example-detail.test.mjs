import assert from "node:assert/strict";
import test from "node:test";
import {
  collectExampleFiles,
  relativeExamplePath,
  resolveExampleView,
  exampleViewUrl,
  filterExampleFiles,
} from "./example-detail.mjs";

const file = (path) => ({ type: "file", name: path.split("/").at(-1), path });
const atmos = file("demo/atmos.yaml");
const readme = file("demo/README.md");
const nested = file("demo/stacks/dev #1.yaml");
const files = [nested, readme, atmos];

test("recursively includes all source files, even empty directories and binary entries", () => {
  const binary = { ...file("demo/icon.png"), content: null };
  const tree = [
    readme,
    {
      type: "directory",
      children: [
        { type: "directory", children: [nested] },
        { type: "directory", children: [] },
      ],
    },
    atmos,
    binary,
  ];
  assert.deepEqual(collectExampleFiles(tree), [readme, nested, atmos, binary]);
  assert.deepEqual(collectExampleFiles([]), []);
});

test("source paths remain relative to the example", () => {
  assert.equal(relativeExamplePath(nested, "demo"), "stacks/dev #1.yaml");
});

test("landing page selects project config regardless of tree order", () => {
  assert.deepEqual(resolveExampleView("", files, "demo"), {
    tab: "overview",
    selected: atmos,
  });
});

test("file selection round-trips reserved characters and retains campaign parameters", () => {
  const url = exampleViewUrl(
    "/examples/demo",
    "?utm_source=docs",
    "files",
    "stacks/dev #1.yaml",
  );
  assert.equal(
    url,
    "/examples/demo?utm_source=docs&tab=files&file=stacks%2Fdev+%231.yaml",
  );
  assert.deepEqual(
    resolveExampleView(url.slice(url.indexOf("?")), files, "demo"),
    { tab: "files", selected: nested },
  );
});

test("switching back to Overview removes file state but preserves unrelated parameters", () => {
  assert.equal(
    exampleViewUrl(
      "/examples/demo",
      "?tab=files&file=README.md&utm_source=docs",
      "overview",
    ),
    "/examples/demo?utm_source=docs",
  );
  assert.equal(
    exampleViewUrl("/examples/demo", "?tab=files&file=README.md", "overview"),
    "/examples/demo",
  );
});

for (const query of [
  "?tab=files&file=../other/secret",
  "?tab=files&file=%",
  "?tab=files&file=deleted.txt",
]) {
  test(`unknown files safely fall back: ${query}`, () => {
    assert.deepEqual(resolveExampleView(query, files, "demo"), {
      tab: "files",
      selected: atmos,
    });
  });
}

test("unknown tabs resolve to Overview", () => {
  assert.equal(
    resolveExampleView("?tab=invalid", files, "demo").tab,
    "overview",
  );
});

test("projects without config use their README, then first file, then empty state", () => {
  assert.equal(
    resolveExampleView("", [nested, readme], "demo").selected,
    readme,
  );
  assert.equal(resolveExampleView("", [nested], "demo").selected, nested);
  assert.deepEqual(resolveExampleView("?tab=files", [], "demo"), {
    tab: "files",
    selected: undefined,
  });
  assert.equal(
    exampleViewUrl("/examples/empty", "", "files"),
    "/examples/empty?tab=files",
  );
});

test("file search ignores case and surrounding whitespace and searches nested paths", () => {
  assert.deepEqual(filterExampleFiles(files, "demo", "  STACKS/DEV "), [
    nested,
  ]);
  assert.deepEqual(filterExampleFiles(files, "demo", "yaml"), [nested, atmos]);
  assert.deepEqual(filterExampleFiles(files, "demo", "missing"), []);
  assert.deepEqual(filterExampleFiles(files, "demo", " "), files);
  assert.deepEqual(filterExampleFiles([], "demo", "yaml"), []);
});
