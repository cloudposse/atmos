import React from 'react';
import './index.css';

const PillBox = ({ children }) => {
  return (
    <div className="pill-box">
      {children}
    </div>
  );
};

export default PillBox;
