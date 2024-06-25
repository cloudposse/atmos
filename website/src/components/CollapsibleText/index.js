import React, { useState } from 'react';
import './index.css';

const CollapsibleText = ({ open = "Read More", close = "Show Less", type="medium", children }) => {
  const [isExpanded, setIsExpanded] = useState(false);

  const handleToggle = () => {
    setIsExpanded(!isExpanded);
  };

  return (
    <div className="collapsible" >
      <div className={`text ${type} ${isExpanded ? 'expanded' : ''}`} onClick={handleToggle}>
        {children}
      </div>
      <button onClick={handleToggle}>
        {isExpanded ? close : open}
      </button>
    </div>
  );
};

export default CollapsibleText;
