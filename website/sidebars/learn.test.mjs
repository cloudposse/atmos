import assert from "node:assert/strict";
import test from "node:test";
import sidebars from "../sidebars.js";
import {
  prepareItems,
  filterItems,
} from "../src/components/SidebarNavigator/navigation.mjs";

test("Learn exposes its overview through the heading above its eight learning sections", () => {
  assert.deepEqual(sidebars.docs[0], {
    type: "doc",
    id: "learn/index",
    label: "Learn",
    customProps: { navigationOverview: true },
  });
  assert.deepEqual(
    sidebars.docs
      .filter((item) => item.type === "category")
      .map((item) => item.label),
    [
      "Get Started",
      "Migration Guides",
      "Learn Atmos",
      "Your First Stack",
      "Quick Start",
      "Tutorials",
      "Best Practices",
      "Troubleshoot",
    ],
  );
});

test("Migration Guides and Task Runners start collapsible and closed", () => {
  const before = structuredClone(sidebars.docs);
  const prepared = prepareItems(sidebars.docs, "/intro");
  const migration = prepared.find(
    (item) => item.label === "Migration Guides",
  );
  assert.equal(migration.collapsed, true);
  assert.equal(migration.collapsible, true);
  assert.equal(
    migration.items.find((item) => item.label === "Task Runners").collapsed,
    true,
  );
  filterItems(prepared, "migration");
  assert.equal(filterItems(prepared, ""), prepared);
  assert.deepEqual(sidebars.docs, before);
});
