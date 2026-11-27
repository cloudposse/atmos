import React from 'react';

import styles from './styles.module.css';

// Views rendered by the Atmos Pro screenshot service
// (https://atmos-pro.com/api/screenshot/<view>.png). Each view is captured from a
// fixture page with mock data, so the images never expose a real workspace.
export type AtmosProScreenshotView = 'approvals' | 'board' | 'pr-comment' | 'repos' | 'stacks';

const VIEW_DIMENSIONS: Record<AtmosProScreenshotView, { width: number; height: number }> = {
  approvals: { width: 1280, height: 720 },
  board: { width: 1280, height: 720 },
  'pr-comment': { width: 1024, height: 980 },
  repos: { width: 1280, height: 720 },
  stacks: { width: 1280, height: 720 },
};

export interface AtmosProScreenshotProps {
  view: AtmosProScreenshotView;
  alt: string;
  caption?: React.ReactNode;
  href?: string;
  // Crop the image to this aspect ratio (width / height), keeping the top edge.
  // Use it for views whose fixture leaves empty space at the bottom.
  cropAspectRatio?: number;
}

/**
 * Renders a live product screenshot from atmos-pro.com, framed like a browser
 * window so it reads as UI rather than decoration.
 */
export default function AtmosProScreenshot({
  view,
  alt,
  caption,
  href = 'https://atmos-pro.com',
  cropAspectRatio,
}: AtmosProScreenshotProps): JSX.Element {
  const { width, height } = VIEW_DIMENSIONS[view];
  const src = `https://atmos-pro.com/api/screenshot/${view}.png`;

  return (
    <figure className={styles.figure}>
      <a href={href} className={styles.frame} target="_blank" rel="noopener noreferrer">
        <span className={styles.chrome} aria-hidden="true">
          <span />
          <span />
          <span />
        </span>
        <img
          src={src}
          alt={alt}
          width={width}
          height={height}
          loading="lazy"
          decoding="async"
          className={cropAspectRatio ? styles.cropped : undefined}
          style={cropAspectRatio ? { aspectRatio: String(cropAspectRatio) } : undefined}
        />
      </a>
      {caption && <figcaption className={styles.caption}>{caption}</figcaption>}
    </figure>
  );
}
