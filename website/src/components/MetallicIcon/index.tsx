import React from 'react';
import './MetallicIcon.css';

interface MetallicIconProps {
  src: string;
  alt?: string;
  size?: number;
  className?: string;
}

export function MetallicIcon({ src, alt = '', size = 200, className = '' }: MetallicIconProps) {
  return (
    <div
      className={`metallic-icon ${className}`}
      style={{ width: size, height: size }}
      role="img"
      aria-label={alt}
    >
      <img
        src={src}
        alt={alt}
        style={{
          width: '100%',
          height: '100%',
          objectFit: 'contain',
        }}
      />
    </div>
  );
}

export default MetallicIcon;
