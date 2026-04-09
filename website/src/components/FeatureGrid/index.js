import React from 'react';
import { MotionConfig } from 'framer-motion';
import './index.css';

const FeatureGrid = ({ children, columns }) => {
  const columnClass = columns ? `feature-grid--cols-${columns}` : '';

  return (
    <MotionConfig reducedMotion="user">
      <div className={`feature-grid ${columnClass}`}>
        {React.Children.map(children, (child, index) => {
          if (React.isValidElement(child)) {
            return React.cloneElement(child, { index });
          }
          return child;
        })}
      </div>
    </MotionConfig>
  );
};

export default FeatureGrid;
