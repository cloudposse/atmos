import React from 'react';
import type { SlideListProps } from './types';

export function SlideList({ children, ordered = false, className = '' }: SlideListProps) {
  const Tag = ordered ? 'ol' : 'ul';
  return (
    <Tag className={['slide-list', ordered && 'slide-list--ordered', className].filter(Boolean).join(' ')}>
      {children}
    </Tag>
  );
}

export default SlideList;
