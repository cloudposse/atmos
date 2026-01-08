import React from 'react';
import type { SlideImageProps } from './types';
import './SlideImage.css';

export function SlideImage({
  src,
  alt,
  className = '',
  width,
  height,
  metallic = false,
}: SlideImageProps) {
  const classes = ['slide-image', metallic && 'slide-image--metallic', className]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={classes}>
      <img
        src={src}
        alt={alt}
        style={{
          width: width || 'auto',
          height: height || 'auto',
          maxWidth: '100%',
          maxHeight: '100%',
          objectFit: 'contain',
        }}
      />
    </div>
  );
}

export default SlideImage;
