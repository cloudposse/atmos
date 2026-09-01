const SCORECARD_URL = 'https://api.scorecard.dev/projects/github.com/cloudposse/atmos';
const BEST_PRACTICES_URL = 'https://www.bestpractices.dev/projects/14393.json';
const FETCH_TIMEOUT_MS = 10000;

async function fetchJson(url, label) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

  try {
    const response = await fetch(url, { signal: controller.signal });

    if (!response.ok) {
      console.warn(`[fetch-security-posture] ${label} responded with ${response.status}`);
      return null;
    }

    return await response.json();
  } catch (error) {
    console.warn(`[fetch-security-posture] Failed to fetch ${label}: ${error.message}`);
    return null;
  } finally {
    clearTimeout(timeout);
  }
}

async function fetchScorecard() {
  return fetchJson(SCORECARD_URL, 'OpenSSF Scorecard');
}

async function fetchBestPractices() {
  return fetchJson(BEST_PRACTICES_URL, 'OpenSSF Best Practices badge');
}

// A 2xx response with an unexpected shape (e.g. `{"error":"unavailable"}`) must not be
// treated as valid Scorecard data -- security.tsx calls score.toFixed()/checks.length
// on it unconditionally once it's non-null.
function isValidScorecard(data) {
  return !!data && typeof data.score === 'number' && Array.isArray(data.checks);
}

module.exports = function(context, options) {
  return {
    name: 'fetch-security-posture',
    async loadContent() {
      const [scorecard, bestPractices] = await Promise.all([
        fetchScorecard(),
        fetchBestPractices(),
      ]);

      return {
        scorecard: isValidScorecard(scorecard) ? scorecard : null,
        bestPractices,
        fetchedAt: new Date().toISOString(),
      };
    },
    async contentLoaded({ content, actions }) {
      const { setGlobalData } = actions;
      setGlobalData(content);
    }
  };
};
