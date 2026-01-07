import React from 'react';
import Link from '@docusaurus/Link';
import './index.css';

interface FirstReleasedProps {
  version: string;      // e.g., "v1.200.0" or "1.200.0"
  changelog?: string;   // Optional: explicit changelog slug override
}

const FirstReleased: React.FC<FirstReleasedProps> = ({ version, changelog }) => {
  // Normalize version (ensure 'v' prefix for display)
  const displayVersion = version.startsWith('v') ? version : `v${version}`;

  // Generate changelog link (if not explicitly provided)
  const changelogPath = changelog
    ? `/changelog/${changelog}`
    : '/changelog';

  return (
    <span className="first-released-badge">
      <Link to={changelogPath} title={`View changelog for ${displayVersion}`}>
        First released in {displayVersion}
      </Link>
    </span>
  );
};

export default FirstReleased;
