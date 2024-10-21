import React from 'react';
import './index.css';

const KeyPoints = ({ title = "You will learn", children }) => {
  return (
    <div className="key-points">
      <h2>{title}</h2>
      <div>{children}</div>
    </div>
  );
};

export default KeyPoints;
