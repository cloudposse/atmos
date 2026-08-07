const ATMOS_PRO_BASE_URL = "https://atmos-pro.com";

export const CAST_FORMATS = ["gif", "mp4", "svg", "webm"];

function encodePathSegments(path) {
  return path
    .replace(/^\/+/, "")
    .split("/")
    .map(encodeURIComponent)
    .join("/");
}

// Builds a https://atmos-pro.com/casts/{owner}/{repo}/{ref}/{path}{suffix} URL.
// A ref containing a slash (e.g. "feature/foo") can't be embedded as a path
// segment unambiguously, so it's passed via `?ref=` instead in that case.
function buildCastUrl({
  owner = "cloudposse",
  repo = "atmos",
  ref,
  path,
  suffix,
  ttlSeconds,
  soundtrack,
  download,
}) {
  if (!ref) throw new Error('"ref" is required');
  if (!path) throw new Error('"path" is required');

  const refHasSlash = ref.includes("/");
  const pathSegment = encodePathSegments(path) + suffix;
  const base = `${ATMOS_PRO_BASE_URL}/casts/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`;
  const url = new URL(
    refHasSlash ? `${base}/${pathSegment}` : `${base}/${encodeURIComponent(ref)}/${pathSegment}`,
  );

  if (refHasSlash) url.searchParams.set("ref", ref);
  if (ttlSeconds !== undefined) url.searchParams.set("ttlSeconds", String(ttlSeconds));
  if (soundtrack) url.searchParams.set("soundtrack", soundtrack);
  if (download) url.searchParams.set("download", "1");

  return url.toString();
}

// Builds the direct rendered-artifact URL (GET returns a 302 once ready, a
// 202 + Retry-After while rendering, or a 400/500 JSON error).
export function buildArtifactUrl({ format, soundtrack, ...rest } = {}) {
  if (!CAST_FORMATS.includes(format)) {
    throw new Error(`"format" must be one of ${CAST_FORMATS.join(", ")}, got "${format}"`);
  }
  if (soundtrack && format !== "mp4") {
    throw new Error('"soundtrack" is only supported for the "mp4" format');
  }
  return buildCastUrl({ ...rest, soundtrack, suffix: `.${format}` });
}

// Builds the suffix-less ".cast" URL that returns an HTML page with an
// embedded player, meant for <iframe> use.
export function buildEmbedUrl(params = {}) {
  return buildCastUrl({ ...params, suffix: "" });
}
