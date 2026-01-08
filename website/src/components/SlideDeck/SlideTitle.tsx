import React from 'react';
import type { SlideTitleProps } from './types';

export function SlideTitle({ children, className = '' }: SlideTitleProps) {
  return (
    <h1 className={`slide-title ${className}`}>
      {children}
    </h1>
  );
}

export default SlideTitle;
