import assert from "node:assert/strict";
import test from "node:test";
import { getFeatureStatus, normalizeFeatureRoute } from "./featureStatus.mjs";

test("route identity handles relative paths, trailing slashes, queries, and anchors", () => {
  for (const href of [
    "/stacks/settings/depends_on",
    "stacks/settings/depends_on/",
    " /stacks/settings/depends_on/?from=nav#migration ",
  ]) {
    assert.equal(normalizeFeatureRoute(href), "/stacks/settings/depends_on");
    assert.equal(getFeatureStatus(href), "deprecated");
  }
});

test("deprecation wins over experimental without inferring status from page names", () => {
  const experimental = new Set([
    "/experimental-feature",
    "/stacks/settings/depends_on",
  ]);
  assert.equal(
    getFeatureStatus("/stacks/settings/depends_on", experimental),
    "deprecated",
  );
  assert.equal(
    getFeatureStatus("/experimental-feature/#usage", experimental),
    "experimental",
  );
  for (const href of [
    undefined,
    "",
    " ",
    "/legacy",
    "/unlisted",
    "/stacks/settings",
  ]) {
    assert.equal(getFeatureStatus(href, experimental), undefined);
  }
});
