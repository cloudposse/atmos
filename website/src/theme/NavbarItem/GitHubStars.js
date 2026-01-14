import React from 'react';
import useGlobalData from '@docusaurus/useGlobalData';

export default function GitHubStars() {
  const globalData = useGlobalData();
  const formattedStars = globalData['fetch-github-stars']?.default?.formattedStars;

  if (!formattedStars) return null;

  return (
    <a
      className="github-stars-badge navbar__item"
      href="https://github.com/cloudposse/atmos"
      target="_blank"
      rel="noopener noreferrer"
      aria-label={`${formattedStars} GitHub stars`}
    >
      <span className="github-stars-icon">&#9733;</span>
      <span className="github-stars-count">{formattedStars}</span>
    </a>
  );
}
