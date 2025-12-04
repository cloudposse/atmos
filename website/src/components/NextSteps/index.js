import React from 'react';
import Link from '@docusaurus/Link';
import './index.css';

const NextSteps = ({ title = "What's Next", children }) => {
  return (
    <div className="next-steps">
      <h2>{title}</h2>
      <div className="next-steps-content">{children}</div>
    </div>
  );
};

export default NextSteps;
