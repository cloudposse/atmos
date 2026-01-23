import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Stream, StreamPlayerApi } from '@cloudflare/stream-react';
import { FEATURED_DEMOS, getDemoAssetUrls } from '../../data/demos';
import styles from './styles.module.css';

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
  // Replace the opening tag to include viewBox and set dimensions to 100%.
  let result = svgContent.replace(
    /<svg\s+xmlns="([^"]*)"\s+width="\d+"\s+height="\d+"/,
    `<svg xmlns="$1" viewBox="0 0 ${width} ${height}" width="100%" height="100%"`
  );

  return result;
}

interface FeaturedDemoCarouselProps {
  demos?: typeof FEATURED_DEMOS;
  delayBetweenVideos?: number; // Delay in milliseconds between videos (default: 5000).
}

export function FeaturedDemoCarousel({ demos = FEATURED_DEMOS, delayBetweenVideos = 5000 }: FeaturedDemoCarouselProps) {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [isHovering, setIsHovering] = useState(false);
  const [isWaitingForNext, setIsWaitingForNext] = useState(false);
  const [isTransitioning, setIsTransitioning] = useState(false);
  const [isSvgPlaying, setIsSvgPlaying] = useState(false);
  const [isSvgPaused, setIsSvgPaused] = useState(false);
  const [svgContent, setSvgContent] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<StreamPlayerApi | undefined>(undefined);
  const svgContainerRef = useRef<HTMLDivElement>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const delayTimerRef = useRef<NodeJS.Timeout | null>(null);
  const svgTimerRef = useRef<NodeJS.Timeout | null>(null);

  const currentDemo = demos[currentIndex];
  const { streamUid, thumbnail, svg, png, mp3, svgDuration } = currentDemo
    ? getDemoAssetUrls(currentDemo.id)
    : { streamUid: null, thumbnail: null, svg: null, png: null, mp3: null, svgDuration: null };

  // Prefer SVG over Stream video for playback.
  const useSvg = !!svg;

  // Fetch SVG content when playing starts.
  useEffect(() => {
    if (!isSvgPlaying || !svg || svgContent) return;

    console.log('[Carousel] Fetching SVG from:', svg);
    fetch(svg)
      .then((response) => {
        if (!response.ok) {
          throw new Error(`Failed to fetch SVG: ${response.status}`);
        }
        return response.text();
      })
      .then((text) => {
        console.log('[Carousel] SVG fetched successfully, length:', text.length);
        setSvgContent(makeResponsiveSvg(text));
      })
      .catch((err) => {
        console.error('[Carousel] Failed to fetch SVG:', err);
      });
  }, [isSvgPlaying, svg, svgContent]);

  // Toggle SVG pause/play.
  // VHS-generated SVGs use CSS animations (keyframes), not SMIL.
  // We control playback via animation-play-state CSS property.
  const toggleSvgPause = useCallback(() => {
    console.log('[Carousel] toggleSvgPause called, isSvgPaused:', isSvgPaused);
    const container = svgContainerRef.current;
    if (!container) {
      console.log('[Carousel] No container ref');
      return;
    }

    const svgEl = container.querySelector('svg') as SVGSVGElement | null;
    console.log('[Carousel] SVG element:', svgEl);
    if (!svgEl) {
      console.log('[Carousel] No SVG element found in container');
      return;
    }

    // Toggle CSS animation-play-state for all animated elements.
    // VHS SVGs use .animation-container for the main slide animation,
    // plus various typing_* and blink animations on other elements.
    const newState = isSvgPaused ? 'running' : 'paused';
    console.log('[Carousel] Setting animation-play-state to:', newState);

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

    // Handle audio.
    if (isSvgPaused) {
      audioRef.current?.play().catch(() => {});
    } else {
      audioRef.current?.pause();
    }

    setIsSvgPaused(!isSvgPaused);
    console.log('[Carousel] State updated, new isSvgPaused:', !isSvgPaused);
  }, [isSvgPaused]);

  // Start SVG playback from poster.
  const startSvgPlayback = useCallback(() => {
    setIsSvgPlaying(true);
    setIsSvgPaused(false);
    setSvgContent(null); // Reset content for new demo.
  }, []);

  // Arrow key navigation (only when container is focused or in viewport).
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Only handle arrow keys when the carousel container or its children have focus.
      if (!containerRef.current?.contains(document.activeElement) && document.activeElement !== document.body) {
        return;
      }

      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        goToPrevious();
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        goToNext();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [demos.length]);

  // Clean up timers on unmount.
  useEffect(() => {
    return () => {
      if (delayTimerRef.current) {
        clearTimeout(delayTimerRef.current);
      }
      if (svgTimerRef.current) {
        clearTimeout(svgTimerRef.current);
      }
    };
  }, []);

  // Timer-based advancement for SVG (since SVG doesn't have onEnded event).
  // Only starts when isSvgPlaying is true (user clicked play).
  useEffect(() => {
    if (useSvg && svgDuration && svgDuration > 0 && isSvgPlaying && !isSvgPaused) {
      // Clear any existing timer.
      if (svgTimerRef.current) {
        clearTimeout(svgTimerRef.current);
      }

      // Set timer for SVG duration.
      svgTimerRef.current = setTimeout(() => {
        handleVideoEnd();
      }, svgDuration * 1000);

      // Start audio playback if available.
      if (mp3 && audioRef.current) {
        audioRef.current.currentTime = 0;
        audioRef.current.play().catch(() => {
          // Autoplay blocked - that's fine, proceed silently.
        });
      }

      return () => {
        if (svgTimerRef.current) {
          clearTimeout(svgTimerRef.current);
        }
      };
    }
  }, [currentIndex, useSvg, svgDuration, mp3, isSvgPlaying, isSvgPaused]);

  const handleVideoEnd = useCallback(() => {
    // Start delay before transitioning to next video.
    setIsWaitingForNext(true);
    delayTimerRef.current = setTimeout(() => {
      setIsWaitingForNext(false);
      transitionToIndex((currentIndex + 1) % demos.length);
    }, delayBetweenVideos);
  }, [currentIndex, demos.length, delayBetweenVideos]);

  const cancelPendingTransition = () => {
    if (delayTimerRef.current) {
      clearTimeout(delayTimerRef.current);
      delayTimerRef.current = null;
    }
    if (svgTimerRef.current) {
      clearTimeout(svgTimerRef.current);
      svgTimerRef.current = null;
    }
    // Pause audio when manually transitioning.
    if (audioRef.current) {
      audioRef.current.pause();
    }
    setIsWaitingForNext(false);
    // Reset SVG state for next demo.
    setIsSvgPlaying(false);
    setIsSvgPaused(false);
    setSvgContent(null);
  };

  // Smooth transition with fade out/in.
  const transitionToIndex = (newIndex: number) => {
    if (newIndex === currentIndex) return;

    // Fade out.
    setIsTransitioning(true);

    // After fade out, switch video and fade back in.
    setTimeout(() => {
      setCurrentIndex(newIndex);
      setSvgContent(null); // Reset SVG content for new demo.
      // Small delay to ensure video loads.
      setTimeout(() => {
        setIsTransitioning(false);
      }, 100);
    }, 200);
  };

  const goToDemo = (index: number) => {
    if (index !== currentIndex) {
      cancelPendingTransition();
      transitionToIndex(index);
    }
  };

  const goToPrevious = () => {
    cancelPendingTransition();
    transitionToIndex((currentIndex - 1 + demos.length) % demos.length);
  };

  const goToNext = () => {
    cancelPendingTransition();
    transitionToIndex((currentIndex + 1) % demos.length);
  };

  // Don't render if no demos available.
  if (!demos || demos.length === 0) {
    return null;
  }

  return (
    <div
      ref={containerRef}
      className={styles.container}
      tabIndex={0}
      role="region"
      aria-label="Featured demo carousel"
    >
      {/* Terminal chrome */}
      <div
        className={styles.terminal}
        onMouseEnter={() => setIsHovering(true)}
        onMouseLeave={() => setIsHovering(false)}
      >
        <div className={styles.windowBar}>
          <div className={styles.windowControls}>
            <div className={styles.closeDot} />
            <div className={styles.minimizeDot} />
            <div className={styles.maximizeDot} />
          </div>
          <span className={styles.title}>{currentDemo?.title || 'Demo'}</span>
          <div className={styles.spacer} />
        </div>

        {/* Video/SVG player - controls visible on hover */}
        <div className={`${styles.viewport} ${isTransitioning ? styles.fading : ''}`}>
          {useSvg ? (
            // SVG animated terminal recording with poster.
            !isSvgPlaying && png ? (
              // Show PNG poster until user clicks play.
              <div className={styles.posterContainer} onClick={startSvgPlayback}>
                <img
                  key={`${currentDemo.id}-poster`}
                  src={png}
                  alt={currentDemo?.title || 'Demo'}
                  className={styles.svgPlayer}
                />
                <div className={styles.playOverlay}>
                  <svg className={styles.playIcon} viewBox="0 0 24 24" fill="currentColor">
                    <path d="M8 5v14l11-7z" />
                  </svg>
                </div>
              </div>
            ) : (
              // Playing: show inlined SVG with pause control.
              <div className={styles.svgContainer}>
                {svgContent ? (
                  <div
                    ref={svgContainerRef}
                    className={styles.svgPlayer}
                    dangerouslySetInnerHTML={{ __html: svgContent }}
                  />
                ) : (
                  // Loading state - show poster while SVG loads.
                  <img
                    src={png || undefined}
                    alt={currentDemo?.title || 'Demo'}
                    className={styles.svgPlayer}
                  />
                )}
                <div
                  className={`${styles.controlOverlay} ${isSvgPaused ? styles.paused : ''}`}
                  onClick={toggleSvgPause}
                >
                  {isSvgPaused ? (
                    <svg className={styles.controlIcon} viewBox="0 0 24 24" fill="currentColor">
                      <path d="M8 5v14l11-7z" />
                    </svg>
                  ) : (
                    <svg className={styles.controlIcon} viewBox="0 0 24 24" fill="currentColor">
                      <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" />
                    </svg>
                  )}
                </div>
              </div>
            )
          ) : streamUid ? (
            <Stream
              key={streamUid}
              src={streamUid}
              controls={isHovering}
              autoplay={true}
              muted={true}
              loop={false}
              poster={thumbnail || undefined}
              streamRef={streamRef}
              onEnded={handleVideoEnd}
              responsive
            />
          ) : (
            <div className={styles.placeholder}>
              <span>Demo coming soon</span>
            </div>
          )}
        </div>

        {/* Hidden audio element for SVG playback */}
        {mp3 && (
          <audio
            ref={audioRef}
            src={mp3}
            preload="auto"
          />
        )}

        {/* Arrow navigation buttons (visible on hover) */}
        <button
          className={`${styles.arrowButton} ${styles.arrowLeft} ${isHovering ? styles.visible : ''}`}
          onClick={goToPrevious}
          aria-label="Previous demo"
        >
          <svg viewBox="0 0 24 24" fill="currentColor" width="24" height="24">
            <path d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z" />
          </svg>
        </button>
        <button
          className={`${styles.arrowButton} ${styles.arrowRight} ${isHovering ? styles.visible : ''}`}
          onClick={goToNext}
          aria-label="Next demo"
        >
          <svg viewBox="0 0 24 24" fill="currentColor" width="24" height="24">
            <path d="M8.59 16.59L10 18l6-6-6-6-1.41 1.41L13.17 12z" />
          </svg>
        </button>
      </div>

      {/* Description below terminal */}
      {currentDemo?.description && (
        <p className={styles.description}>{currentDemo.description}</p>
      )}

      {/* Dot navigation - below terminal */}
      <div className={styles.navigation} role="tablist" aria-label="Demo navigation">
        {demos.map((demo, i) => (
          <button
            key={demo.id}
            className={`${styles.dot} ${i === currentIndex ? styles.active : ''}`}
            onClick={() => goToDemo(i)}
            role="tab"
            aria-selected={i === currentIndex}
            aria-label={`Go to ${demo.title}`}
          />
        ))}
      </div>
    </div>
  );
}

export default FeaturedDemoCarousel;
