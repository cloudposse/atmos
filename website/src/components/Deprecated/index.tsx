import React from 'react';
import Link from '@docusaurus/Link';
import { RiAlertLine } from 'react-icons/ri';
import './index.css';

interface DeprecatedProps {
  feature?: string;  // Optional feature name for aria-label
  since?: string;    // Optional version when deprecated
}

const Deprecated: React.FC<DeprecatedProps> = ({ feature, since }) => {
  const ariaLabel = feature
    ? `${feature} is deprecated${since ? ` since version ${since}` : ''}`
    : `This feature is deprecated${since ? ` since version ${since}` : ''}`;

  return (
    <Link to="/deprecated" className="deprecated-badge" role="status" aria-label={ariaLabel}>
      <RiAlertLine className="deprecated-icon" />
      <span className="deprecated-text">
        Deprecated{since && <span className="deprecated-since"> (since {since})</span>}
      </span>
    </Link>
  );
};

export default Deprecated;
