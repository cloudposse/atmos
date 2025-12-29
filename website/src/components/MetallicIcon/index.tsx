import React from 'react';
import './MetallicIcon.css';

interface MetallicIconProps {
  src: string;
  alt?: string;
  size?: number;
  className?: string;
}

export function MetallicIcon({ src, alt = '', size = 200, className = '' }: MetallicIconProps) {
  // Use CSS mask-image technique - gradient background with SVG as mask
  // Exactly matches apps repo implementation
  return (
    <div
      className={`metallic-icon ${className}`}
      style={{
        width: size,
        height: size,
        WebkitMaskImage: `url(${src})`,
        WebkitMaskSize: 'contain',
        WebkitMaskRepeat: 'no-repeat',
        WebkitMaskPosition: 'center',
        maskImage: `url(${src})`,
        maskSize: 'contain',
        maskRepeat: 'no-repeat',
        maskPosition: 'center',
      }}
      role="img"
      aria-label={alt}
    />
  );
}

export default MetallicIcon;
