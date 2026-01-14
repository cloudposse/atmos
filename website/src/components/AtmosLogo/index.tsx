import React from 'react';
import './AtmosLogo.css';

interface AtmosLogoProps {
  size?: number;
  className?: string;
}

export function AtmosLogo({ size = 200, className = '' }: AtmosLogoProps) {
  return (
    <div
      className={`atmos-logo-animated ${className}`.trim()}
      style={{ width: size, height: size }}
    >
      <img src="/img/atmos-logo.svg" alt="Atmos" width={size} height={size} />
    </div>
  );
}

export default AtmosLogo;
