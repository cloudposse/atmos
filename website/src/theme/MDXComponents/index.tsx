import React from 'react';
import MDXComponentsOriginal from '@theme-original/MDXComponents';

export default {
  ...MDXComponentsOriginal,
  table: ({children, ...props}) => {
    const tableHeadings = children[0].props.children.props.children;

    const hasTheadValue = !Array.isArray(tableHeadings) || tableHeadings.every(({props}) => props.children);

    return (
      <div className="table-wrapper">
        <table {...props} children={hasTheadValue ? children : children.slice(1)}/>
      </div>
    );
  },
};
