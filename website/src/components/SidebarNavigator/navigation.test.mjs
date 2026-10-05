import assert from "node:assert/strict";
import test from "node:test";
import {
  filterItems,
  findSection,
  normalizePath,
  prepareItems,
} from "./navigation.mjs";

const leaf = (label, href, docId = href) => ({
  type: "link",
  label,
  href,
  docId,
});
const category = (label, href, items) => ({
  type: "category",
  label,
  href,
  items,
  collapsed: true,
});
const tree = [
  category("Workflows", "/workflows", [
    leaf("workflow", "/workflows/workflow"),
    { type: "link", label: "Steps reference", href: "/steps" },
  ]),
  category("Steps", "/steps", [
    category("Step types", "/steps/type", [
      leaf("http", "/steps/type/http"),
      leaf("shell", "/steps/type/shell"),
    ]),
    category("Step configuration", "/steps/configuration", [
      leaf("retry", "/steps/retry"),
    ]),
    category("Using steps", undefined, [
      { type: "link", label: "Workflows", href: "/workflows" },
    ]),
  ]),
  leaf("CLI", "/cli"),
];

test("canonical sections win over cross-links, including direct and anchored arrivals", () => {
  assert.equal(findSection(tree, "/steps"), 1);
  assert.equal(findSection(tree, "/steps/type/http/?source=search#retry"), 1);
  assert.equal(findSection(tree, "/workflows"), 0);
  assert.equal(findSection(tree, "/cli"), null);
  assert.equal(findSection(tree, "/missing"), null);
});

test("sidebar filtering reaches another section and keeps its ancestry", () => {
  const result = filterItems(tree, "HTTP");
  assert.equal(result.length, 1);
  assert.equal(result[0].label, "Steps");
  assert.equal(result[0].items[0].label, "Step types");
  assert.deepEqual(
    result[0].items[0].items.map((item) => item.label),
    ["http"],
  );
  assert.equal(result[0].collapsed, false);
  assert.equal(result[0].items[0].collapsed, false);
});

test("query words may match both a section and its descendant", () => {
  const result = filterItems(tree, " steps   retry ");
  assert.equal(result[0].items[0].label, "Step configuration");
  assert.equal(result[0].items[0].items[0].label, "retry");
});

test("a category match reveals all descendants without mutating saved expansion defaults", () => {
  const before = structuredClone(tree);
  const result = filterItems(tree, "Step types");
  assert.equal(result[0].items[0].items.length, 2);
  assert.deepEqual(tree, before);
});

test("page titles match when the label is a YAML key", () => {
  const config = [
    category("CLI Configuration", "/cli/configuration", [
      {
        ...category("commands", "/cli/configuration/commands", [
          leaf("command", "/cli/configuration/commands/command"),
          leaf("steps", "/cli/configuration/commands/steps"),
        ]),
        customProps: { title: "Custom Commands" },
      },
      leaf("workflows", "/cli/configuration/workflows"),
    ]),
  ];
  const result = filterItems(config, "custom co");
  assert.equal(result[0].label, "CLI Configuration");
  assert.deepEqual(
    result[0].items.map((item) => item.label),
    ["commands"],
  );
  assert.equal(result[0].items[0].collapsed, false);
  assert.deepEqual(
    result[0].items[0].items.map((item) => item.label),
    ["command", "steps"],
  );
  assert.deepEqual(filterItems(config, "custom workflows"), []);
});

test("clearing a filter returns the original tree; absent terms return no entries", () => {
  assert.equal(filterItems(tree, "  "), tree);
  assert.deepEqual(filterItems(tree, "nonexistent"), []);
});

test("prepare keeps inactive categories collapsed and does not expose unlisted docs", () => {
  const input = [
    category("Private", "/private", [
      { ...leaf("Secret", "/secret"), unlisted: true },
      { ...leaf("Current", "/current"), unlisted: true },
    ]),
  ];
  input[0].collapsible = false;
  input[0].collapsed = false;
  const before = structuredClone(input);
  const result = prepareItems(input, "/current");
  assert.equal(result[0].collapsible, true);
  assert.equal(result[0].collapsed, true);
  assert.deepEqual(
    result[0].items.map((item) => item.label),
    ["Current"],
  );
  assert.deepEqual(filterItems(result, "secret"), []);
  assert.deepEqual(input, before);
});

test("route normalization ignores query strings, fragments, and trailing slashes", () => {
  assert.equal(
    normalizePath("/steps/type/http/?a=b#example"),
    "/steps/type/http",
  );
  assert.equal(normalizePath("/"), "/");
});
