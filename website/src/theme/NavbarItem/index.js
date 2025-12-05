import React, { useEffect } from 'react';
import OriginalNavBarItem from '@theme-original/NavbarItem';
import useGlobalData from '@docusaurus/useGlobalData';
import GitHubStars from './GitHubStars';

export default function NavbarItem(props) {
  // Handle custom GitHub stars navbar item type
  if (props.type === 'custom-github-stars') {
    return <GitHubStars />;
  }

  const globalData = useGlobalData();
  const latestRelease = globalData['fetch-latest-release']?.default?.latestRelease || 'v0.0.0';

  // Update the latest release link with actual version
  useEffect(() => {
    const latestReleaseLink = document.querySelector('.latest-release-link');
    if (latestReleaseLink) {
      latestReleaseLink.href = `https://github.com/cloudposse/atmos/releases/tag/${latestRelease}`;
      latestReleaseLink.innerText = `${latestRelease}`;
    }
  }, [latestRelease]);

  return <OriginalNavBarItem {...props} />;
}
