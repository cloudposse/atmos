import React from 'react';
import type { SlideSplitProps } from './types';

export function SlideSplit({ children, ratio = '1:1', className = '' }: SlideSplitProps) {
  return (
    <div className={['slide-split', `slide-split--${ratio.replace(':', '-')}`, className].filter(Boolean).join(' ')}>
      {children}
    </div>
  );
}

export default SlideSplit;
