import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import matter from "gray-matter";
import navigation from "./stacks.js";
import sidebars from "../sidebars.js";
import {
  filterItems,
  findSection,
} from "../src/components/SidebarNavigator/navigation.mjs";

const { stackConfiguration: tree } = navigation;
const howTo = sidebars.cli.find((item) => item.label === "How-To Guides");
const sharingState = howTo.items.find((item) => item.label === "Sharing State");
const remoteStateGuide = sharingState.items.find(
  (item) => item.label === "Remote State Module",
);
const at = (...labels) =>
  labels.reduce(
    (item, label) => item?.items?.find((child) => child.label === label),
    tree,
  );
const walk = (item) => [item, ...(item.items || []).flatMap(walk)];
const docsDir = fileURLToPath(new URL("../docs/stacks/", import.meta.url));
const docs = new Map(
  readdirSync(docsDir, { recursive: true })
    .filter((file) => /\.mdx?$/.test(file) && !file.includes("_partials"))
    .map((file) => {
      const { data } = matter(readFileSync(path.join(docsDir, file), "utf8"));
      const dir = path.posix.dirname(file);
      const id = path.posix.join(
        "stacks",
        dir,
        data.id || path.basename(file).replace(/\.mdx?$/, ""),
      );
      return [id, data.slug || `/${id.replace(/\/index$/, "")}`];
    }),
);
const remoteStateId = "tutorials/sharing-state/remote-state-module";
const { data: remoteState } = matter(
  readFileSync(
    path.join(docsDir, "../tutorials/sharing-state/remote-state-module.mdx"),
    "utf8",
  ),
);
docs.set(remoteStateId, remoteState.slug);

function resolve(item) {
  const id = item.id || item.link?.id;
  return {
    ...item,
    href: docs.get(id),
    docId: id,
    items: item.items?.map(resolve),
  };
}

test("component fields sit below a named instance, never beside component types", () => {
  assert.deepEqual(
    at("components").items.map((item) => item.label),
    [
      "ansible",
      "container",
      "emulator",
      "helm",
      "helmfile",
      "kubernetes",
      "packer",
      "terraform",
    ],
  );
  for (const kind of at("components").items) {
    assert.deepEqual(
      kind.items.map((item) => item.label),
      ["<name>"],
    );
    assert.ok(at("components", kind.label, "<name>", "metadata"));
  }
  assert.ok(at("components", "terraform", "<name>", "mocks"));
  assert.equal(at("mocks"), undefined);
  assert.equal(at("provision"), undefined);
  assert.equal(at("components", "helmfile", "<name>", "mocks"), undefined);
  assert.equal(at("components", "*.metadata"), undefined);
});

test("scope annotations distinguish defaults from named component instances", () => {
  assert.equal(tree.customProps.yamlScope, "Stack root");
  assert.equal(at("terraform").customProps.yamlScope, "Toolchain defaults");
  assert.equal(
    at("components", "terraform", "<name>").customProps.yamlScope,
    "Component instance",
  );
  const matches = filterItems([resolve(tree)], "terraform settings");
  assert.equal(matches[0].customProps.yamlScope, "Stack root");
  assert.equal(
    matches[0].items.find((item) => item.label === "terraform").customProps
      .yamlScope,
    "Toolchain defaults",
  );
});

test("provisioning respects toolchain and component runtime scopes", () => {
  for (const kind of ["terraform", "helm", "kubernetes"]) {
    assert.ok(at(kind, "provision", "workdir"));
    assert.ok(at("components", kind, "<name>", "provision", "workdir"));
  }
  for (const kind of ["helmfile", "packer"]) {
    assert.ok(at("components", kind, "<name>", "provision", "workdir"));
    assert.equal(at(kind, "provision"), undefined);
  }
  assert.equal(at("components", "ansible", "<name>", "provision"), undefined);
  for (const item of walk(tree).filter((item) => item.label === "provision")) {
    if (item.items.some((child) => child.label === "backend")) {
      assert.equal(
        item.items.find((child) => child.label === "backend").id,
        "stacks/components/provision/component-provision-backend",
      );
    }
  }
  assert.ok(at("terraform", "provision", "backend"));
  assert.ok(at("components", "terraform", "<name>", "provision", "backend"));
  assert.equal(at("helm", "provision", "backend"), undefined);
  assert.equal(
    at("components", "helmfile", "<name>", "provision", "backend"),
    undefined,
  );
});

test("shared fields retain canonical pages and use references for repeated leaves", () => {
  assert.equal(at("vars").type, "doc");
  assert.equal(at("components", "terraform", "<name>", "vars").type, "ref");
  assert.equal(at("terraform", "vars").id, at("vars").id);
  for (const key of ["backend", "command", "providers"])
    assert.equal(at(key), undefined);
  assert.ok(at("terraform", "backend"));
  assert.ok(at("components", "terraform", "<name>", "command"));
  const nodes = [...walk(tree), remoteStateGuide];
  const ids = new Set(
    nodes.flatMap((item) => [item.id, item.link?.id]).filter(Boolean),
  );
  for (const id of ids) assert.ok(docs.has(id), `Unknown document ${id}`);
  for (const id of docs.keys()) {
    if (!["stacks/share-data", "stacks/remote-state"].includes(id))
      assert.ok(ids.has(id), `Unreachable reference ${id}`);
  }
  for (const item of nodes.filter((item) => item.items)) {
    assert.deepEqual(
      item.items.map((child) => child.label),
      item.items.map((child) => child.label).sort((a, b) => a.localeCompare(b)),
    );
  }
});

test("filtering preserves YAML ancestry and state guides belong to How-To", () => {
  const resolved = [resolve(tree), resolve(howTo)];
  const matches = filterItems(resolved, "terraform mocks");
  assert.equal(matches[0].items[0].label, "components");
  assert.equal(matches[0].items[0].items[0].items[0].label, "<name>");
  assert.equal(findSection(resolved, "/stacks/components/mocks"), 0);
  assert.equal(
    findSection(resolved, "/stacks/sharing-state/remote-state-module"),
    1,
  );
  assert.equal(at("settings", "depends_on").label, "depends_on");
});

test("Remote State appears once under How-To without a standalone Stack Guides group", () => {
  assert.equal(sidebars.cli.some((item) => item.label === "Stack Guides"), false);
  assert.equal(
    sidebars.cli.flatMap(walk).filter((item) => item.id === remoteStateId).length,
    1,
  );
  assert.equal(remoteStateGuide.type, "doc");
  assert.equal(remoteStateGuide.id, remoteStateId);
  assert.notEqual(remoteState.sidebar_class_name, "hidden");
  assert.equal(
    resolve(remoteStateGuide).href,
    "/stacks/sharing-state/remote-state-module",
  );
});

test("custom commands represent list entries without introducing a fictitious YAML key", () => {
  const category = JSON.parse(
    readFileSync(
      new URL(
        "../docs/cli/configuration/commands/command/_category_.json",
        import.meta.url,
      ),
    ),
  );
  assert.equal(category.label, "<index>");
  assert.equal(category.link.id, "cli/configuration/commands/command/index");
});

test("supported-type cards include each component type without configuration fields", () => {
  const source = readFileSync(
    path.join(docsDir, "components/index.mdx"),
    "utf8",
  );
  const cards = source.slice(
    source.indexOf("<DocCardList"),
    source.indexOf("## Component Schema"),
  );
  const kinds = [
    ...cards.matchAll(/href: '\/stacks\/components\/([^']+)'/g),
  ].map((match) => match[1]);
  assert.deepEqual(
    kinds,
    at("components").items.map((item) => item.label),
  );
});

test("explicit references retain descriptive titles for sidebar filtering", () => {
  const result = filterItems([resolve(tree)], "output mocks");
  assert.equal(
    result[0].items[0].items[0].items[0].items[0].id,
    "stacks/components/mocks",
  );
});
