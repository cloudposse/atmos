import React from 'react';
import Link from '@docusaurus/Link';
import './index.css';

const ActionCard = ({ title = "Ready to learn this topic?", ctaText = "Read More", ctaLink, children }) => {
  return (
    <div className="action-card">
      <h2>{title}</h2>
      <p>{children}</p>
      <Link to={ctaLink} className="button button--lg button--primary">{ctaText}</Link>
    </div>
  );
};

export default ActionCard;
