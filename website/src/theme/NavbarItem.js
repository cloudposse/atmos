import React, { useEffect } from 'react';
import OriginalNavBarItem from '@theme-original/NavbarItem';
import useGlobalData from '@docusaurus/useGlobalData';

export default function NavbarItem(props) {
  const globalData = useGlobalData();
  
  //console.log('Global Data:', JSON.stringify(globalData['fetch-latest-release']));

  const latestRelease = globalData['fetch-latest-release']?.default?.latestRelease || 'v0.0.0';

  useEffect(() => {
    const latestReleaseLink = document.querySelector('.latest-release-link');
    if (latestReleaseLink) {
      latestReleaseLink.href = `https://github.com/cloudposse/atmos/releases/tag/${latestRelease}`;
      latestReleaseLink.innerText = `${latestRelease}`;
    }
  }, [latestRelease]);

  return <OriginalNavBarItem {...props} />;
}
