import React from 'react';
import './index.css';

// Display a more subtle note than an admonition, with a title and content
const Note = ({ title = "NOTE", children }) => {
  return (
    <div className="note" >
      <strong>{title}: </strong>
      <div>{children}</div>
    </div>
  );
};

export default Note;
