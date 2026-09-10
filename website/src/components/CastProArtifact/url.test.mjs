import assert from "node:assert/strict";
import test from "node:test";

import { CAST_FORMATS, buildArtifactUrl, buildEmbedUrl } from "./url.mjs";

test("buildArtifactUrl defaults owner/repo to cloudposse/atmos", () => {
  const url = buildArtifactUrl({ ref: "main", path: "examples/quickstart.cast", format: "gif" });
  assert.equal(url, "https://atmos-pro.com/casts/cloudposse/atmos/main/examples/quickstart.cast.gif");
});

test("buildArtifactUrl honors an explicit owner/repo", () => {
  const url = buildArtifactUrl({
    owner: "acme",
    repo: "infra",
    ref: "main",
    path: "demo.cast",
    format: "mp4",
  });
  assert.equal(url, "https://atmos-pro.com/casts/acme/infra/main/demo.cast.mp4");
});

test("buildArtifactUrl embeds a slash-free ref as a path segment", () => {
  const url = buildArtifactUrl({ ref: "v1.2.3", path: "demo.cast", format: "svg" });
  assert.equal(url, "https://atmos-pro.com/casts/cloudposse/atmos/v1.2.3/demo.cast.svg");
});

test("buildArtifactUrl moves a slash-containing ref into the ?ref= query param", () => {
  const url = new URL(buildArtifactUrl({ ref: "feature/foo", path: "demo.cast", format: "gif" }));
  assert.equal(url.pathname, "/casts/cloudposse/atmos/demo.cast.gif");
  assert.equal(url.searchParams.get("ref"), "feature/foo");
});

test("buildArtifactUrl omits ttlSeconds/soundtrack/download when not provided", () => {
  const url = new URL(buildArtifactUrl({ ref: "main", path: "demo.cast", format: "gif" }));
  assert.equal(url.searchParams.has("ttlSeconds"), false);
  assert.equal(url.searchParams.has("soundtrack"), false);
  assert.equal(url.searchParams.has("download"), false);
});

test("buildArtifactUrl includes ttlSeconds/soundtrack/download when provided", () => {
  const url = new URL(
    buildArtifactUrl({
      ref: "main",
      path: "demo.cast",
      format: "mp4",
      ttlSeconds: 3600,
      soundtrack: "background-1",
      download: true,
    }),
  );
  assert.equal(url.searchParams.get("ttlSeconds"), "3600");
  assert.equal(url.searchParams.get("soundtrack"), "background-1");
  assert.equal(url.searchParams.get("download"), "1");
});

test("buildArtifactUrl rejects an unknown format", () => {
  assert.throws(() => buildArtifactUrl({ ref: "main", path: "demo.cast", format: "png" }), /must be one of/);
});

test("buildArtifactUrl rejects soundtrack combined with a non-mp4 format", () => {
  assert.throws(
    () => buildArtifactUrl({ ref: "main", path: "demo.cast", format: "gif", soundtrack: "background-1" }),
    /only supported for the "mp4" format/,
  );
});

test("buildArtifactUrl requires ref and path", () => {
  assert.throws(() => buildArtifactUrl({ path: "demo.cast", format: "gif" }), /"ref" is required/);
  assert.throws(() => buildArtifactUrl({ ref: "main", format: "gif" }), /"path" is required/);
});

test("CAST_FORMATS lists the four supported render formats", () => {
  assert.deepEqual(CAST_FORMATS, ["gif", "mp4", "svg", "webm"]);
});

test("buildEmbedUrl builds the suffix-less .cast HTML player URL", () => {
  const url = buildEmbedUrl({ ref: "main", path: "examples/quickstart.cast" });
  assert.equal(url, "https://atmos-pro.com/casts/cloudposse/atmos/main/examples/quickstart.cast");
});

test("buildEmbedUrl includes ttlSeconds and moves a slash-containing ref to the query param", () => {
  const url = new URL(buildEmbedUrl({ ref: "feature/foo", path: "demo.cast", ttlSeconds: 60 }));
  assert.equal(url.pathname, "/casts/cloudposse/atmos/demo.cast");
  assert.equal(url.searchParams.get("ref"), "feature/foo");
  assert.equal(url.searchParams.get("ttlSeconds"), "60");
});
