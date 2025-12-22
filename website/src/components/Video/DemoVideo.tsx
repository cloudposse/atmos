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
}

/**
 * DemoVideo component displays terminal demo recordings.
 *
 * Uses Cloudflare Stream for video playback when available,
 * with automatic fallback to GIF for older demos.
 */
export default function DemoVideo({
  id,
  title,
  description,
  autoPlay = true,
  loop = true,
  muted = true,
}: DemoVideoProps): JSX.Element {
  // Get asset URLs and manifest data
  const assets = getDemoAssetUrls(id);
  const manifestData = getManifestData(id);

  // Check if this demo has a Stream video
  const streamUid = assets.streamUid || manifestData?.formats?.mp4?.uid;
  const thumbnail = assets.thumbnail;

  // If we have a Stream UID, use the CloudflareStream player
  if (streamUid) {
    return (
      <div className={styles.videoContainer}>
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
        {(title || description) && (
          <div className={styles.caption}>
            {title && <h4 className={styles.title}>{title}</h4>}
            {description && <p className={styles.description}>{description}</p>}
          </div>
        )}
      </div>
    );
  }

  // Fallback to GIF for demos without Stream video
  return (
    <div className={styles.videoContainer}>
      <img
        className={styles.gif}
        src={assets.gif}
        alt={title}
        loading="lazy"
      />
      {(title || description) && (
        <div className={styles.caption}>
          {title && <h4 className={styles.title}>{title}</h4>}
          {description && <p className={styles.description}>{description}</p>}
        </div>
      )}
    </div>
  );
}
