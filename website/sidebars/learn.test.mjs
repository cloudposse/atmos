import assert from "node:assert/strict";
import test from "node:test";
import sidebars from "../sidebars.js";
import {
  prepareItems,
  filterItems,
} from "../src/components/SidebarNavigator/navigation.mjs";

test("Learn has its own overview beside its six learning sections", () => {
  assert.deepEqual(sidebars.docs[0], {
    type: "doc",
    id: "learn/index",
    label: "Overview",
  });
  assert.deepEqual(
    sidebars.docs
      .filter((item) => item.type === "category")
      .map((item) => item.label),
    [
      "Get Started",
      "Learn Atmos",
      "Your First Stack",
      "Quick Start",
      "Best Practices",
      "Troubleshoot",
    ],
  );
});

test("Migration Guides starts expanded while Task Runners remains collapsible and closed", () => {
  const before = structuredClone(sidebars.docs);
  const prepared = prepareItems(sidebars.docs, "/intro");
  const getStarted = prepared.find((item) => item.label === "Get Started");
  const migration = getStarted.items.find(
    (item) => item.label === "Migration Guides",
  );
  assert.equal(migration.collapsed, false);
  assert.equal(migration.collapsible, true);
  assert.equal(
    migration.items.find((item) => item.label === "Task Runners").collapsed,
    true,
  );
  filterItems(prepared, "migration");
  assert.equal(filterItems(prepared, ""), prepared);
  assert.deepEqual(sidebars.docs, before);
});
