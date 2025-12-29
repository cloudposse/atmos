import React from 'react';
import type { SlideSubtitleProps } from './types';

export function SlideSubtitle({ children, className = '' }: SlideSubtitleProps) {
  return (
    <h2 className={`slide-subtitle ${className}`}>
      {children}
    </h2>
  );
}

export default SlideSubtitle;
