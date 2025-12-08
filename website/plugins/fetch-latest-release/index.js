async function fetchLatestRelease() {
  const headers = {
    'Accept': 'application/vnd.github.v3+json'
  };

  // Use GitHub token if available to avoid rate limits
  const token = process.env.GITHUB_TOKEN || process.env.ATMOS_GITHUB_TOKEN;
  if (token) {
    headers['Authorization'] = `token ${token}`;
  }

  const response = await fetch(`https://api.github.com/repos/cloudposse/atmos/releases/latest`, {
    headers
  });

  if (!response.ok) {
    const errorMsg = token
      ? `GitHub API responded with ${response.status} (authenticated request)`
      : `GitHub API responded with ${response.status} - likely rate limited. Set GITHUB_TOKEN or ATMOS_GITHUB_TOKEN environment variable.`;
    throw new Error(errorMsg);
  }

  const release = await response.json();
  return release.tag_name;
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
