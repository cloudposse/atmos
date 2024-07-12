import React from 'react';
import useGlobalData from '@docusaurus/useGlobalData';
  
const LatestRelease = () => {
    const globalData = useGlobalData();
    const latestRelease = globalData['fetch-latest-release']?.default?.latestRelease || 'v0.0.0';
    return (
        <span>{latestRelease}</span>
    );
};

export default LatestRelease;
