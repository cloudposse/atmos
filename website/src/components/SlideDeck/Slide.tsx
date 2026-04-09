import React from 'react';
import type { SlideProps } from './types';
import './Slide.css';

export function Slide({
  children,
  layout = 'content',
  background,
  className = '',
}: SlideProps) {
  const style = background ? { background } : undefined;

  return (
    <div
      className={['slide', `slide--${layout}`, className].filter(Boolean).join(' ')}
      style={style}
    >
      <div className="slide__inner">
        {children}
      </div>
    </div>
  );
}

export default Slide;
