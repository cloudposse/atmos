import React from 'react';
import Link from '@docusaurus/Link';
import './index.css';

interface ExperimentalProps {
  feature?: string;  // Optional feature name for aria-label
}

const Experimental: React.FC<ExperimentalProps> = ({ feature }) => {
  const ariaLabel = feature
    ? `${feature} is an experimental feature`
    : 'This is an experimental feature';

  return (
    <div className="experimental-badge" role="status" aria-label={ariaLabel}>
      <span className="experimental-icon">&#x1F9EA;</span>
      <span className="experimental-text">
        <strong>Experimental</strong>
        <Link to="/community/experimental-features">Learn more</Link>
      </span>
    </div>
  );
};

export default Experimental;
