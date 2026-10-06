import assert from "node:assert/strict";
import test from "node:test";
import { canonicalSidebar, sidebarActivePath } from "./canonical.mjs";

const leaf = (yamlReference) => ({
  href: "/stacks/vars",
  customProps: { yamlReference },
});
test("only the canonical branch automatically expands for shared pages", () => {
  const active = "/stacks/vars/?from=nav#usage";
  assert.equal(sidebarActivePath({ items: [leaf(false)] }, active), active);
  assert.equal(
    sidebarActivePath({ items: [{ items: [leaf(true)] }] }, active),
    "",
  );
  assert.equal(sidebarActivePath(leaf(true), active), "");
  assert.equal(
    sidebarActivePath({ items: [leaf(true), leaf(false)] }, active),
    active,
  );
});
test("ordinary navigation and unrelated pages retain the original active path", () => {
  assert.equal(
    sidebarActivePath({ href: "/stacks/vars" }, "/stacks/vars"),
    "/stacks/vars",
  );
  assert.equal(sidebarActivePath(leaf(true), "/stacks/env"), "/stacks/env");
});

test("breadcrumbs ignore earlier aliases and keep canonical nesting without mutating navigation", () => {
  const tree = [{ label: "ansible", items: [leaf(true)] }, leaf(false)];
  const before = structuredClone(tree);
  const canonical = canonicalSidebar(tree);
  assert.deepEqual(canonical[0].items, []);
  assert.deepEqual(canonical[1], leaf(false));
  assert.deepEqual(tree, before);
});
