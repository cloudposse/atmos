import React from 'react';
import Link from '@docusaurus/Link';
import { RiFlaskLine } from 'react-icons/ri';
import './index.css';

interface ExperimentalProps {
  feature?: string;  // Optional feature name for aria-label
}

const Experimental: React.FC<ExperimentalProps> = ({ feature }) => {
  const ariaLabel = feature
    ? `${feature} is an experimental feature`
    : 'This is an experimental feature';

  return (
    <Link to="/experimental" className="experimental-badge" role="status" aria-label={ariaLabel}>
      <RiFlaskLine className="experimental-icon" />
      <span className="experimental-text">Experimental</span>
    </Link>
  );
};

export default Experimental;
