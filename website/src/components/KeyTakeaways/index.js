import React from 'react';
import './index.css';

const KeyTakeaways = ({ title = "Key Takeaways", children }) => {
  return (
    <div className="key-takeaways">
      <h2>{title}</h2>
      <div className="key-takeaways-content">{children}</div>
    </div>
  );
};

export default KeyTakeaways;
