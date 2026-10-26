/**
 * Route identity ignores fragments, queries, and optional trailing slashes.
 * @param {string | undefined} href
 * @returns {string}
 */
export function normalizeFeatureRoute(href) {
  if (!href?.trim()) return "";
  const path = href.trim().split(/[?#]/)[0];
  return `/${path.replace(/^\/+|\/+$/g, "")}`;
}

// Explicit lifecycle metadata; a page mentioning “legacy” is not necessarily deprecated.
export const deprecatedRoutes = new Set(["/stacks/settings/depends_on"]);

/**
 * Deprecation takes priority if a feature also remains on the experimental roadmap.
 * @param {string | undefined} href
 * @param {Set<string>} experimentalRoutes
 * @returns {'experimental' | 'deprecated' | undefined}
 */
export function getFeatureStatus(href, experimentalRoutes = new Set()) {
  const route = normalizeFeatureRoute(href);
  if (!route) return undefined;
  if (deprecatedRoutes.has(route)) return "deprecated";
  if (experimentalRoutes.has(route)) return "experimental";
  return undefined;
}
