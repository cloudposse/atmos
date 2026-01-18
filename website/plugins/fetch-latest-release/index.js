// Sleep helper for retry delays.
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function fetchLatestRelease() {
  const headers = {
    'Accept': 'application/vnd.github.v3+json'
  };

  // Use GitHub token to avoid rate limits.
  // In CI, GITHUB_TOKEN should always be available.
  const token = process.env.GITHUB_TOKEN || process.env.ATMOS_GITHUB_TOKEN;
  if (token) {
    headers['Authorization'] = `token ${token}`;
  }

  const isDev = process.env.NODE_ENV !== 'production';
  const maxRetries = 3;
  let lastError;

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 30000); // 30 second timeout

    try {
      const response = await fetch(`https://api.github.com/repos/cloudposse/atmos/releases/latest`, {
        headers,
        signal: controller.signal
      });

      if (!response.ok) {
        const errorMsg = token
          ? `GitHub API responded with ${response.status} (authenticated request)`
          : `GitHub API responded with ${response.status} - likely rate limited. Set GITHUB_TOKEN or ATMOS_GITHUB_TOKEN environment variable.`;
        throw new Error(errorMsg);
      }

      const release = await response.json();
      return release.tag_name;
    } catch (error) {
      lastError = error;
      clearTimeout(timeout);

      const errorCode = error.code || error.cause?.code;
      const isRetryable = errorCode === 'UND_ERR_CONNECT_TIMEOUT' ||
                          error.name === 'AbortError' ||
                          errorCode === 'ENOTFOUND' ||
                          errorCode === 'ECONNRESET';

      if (isRetryable && attempt < maxRetries) {
        const delay = Math.pow(2, attempt) * 1000; // Exponential backoff: 2s, 4s.
        console.warn(`[fetch-latest-release] Attempt ${attempt} failed: ${error.message}. Retrying in ${delay/1000}s...`);
        await sleep(delay);
        continue;
      }

      let message = `Failed to fetch latest release: ${error.message}`;

      if (errorCode === 'UND_ERR_CONNECT_TIMEOUT' || error.name === 'AbortError') {
        message += '\nThis may be a network issue. Check your internet connection.';
      } else if (errorCode === 'ENOTFOUND') {
        message += '\nDNS resolution failed. Check your network configuration.';
      } else if (!token) {
        message += '\nConsider setting GITHUB_TOKEN to avoid rate limits.';
      }

      // In dev mode, use placeholder so local development isn't blocked.
      if (isDev) {
        console.warn(`[fetch-latest-release] ${message}`);
        console.warn(`[fetch-latest-release] Using placeholder 'latest' for development.`);
        return 'latest';
      }

      // In production, log error but use 'latest' as fallback to avoid build failures.
      console.error(`[fetch-latest-release] ${message}`);
      console.warn(`[fetch-latest-release] Using fallback 'latest' to avoid build failure.`);
      return 'latest';
    } finally {
      clearTimeout(timeout);
    }
  }

  // Should not reach here, but just in case.
  console.error(`[fetch-latest-release] All ${maxRetries} attempts failed.`);
  return 'latest';
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
