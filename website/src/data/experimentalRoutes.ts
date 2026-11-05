import { getGroupedExperimentalFeatures } from "@site/src/data/experimentalFeatures";
import { normalizeFeatureRoute } from "@site/src/data/featureStatus.mjs";

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
    .map(normalizeFeatureRoute),
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
  return experimentalRoutes.has(normalizeFeatureRoute(href));
}

export { experimentalRoutes };
