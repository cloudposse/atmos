import React from 'react';
import { CloudflareStream } from './CloudflareStream';
import { getDemoAssetUrls, getManifestData } from '../../data/demos';
import styles from './DemoVideo.module.css';

interface DemoVideoProps {
  id: string;
  title: string;
  description?: string;
  autoPlay?: boolean;
  loop?: boolean;
  muted?: boolean;
  showCaption?: boolean;
}

/**
 * DemoVideo component displays terminal demo recordings.
 *
 * Priority order:
 * 1. SVG (animated terminal recording) - preferred for quality and file size
 * 2. Cloudflare Stream video (MP4)
 * 3. GIF fallback for older demos
 */
export default function DemoVideo({
  id,
  title,
  description,
  autoPlay = true,
  loop = true,
  muted = true,
  showCaption = true,
}: DemoVideoProps): JSX.Element {
  // Get asset URLs and manifest data.
  const assets = getDemoAssetUrls(id);
  const manifestData = getManifestData(id);

  // Check what formats are available.
  const svgUrl = assets.svg;
  const streamUid = assets.streamUid || manifestData?.formats?.mp4?.uid;
  const thumbnail = assets.thumbnail;

  // Determine container class based on caption visibility.
  const containerClass = showCaption
    ? styles.videoContainer
    : `${styles.videoContainer} ${styles.noCaption}`;

  // Priority 1: SVG animated terminal recording.
  if (svgUrl) {
    return (
      <div className={containerClass}>
        <div className={styles.videoWrapper}>
          <img
            className={styles.svgPlayer}
            src={svgUrl}
            alt={title}
            loading="lazy"
          />
        </div>
        {showCaption && (title || description) && (
          <div className={styles.caption}>
            {title && <h4 className={styles.title}>{title}</h4>}
            {description && <p className={styles.description}>{description}</p>}
          </div>
        )}
      </div>
    );
  }

  // Priority 2: Cloudflare Stream video.
  if (streamUid) {
    return (
      <div className={containerClass}>
        <div className={styles.videoWrapper}>
          <CloudflareStream
            src={streamUid}
            controls={true}
            autoplay={autoPlay}
            muted={muted}
            loop={loop}
            poster={thumbnail}
            className={styles.streamPlayer}
          />
        </div>
        {showCaption && (title || description) && (
          <div className={styles.caption}>
            {title && <h4 className={styles.title}>{title}</h4>}
            {description && <p className={styles.description}>{description}</p>}
          </div>
        )}
      </div>
    );
  }

  // Priority 3: GIF fallback.
  return (
    <div className={containerClass}>
      <img
        className={styles.gif}
        src={assets.gif}
        alt={title}
        loading="lazy"
      />
      {showCaption && (title || description) && (
        <div className={styles.caption}>
          {title && <h4 className={styles.title}>{title}</h4>}
          {description && <p className={styles.description}>{description}</p>}
        </div>
      )}
    </div>
  );
}
