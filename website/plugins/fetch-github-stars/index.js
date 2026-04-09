async function fetchGitHubStars() {
  const headers = {
    'Accept': 'application/vnd.github.v3+json'
  };

  // Use GitHub token if available to avoid rate limits
  const token = process.env.GITHUB_TOKEN || process.env.ATMOS_GITHUB_TOKEN;
  if (token) {
    headers['Authorization'] = `token ${token}`;
  }

  const response = await fetch('https://api.github.com/repos/cloudposse/atmos', {
    headers
  });

  if (!response.ok) {
    const errorMsg = token
      ? `GitHub API responded with ${response.status} (authenticated request)`
      : `GitHub API responded with ${response.status} - likely rate limited. Set GITHUB_TOKEN or ATMOS_GITHUB_TOKEN environment variable.`;
    console.warn(`[fetch-github-stars] ${errorMsg}`);
    return null;
  }

  const repo = await response.json();
  return repo.stargazers_count;
}

function formatStarCount(count) {
  if (count === null || count === undefined) {
    return null;
  }
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1).replace(/\.0$/, '')}k`;
  }
  return count.toString();
}

module.exports = function(context, options) {
  return {
    name: 'fetch-github-stars',
    async loadContent() {
      try {
        const starCount = await fetchGitHubStars();
        const formattedStars = formatStarCount(starCount);
        return { starCount, formattedStars };
      } catch (error) {
        console.warn(`[fetch-github-stars] Failed to fetch star count: ${error.message}`);
        return { starCount: null, formattedStars: null };
      }
    },
    async contentLoaded({ content, actions }) {
      const { setGlobalData } = actions;
      setGlobalData(content);
    }
  };
};
