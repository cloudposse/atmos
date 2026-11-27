import React, { useEffect, useState } from 'react';

import styles from './styles.module.css';

// Views served by the Atmos Pro embed service under https://atmos-pro.com/embed/screenshots/.
// Each view renders a fixture page with sample data, so it never exposes a real workspace.
export type AtmosProScreenshotView = 'approvals' | 'board' | 'pr-comment' | 'repos' | 'stacks';

const VIEW_DIMENSIONS: Record<AtmosProScreenshotView, { width: number; height: number }> = {
  approvals: { width: 1280, height: 720 },
  board: { width: 1280, height: 720 },
  'pr-comment': { width: 1024, height: 980 },
  repos: { width: 1280, height: 720 },
  stacks: { width: 1280, height: 720 },
};

const EMBED_BASE_URL = 'https://atmos-pro.com/embed/screenshots';

// The live HTML embed only allows these parent hosts (its Content-Security-Policy
// `frame-ancestors`). Elsewhere, such as localhost and docs preview deployments,
// the iframe would render blank, so the static PNG is shown instead.
function canEmbedLive(hostname: string): boolean {
  return hostname === 'atmos.tools' || hostname.endsWith('.atmos.tools');
}

export interface AtmosProScreenshotProps {
  view: AtmosProScreenshotView;
  alt: string;
  caption?: React.ReactNode;
  href?: string;
  // Crop to this aspect ratio (width / height), keeping the top edge.
  // Use it for views whose fixture leaves empty space at the bottom.
  cropAspectRatio?: number;
}

/**
 * Renders an Atmos Pro product view inside the same window chrome as the Terminal
 * component. The page always renders the static PNG; on hosts the live embed allows,
 * an interactive iframe of the same view loads over it and fades in once ready.
 */
export default function AtmosProScreenshot({
  view,
  alt,
  caption,
  href = 'https://atmos-pro.com',
  cropAspectRatio,
}: AtmosProScreenshotProps): JSX.Element {
  const { width, height } = VIEW_DIMENSIONS[view];
  const [live, setLive] = useState(false);
  const [liveLoaded, setLiveLoaded] = useState(false);

  useEffect(() => {
    setLive(canEmbedLive(window.location.hostname));
  }, []);

  return (
    <figure className={styles.figure}>
      <div className={`terminal ${styles.frame}`}>
        <div className="window-bar">
          <div className="window-controls" aria-hidden="true">
            <div className="control-dot close-dot"></div>
            <div className="control-dot minimize-dot"></div>
            <div className="control-dot maximize-dot"></div>
          </div>
          <a className={`window-title ${styles.title}`} href={href} target="_blank" rel="noopener noreferrer">
            atmos-pro.com
          </a>
        </div>
        <div className={styles.viewport}>
          <div
            className={styles.shot}
            style={cropAspectRatio ? { aspectRatio: String(cropAspectRatio) } : undefined}
          >
            <img
              src={`${EMBED_BASE_URL}/${view}.png`}
              alt={alt}
              width={width}
              height={height}
              loading="lazy"
              decoding="async"
              aria-hidden={liveLoaded || undefined}
            />
            {live && (
              <iframe
                src={`${EMBED_BASE_URL}/${view}?theme=dark`}
                title={alt}
                loading="lazy"
                className={styles.live}
                style={{ aspectRatio: `${width} / ${height}`, opacity: liveLoaded ? 1 : 0 }}
                onLoad={() => setLiveLoaded(true)}
              />
            )}
          </div>
        </div>
      </div>
      {caption && <figcaption className={styles.caption}>{caption}</figcaption>}
    </figure>
  );
}
