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
 * Renders a live product screenshot from atmos-pro.com inside the same window
 * chrome as the Terminal component, so screenshots and terminals on a page share
 * one border, radius, and title-bar style.
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
      <a href={href} className={`terminal ${styles.frame}`} target="_blank" rel="noopener noreferrer">
        <div className="window-bar">
          <div className="window-controls" aria-hidden="true">
            <div className="control-dot close-dot"></div>
            <div className="control-dot minimize-dot"></div>
            <div className="control-dot maximize-dot"></div>
          </div>
          <div className="window-title">atmos-pro.com</div>
        </div>
        <div className={styles.viewport}>
          <div className={styles.shot}>
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
          </div>
        </div>
      </a>
      {caption && <figcaption className={styles.caption}>{caption}</figcaption>}
    </figure>
  );
}
