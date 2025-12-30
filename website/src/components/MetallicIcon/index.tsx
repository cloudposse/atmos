import React from 'react';
import './MetallicIcon.css';

export interface MetallicIconProps {
  src: string;
  alt?: string;
  size?: number;
  className?: string;
}

export function MetallicIcon({ src, alt = '', size = 200, className = '' }: MetallicIconProps) {
  // Use CSS mask-image technique - gradient background with SVG as mask.
  // Exactly matches apps repo implementation.
  const isDecorative = !alt;
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
      role={isDecorative ? 'presentation' : 'img'}
      aria-label={isDecorative ? undefined : alt}
      aria-hidden={isDecorative ? 'true' : undefined}
    />
  );
}

export default MetallicIcon;
