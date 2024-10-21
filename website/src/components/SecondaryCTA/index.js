import React from 'react';
import Link from '@docusaurus/Link';
import './index.css';

const SecondaryCTA = ({ to, children }) => {
  return (
        <Link to={to} className="button button--lg button--secondary button--outline ml20">{children}</Link>
      )
};

export default SecondaryCTA;
