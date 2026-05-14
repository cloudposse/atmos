const ATMOS_TOOLS_INDEX_NAME = "atmos.tools";
const ATMOS_TOOLS_APP_ID = "32YOERUX83";
const ATMOS_TOOLS_START_URL = "https://atmos.tools";
const PLACEHOLDER_INDEXING_API_KEY = "PASTE_INDEXING_API_KEY_HERE";

const TAXONOMY_PREFIXES = [
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

const PAGE_RANK_PREFIXES = [
  ["/cli/commands", 80],
  ["/tutorials", 65],
  ["/changelog", 60],
  ["/quick-start", 55],
  ["/learn", 50],
  ["/intro", 50],
  ["/cli/configuration", 35],
  ["/reference", 30],
];

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

function getLvl0ForPath(pathname) {
  return findPrefixValue(TAXONOMY_PREFIXES, pathname, "Documentation");
}

function getPageRankForPath(pathname) {
  return findPrefixValue(PAGE_RANK_PREFIXES, pathname, 40);
}

function createRecordExtractor() {
  return ({ url, helpers }) => {
    const pathname = getPathname(url);

    return helpers.docsearch({
      recordProps: {
        lvl0: {
          selectors: "",
          defaultValue: getLvl0ForPath(pathname),
        },
        lvl1: ["header h1", "article > h1"],
        lvl2: "article h2",
        lvl3: "article h3",
        lvl4: "article h4",
        lvl5: "article h5",
        lvl6: "article h6, article dt",
        content: "article p, article li, article dd, article td",
        pageRank: getPageRankForPath(pathname),
      },
      aggregateContent: true,
      recordVersion: "v3",
    });
  };
}

function createIndexSettings() {
  return {
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
  };
}

function createAtmosToolsCrawlerConfig(options = {}) {
  const env = typeof process !== "undefined" && process.env ? process.env : {};
  const indexName =
    options.indexName || env.ALGOLIA_INDEX_NAME || ATMOS_TOOLS_INDEX_NAME;
  const appId = options.appId || env.ALGOLIA_APP_ID || ATMOS_TOOLS_APP_ID;

  return {
    appId,
    indexPrefix: "",
    rateLimit: 8,
    maxDepth: 10,
    maxUrls: 2000,
    schedule: "on the 12 day of the month",
    startUrls: [ATMOS_TOOLS_START_URL],
    renderJavaScript: false,
    sitemaps: [`${ATMOS_TOOLS_START_URL}/sitemap.xml`],
    ignoreCanonicalTo: false,
    discoveryPatterns: [`${ATMOS_TOOLS_START_URL}/**`],
    actions: [
      {
        indexName,
        pathsToMatch: [`${ATMOS_TOOLS_START_URL}/**`],
        recordExtractor: createRecordExtractor(),
      },
    ],
    safetyChecks: { maxFailedUrls: 30 },
    initialIndexSettings: {
      [indexName]: createIndexSettings(),
    },
    apiKey:
      options.apiKey ||
      env.ALGOLIA_CRAWLER_INDEXING_API_KEY ||
      PLACEHOLDER_INDEXING_API_KEY,
  };
}

if (typeof Crawler !== "undefined") {
  new Crawler(createAtmosToolsCrawlerConfig());
}

if (typeof module !== "undefined") {
  module.exports = {
    ATMOS_TOOLS_INDEX_NAME,
    PLACEHOLDER_INDEXING_API_KEY,
    createAtmosToolsCrawlerConfig,
    createIndexSettings,
    createRecordExtractor,
    getLvl0ForPath,
    getPageRankForPath,
    getPathname,
    normalizePathname,
  };
}
