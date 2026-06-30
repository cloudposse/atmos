import { getGroupedExperimentalFeatures } from '@site/src/data/experimentalFeatures';

/**
 * Normalizes a route/href so the roadmap `docs:` values and the sidebar item
 * `href` permalinks compare equal regardless of the site's `trailingSlash`
 * setting: ensure a single leading slash and strip any trailing slash.
 */
function normalizeRoute(route: string): string {
  let normalized = route.trim();
  if (!normalized.startsWith('/')) {
    normalized = `/${normalized}`;
  }
  if (normalized.length > 1 && normalized.endsWith('/')) {
    normalized = normalized.slice(0, -1);
  }
  return normalized;
}

/**
 * The set of doc routes that belong to experimental features, derived from the
 * roadmap (`experimental: true` + a `docs:` route). With `routeBasePath: '/'`,
 * these routes are exactly the doc permalinks, so they match sidebar item hrefs.
 */
const experimentalRoutes: Set<string> = new Set(
  getGroupedExperimentalFeatures()
    .flatMap((group) => group.features)
    .map((feature) => feature.docs)
    .filter((docs): docs is string => Boolean(docs))
    .map(normalizeRoute),
);

/**
 * Returns true when the given sidebar item href points at an experimental
 * feature's doc page. Undefined hrefs (e.g. categories without a doc link)
 * are never experimental.
 */
export function isExperimentalRoute(href?: string): boolean {
  if (!href) {
    return false;
  }
  return experimentalRoutes.has(normalizeRoute(href));
}

export { experimentalRoutes };
