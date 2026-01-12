import React, { useState, useEffect } from 'react';

interface SvgThumbnailProps {
  src: string;
  freezeTime: number; // Time in seconds to freeze at
  alt?: string;
  className?: string;
}

/**
 * SvgThumbnail renders an SVG with its CSS animation paused at a specific time.
 * This is used for thumbnail displays where we want to show a specific frame
 * of an animated SVG without playing the animation.
 */
// Generate a unique ID for scoping CSS.
let svgIdCounter = 0;
function generateSvgId(): string {
  return `svg-thumb-${++svgIdCounter}`;
}

export function SvgThumbnail({ src, freezeTime, alt, className }: SvgThumbnailProps): JSX.Element | null {
  const [svgContent, setSvgContent] = useState<string | null>(null);
  const [error, setError] = useState<boolean>(false);
  const [svgId] = useState(() => generateSvgId());

  useEffect(() => {
    let cancelled = false;

    async function fetchSvg() {
      try {
        const response = await fetch(src);
        if (!response.ok) {
          throw new Error(`Failed to fetch SVG: ${response.status}`);
        }
        const text = await response.text();
        if (!cancelled) {
          setSvgContent(text);
        }
      } catch (err) {
        console.error('Failed to load SVG:', err);
        if (!cancelled) {
          setError(true);
        }
      }
    }

    fetchSvg();

    return () => {
      cancelled = true;
    };
  }, [src]);

  if (error || !svgContent) {
    // Fallback to regular img tag if fetch fails.
    return error ? null : <div className={className} aria-label={alt} />;
  }

  // Inject CSS to freeze the animation at the specified time.
  // The animation uses: animation: slide Xs step-end 0s infinite;
  // We pause it and set a negative delay to jump to the freeze point.
  // IMPORTANT: Scope CSS to this specific SVG using ID to avoid affecting other SVGs on the page.
  const freezeStyle = `
    <style>
      #${svgId} .animation-container {
        animation-play-state: paused !important;
        animation-delay: -${freezeTime}s !important;
      }
    </style>
  `;

  // Add unique ID to the SVG and insert the scoped freeze style.
  const modifiedSvg = svgContent.replace(
    /(<svg)([^>]*>)/i,
    `$1 id="${svgId}"$2${freezeStyle}`
  );

  return (
    <div
      className={className}
      aria-label={alt}
      dangerouslySetInnerHTML={{ __html: modifiedSvg }}
    />
  );
}

interface AnimatedSvgProps {
  src: string;
  alt?: string;
  className?: string;
}

/**
 * AnimatedSvg renders an SVG with its CSS animation playing.
 * SVGs must be embedded inline for CSS animations to work (img tags don't support them).
 */
export function AnimatedSvg({ src, alt, className }: AnimatedSvgProps): JSX.Element | null {
  const [svgContent, setSvgContent] = useState<string | null>(null);
  const [error, setError] = useState<boolean>(false);

  useEffect(() => {
    let cancelled = false;

    async function fetchSvg() {
      try {
        const response = await fetch(src);
        if (!response.ok) {
          throw new Error(`Failed to fetch SVG: ${response.status}`);
        }
        const text = await response.text();
        if (!cancelled) {
          setSvgContent(text);
        }
      } catch (err) {
        console.error('Failed to load SVG:', err);
        if (!cancelled) {
          setError(true);
        }
      }
    }

    fetchSvg();

    return () => {
      cancelled = true;
    };
  }, [src]);

  if (error || !svgContent) {
    return error ? null : <div className={className} aria-label={alt} />;
  }

  return (
    <div
      className={className}
      aria-label={alt}
      dangerouslySetInnerHTML={{ __html: svgContent }}
    />
  );
}

export default SvgThumbnail;
