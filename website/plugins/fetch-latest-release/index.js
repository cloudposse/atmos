const fetch = require('node-fetch');
//require('dotenv').config();

async function fetchLatestRelease() {
  const response = await fetch(`https://api.github.com/repos/cloudposse/atmos/releases/latest`, {
    headers: {
//      'Authorization': `token ${process.env.GITHUB_TOKEN}`,
      'Accept': 'application/vnd.github.v3+json'
    }
  });

  if (!response.ok) {
    throw new Error(`GitHub API responded with ${response.status}`);
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
