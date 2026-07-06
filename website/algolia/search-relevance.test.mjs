import assert from "node:assert/strict";
import test from "node:test";

const LIVE_RELEVANCE_ENABLED = ["1", "true", "yes"].includes(
  String(process.env.ALGOLIA_LIVE_RELEVANCE_TESTS).toLowerCase(),
);

const APP_ID = process.env.ALGOLIA_APP_ID || "32YOERUX83";
const INDEX_NAME = process.env.ALGOLIA_INDEX_NAME || "atmos.tools";
const SEARCH_API_KEY = process.env.ALGOLIA_SEARCH_API_KEY || "";

if (LIVE_RELEVANCE_ENABLED && !SEARCH_API_KEY) {
  throw new Error(
    "ALGOLIA_SEARCH_API_KEY is required when ALGOLIA_LIVE_RELEVANCE_TESTS is enabled",
  );
}

function canonicalPath(hit) {
  const url = hit.url_without_anchor || hit.url;

  return new URL(url).pathname.replace(/\/+$/, "") || "/";
}

function canonicalUrlWithAnchor(hit) {
  const url = new URL(hit.url);
  const path = url.pathname.replace(/\/+$/, "") || "/";

  return `${url.origin}${path}${url.hash}`;
}

async function search(query, hitsPerPage = 20) {
  const response = await fetch(
    `https://${APP_ID}-dsn.algolia.net/1/indexes/${encodeURIComponent(
      INDEX_NAME,
    )}/query`,
    {
      method: "POST",
      signal: AbortSignal.timeout(10_000),
      headers: {
        "Content-Type": "application/json",
        "X-Algolia-API-Key": SEARCH_API_KEY,
        "X-Algolia-Application-Id": APP_ID,
      },
      body: JSON.stringify({ query, hitsPerPage }),
    },
  );

  if (!response.ok) {
    throw new Error(
      `Algolia search failed with ${response.status}: ${await response.text()}`,
    );
  }

  return response.json();
}

function findRank(hits, predicate) {
  return hits.findIndex((hit) => predicate(canonicalPath(hit), hit));
}

test(
  "live Algolia ranks Atmos Auth command docs above configuration reference pages",
  { skip: !LIVE_RELEVANCE_ENABLED },
  async () => {
    const { hits } = await search("atmos auth");
    const commandRank = findRank(hits, (path) =>
      path.startsWith("/cli/commands/auth"),
    );
    const configRank = findRank(hits, (path) =>
      path.startsWith("/cli/configuration/auth"),
    );

    assert.notEqual(
      commandRank,
      -1,
      `Expected an Atmos Auth command page in top results. Got:\n${hits
        .map((hit, index) => `${index + 1}. ${hit.url}`)
        .join("\n")}`,
    );
    assert.equal(
      commandRank,
      0,
      `Expected Atmos Auth command docs to be the top hit. Got:\n${hits
        .slice(0, 10)
        .map((hit, index) => `${index + 1}. ${hit.url}`)
        .join("\n")}`,
    );
    assert.ok(
      configRank === -1 || commandRank < configRank,
      `Expected command docs before configuration reference pages. Got command rank ${
        commandRank + 1
      }, config rank ${configRank + 1}.`,
    );
  },
);

test(
  "live Algolia returns snippet content for the top atmos auth hit",
  { skip: !LIVE_RELEVANCE_ENABLED },
  async () => {
    // Regression guard for hierarchy-only hits: when "atmos auth" matches
    // the page title (`hierarchy.lvl1`), the distinct dedup must surface a
    // record with snippetable content (the page `<Intro>`), not a bare
    // `lvl1` record. Bare lvl1 records render in the UI as title-only,
    // with no subtext — which is exactly what we want to prevent here.
    const { hits } = await search("atmos auth");
    const topHit = hits[0];
    const snippet = topHit && topHit._snippetResult && topHit._snippetResult.content;
    const snippetValue = snippet && snippet.value;

    assert.ok(
      snippetValue && snippetValue.trim().length > 0,
      `Expected the top "atmos auth" hit to include a content snippet. Got top hit:\n${
        JSON.stringify(topHit, null, 2)
      }`,
    );
  },
);

test(
  "live Algolia does not return slash and non-slash duplicates for auth config",
  { skip: !LIVE_RELEVANCE_ENABLED },
  async () => {
    const { hits } = await search("atmos auth");
    const urls = hits.map(canonicalUrlWithAnchor);
    const duplicateUrls = urls.filter(
      (url, index) => urls.indexOf(url) !== index,
    );

    assert.deepEqual(
      duplicateUrls,
      [],
      `Expected canonicalized top results to be unique. Got:\n${hits
        .slice(0, 10)
        .map((hit, index) => `${index + 1}. ${hit.url}`)
        .join("\n")}`,
    );
  },
);
