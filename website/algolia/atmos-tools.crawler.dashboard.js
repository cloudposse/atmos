// Paste this whole file into the Algolia Crawler editor.
// Replace PASTE_INDEXING_API_KEY_HERE with the crawler indexing key from the current config.

const ATMOS_TOOLS_INDEX_NAME = "atmos.tools";
const ATMOS_TOOLS_INDEXING_API_KEY = "PASTE_INDEXING_API_KEY_HERE";

function normalizePathname(pathname) {
  if (!pathname || pathname === "/") {
    return "/";
  }

  return pathname.replace(/\/+$/, "");
}

function getPathname(url) {
  const rawUrl = typeof url === "string" ? url : url?.href || url?.url || "";

  try {
    return normalizePathname(new URL(rawUrl).pathname);
  } catch {
    return normalizePathname(
      String(rawUrl).split("#")[0]?.split("?")[0] || "/",
    );
  }
}

function matchesPrefix(pathname, prefix) {
  return pathname === prefix || pathname.startsWith(`${prefix}/`);
}

function findPrefixValue(prefixes, pathname, fallback) {
  const normalized = normalizePathname(pathname);
  const match = prefixes.find(([prefix]) => matchesPrefix(normalized, prefix));

  return match?.[1] || fallback;
}

function createRecordExtractor() {
  return ({ url, helpers }) => {
    const pathname = getPathname(url);
    const taxonomyPrefixes = [
      ["/cli/commands", "CLI Commands"],
      ["/cli/configuration", "CLI Configuration"],
      ["/changelog", "Changelog"],
      ["/tutorials", "Tutorials"],
      ["/functions", "Functions"],
      ["/stacks", "Stacks"],
      ["/components", "Components"],
      ["/integrations", "Integrations"],
      ["/reference", "Reference"],
      ["/quick-start", "Quick Start"],
      ["/learn", "Learn"],
      ["/intro", "Learn"],
      ["/terms", "Terms"],
    ];
    const pageRankPrefixes = [
      ["/cli/commands", 80],
      ["/tutorials", 65],
      ["/changelog", 60],
      ["/quick-start", 55],
      ["/learn", 50],
      ["/intro", 50],
      ["/cli/configuration", 35],
      ["/reference", 30],
    ];

    return helpers.docsearch({
      recordProps: {
        lvl0: {
          selectors: "",
          defaultValue: findPrefixValue(
            taxonomyPrefixes,
            pathname,
            "Documentation",
          ),
        },
        lvl1: ["header h1", "article > h1"],
        lvl2: "article h2",
        lvl3: "article h3",
        lvl4: "article h4",
        lvl5: "article h5",
        lvl6: "article h6, article dt",
        content: "article p, article li, article dd, article td",
        pageRank: findPrefixValue(pageRankPrefixes, pathname, 40),
      },
      aggregateContent: true,
      recordVersion: "v3",
    });
  };
}

new Crawler({
  appId: "32YOERUX83",
  indexPrefix: "",
  rateLimit: 8,
  maxDepth: 10,
  maxUrls: 2000,
  schedule: "on the 12 day of the month",
  startUrls: ["https://atmos.tools"],
  renderJavaScript: false,
  sitemaps: ["https://atmos.tools/sitemap.xml"],
  ignoreCanonicalTo: false,
  discoveryPatterns: ["https://atmos.tools/**"],
  actions: [
    {
      indexName: ATMOS_TOOLS_INDEX_NAME,
      pathsToMatch: ["https://atmos.tools/**"],
      recordExtractor: createRecordExtractor(),
    },
  ],
  safetyChecks: { maxFailedUrls: 30 },
  initialIndexSettings: {
    [ATMOS_TOOLS_INDEX_NAME]: {
      attributesForFaceting: [
        "type",
        "lang",
        "language",
        "version",
        "docusaurus_tag",
      ],
      attributesToRetrieve: [
        "hierarchy",
        "content",
        "anchor",
        "url",
        "url_without_anchor",
        "type",
      ],
      attributesToHighlight: ["hierarchy", "content"],
      attributesToSnippet: ["content:10"],
      camelCaseAttributes: ["hierarchy", "content"],
      searchableAttributes: [
        "unordered(hierarchy.lvl0)",
        "unordered(hierarchy.lvl1)",
        "unordered(hierarchy.lvl2)",
        "unordered(hierarchy.lvl3)",
        "unordered(hierarchy.lvl4)",
        "unordered(hierarchy.lvl5)",
        "unordered(hierarchy.lvl6)",
        "content",
      ],
      distinct: true,
      attributeForDistinct: "url",
      customRanking: [
        "desc(weight.pageRank)",
        "desc(weight.level)",
        "asc(weight.position)",
      ],
      ranking: [
        "words",
        "filters",
        "typo",
        "attribute",
        "proximity",
        "exact",
        "custom",
      ],
      highlightPreTag: '<span class="algolia-docsearch-suggestion--highlight">',
      highlightPostTag: "</span>",
      minWordSizefor1Typo: 3,
      minWordSizefor2Typos: 7,
      allowTyposOnNumericTokens: false,
      minProximity: 1,
      ignorePlurals: true,
      advancedSyntax: true,
      attributeCriteriaComputedByMinProximity: true,
      removeWordsIfNoResults: "allOptional",
    },
  },
  apiKey: ATMOS_TOOLS_INDEXING_API_KEY,
});
