import React from 'react';
import type { SlideContentProps } from './types';

export function SlideContent({ children, className = '' }: SlideContentProps) {
  return (
    <div className={`slide-content ${className}`}>
      {children}
    </div>
  );
}

export default SlideContent;
