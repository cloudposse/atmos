import React from 'react';
import Link from '@docusaurus/Link';
import './index.css';

const JourneyCard = ({ title, to, children }) => {
  return (
    <div className="journey-card">
      <div className="journey-card__content">
        <h3>{title}</h3>
        <p>{children}</p>
      </div>
      <div className="journey-card__footer">
        <Link to={to} className="button button--primary button--block">
          Learn More
        </Link>
      </div>
    </div>
  );
};

export default JourneyCard;
