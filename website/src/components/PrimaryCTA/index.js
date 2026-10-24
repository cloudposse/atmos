import React from 'react';
import Link from '@docusaurus/Link';
import './index.css';

const PrimaryCTA = ({ to, children }) => {
  return (
        <Link to={to} className="button button--lg button--primary lp-primary-cta">{children}</Link>
      )
};

export default PrimaryCTA;
