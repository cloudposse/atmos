async function fetchLatestRelease() {
  const headers = {
    'Accept': 'application/vnd.github.v3+json'
  };

  // Use GitHub token if available to avoid rate limits
  const token = process.env.GITHUB_TOKEN || process.env.ATMOS_GITHUB_TOKEN;
  if (token) {
    headers['Authorization'] = `token ${token}`;
  }

  // Fallback version for offline/network issues.
  // NOTE: Update this value when making major releases to keep docs reasonably current.
  // This is only used when GitHub API is unreachable (rate limits, network issues, etc.).
  const fallbackVersion = 'v1.204.0';

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 10000); // 10 second timeout

  try {
    const response = await fetch(`https://api.github.com/repos/cloudposse/atmos/releases/latest`, {
      headers,
      signal: controller.signal
    });

    if (!response.ok) {
      const errorMsg = token
        ? `GitHub API responded with ${response.status} (authenticated request)`
        : `GitHub API responded with ${response.status} - likely rate limited. Set GITHUB_TOKEN or ATMOS_GITHUB_TOKEN environment variable.`;
      console.warn(`[fetch-latest-release] ${errorMsg}, using fallback version ${fallbackVersion}`);
      return fallbackVersion;
    }

    const release = await response.json();
    return release.tag_name;
  } catch (error) {
    console.warn(`[fetch-latest-release] Network error: ${error.message}, using fallback version ${fallbackVersion}`);
    return fallbackVersion;
  } finally {
    clearTimeout(timeout);
  }
}

module.exports = function(context, options) {
  return {
    name: 'fetch-latest-release',
    async loadContent() {
      const latestRelease = await fetchLatestRelease();
      return { latestRelease };
    },
    async contentLoaded({ content, actions }) {
      const { setGlobalData } = actions;
      setGlobalData(content);
    }
  };
};
