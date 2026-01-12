import React, { useState, useRef, useCallback, useEffect } from 'react';
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

// Play button SVG icon.
function PlayIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor">
      <path d="M8 5v14l11-7z" />
    </svg>
  );
}

// Pause button SVG icon.
function PauseIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor">
      <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" />
    </svg>
  );
}

// Make SVG responsive by adding viewBox to root and setting width/height to 100%.
// VHS SVGs have hardcoded pixel dimensions that prevent proper CSS scaling.
// VHS structure: <svg width="1400" height="800"><svg viewBox="...">...</svg></svg>
function makeResponsiveSvg(svgContent: string): string {
  // Extract width/height from the ROOT svg element (first one).
  const rootSvgMatch = svgContent.match(/<svg\s+xmlns="[^"]*"\s+width="(\d+)"\s+height="(\d+)"/);

  if (!rootSvgMatch) {
    // Fallback: just return as-is if we can't parse
    return svgContent;
  }

  const width = rootSvgMatch[1];
  const height = rootSvgMatch[2];

  // Add viewBox to the root SVG element to preserve aspect ratio when scaled.
  // Replace the opening tag to include viewBox. Remove height to let SVG scale naturally.
  // With viewBox + width="100%", the SVG will maintain aspect ratio automatically.
  let result = svgContent.replace(
    /<svg\s+xmlns="([^"]*)"\s+width="\d+"\s+height="\d+"/,
    `<svg xmlns="$1" viewBox="0 0 ${width} ${height}" width="100%"`
  );

  return result;
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
  // State for SVG playback control.
  // Start playing immediately if autoPlay is true.
  const [isPlaying, setIsPlaying] = useState(autoPlay);
  const [isPaused, setIsPaused] = useState(false);
  const [svgContent, setSvgContent] = useState<string | null>(null);
  const [svgError, setSvgError] = useState(false);
  const svgContainerRef = useRef<HTMLDivElement>(null);

  // Get asset URLs and manifest data.
  const assets = getDemoAssetUrls(id);
  const manifestData = getManifestData(id);

  // Check what formats are available.
  const svgUrl = assets.svg;
  const pngUrl = assets.png;
  const streamUid = assets.streamUid || manifestData?.formats?.mp4?.uid;
  const thumbnail = assets.thumbnail;

  // Fetch SVG content when playing starts.
  // Note: Requires CORS headers on the R2 bucket's custom domain.
  // The pub-*.r2.dev URLs don't support CORS - use a custom domain like demos.atmos.tools.
  useEffect(() => {
    if (!isPlaying || !svgUrl || svgContent || svgError) return;

    console.log('[DemoVideo] Fetching SVG from:', svgUrl);
    fetch(svgUrl, { mode: 'cors' })
      .then((response) => {
        if (!response.ok) {
          throw new Error(`Failed to fetch SVG: ${response.status}`);
        }
        return response.text();
      })
      .then((text) => {
        console.log('[DemoVideo] SVG fetched successfully, length:', text.length);
        setSvgContent(makeResponsiveSvg(text));
      })
      .catch((err) => {
        console.error('[DemoVideo] Failed to fetch SVG (CORS?):', err);
        console.log('[DemoVideo] Falling back to video player');
        setSvgError(true);
      });
  }, [isPlaying, svgUrl, svgContent, svgError]);

  // Handle SVG play/pause toggle.
  // VHS-generated SVGs use CSS animations (keyframes), not SMIL.
  // We control playback via animation-play-state CSS property.
  const toggleSvgPlayback = useCallback(() => {
    console.log('[DemoVideo] toggleSvgPlayback called, isPaused:', isPaused);
    const container = svgContainerRef.current;
    if (!container) {
      console.log('[DemoVideo] No container ref');
      return;
    }

    const svgEl = container.querySelector('svg') as SVGSVGElement | null;
    console.log('[DemoVideo] SVG element:', svgEl);
    if (!svgEl) {
      console.log('[DemoVideo] No SVG element found in container');
      return;
    }

    // Toggle CSS animation-play-state for all animated elements.
    // VHS SVGs use .animation-container for the main slide animation,
    // plus various typing_* and blink animations on other elements.
    const newState = isPaused ? 'running' : 'paused';
    console.log('[DemoVideo] Setting animation-play-state to:', newState);

    // Inject a style element to override all animations with !important.
    // This is more reliable than setting inline styles on each element.
    let pauseStyle = svgEl.querySelector('#pause-style') as HTMLStyleElement;
    if (!pauseStyle) {
      pauseStyle = document.createElementNS('http://www.w3.org/2000/svg', 'style') as unknown as HTMLStyleElement;
      pauseStyle.id = 'pause-style';
      svgEl.prepend(pauseStyle);
    }

    if (newState === 'paused') {
      pauseStyle.textContent = '* { animation-play-state: paused !important; }';
    } else {
      pauseStyle.textContent = '';
    }

    setIsPaused(!isPaused);
    console.log('[DemoVideo] State updated, new isPaused:', !isPaused);
  }, [isPaused]);

  // Start playing SVG animation.
  const startPlaying = useCallback(() => {
    setIsPlaying(true);
  }, []);

  // Determine container class based on caption visibility.
  const containerClass = showCaption
    ? styles.videoContainer
    : `${styles.videoContainer} ${styles.noCaption}`;

  // Priority 1: SVG animated terminal recording with poster.
  if (svgUrl && !svgError) {
    // Show PNG poster until user clicks to play (only if not autoPlay).
    if (!isPlaying && pngUrl) {
      return (
        <div className={containerClass}>
          <div className={styles.videoWrapper} onClick={startPlaying}>
            <img
              className={styles.svgPlayer}
              src={pngUrl}
              alt={title}
              loading="lazy"
            />
            <div className={styles.playOverlay}>
              <PlayIcon className={styles.playIcon} />
            </div>
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

    // Playing state: show inlined SVG with pause control.
    // Fetches SVG and inlines it to enable JavaScript access without CORS issues.
    return (
      <div className={containerClass}>
        <div className={styles.videoWrapper}>
          {svgContent ? (
            <div
              ref={svgContainerRef}
              className={styles.svgPlayer}
              dangerouslySetInnerHTML={{ __html: svgContent }}
            />
          ) : (
            // Loading state - show poster while SVG loads.
            <img
              className={styles.svgPlayer}
              src={pngUrl || undefined}
              alt={title}
            />
          )}
          <div
            className={`${styles.controlOverlay} ${isPaused ? styles.paused : ''}`}
            onClick={toggleSvgPlayback}
          >
            {isPaused ? (
              <PlayIcon className={styles.controlIcon} />
            ) : (
              <PauseIcon className={styles.controlIcon} />
            )}
          </div>
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

  // Priority 3: GIF fallback (or placeholder if no assets available).
  return (
    <div className={containerClass}>
      {assets.gif ? (
        <img
          className={styles.gif}
          src={assets.gif}
          alt={title}
          loading="lazy"
        />
      ) : (
        <div className={styles.placeholder}>
          <p>Demo not yet available</p>
        </div>
      )}
      {showCaption && (title || description) && (
        <div className={styles.caption}>
          {title && <h4 className={styles.title}>{title}</h4>}
          {description && <p className={styles.description}>{description}</p>}
        </div>
      )}
    </div>
  );
}
