import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Stream, StreamPlayerApi } from '@cloudflare/stream-react';
import { FEATURED_DEMOS, getDemoAssetUrls } from '../../data/demos';
import styles from './styles.module.css';

interface FeaturedDemoCarouselProps {
  demos?: typeof FEATURED_DEMOS;
  delayBetweenVideos?: number; // Delay in milliseconds between videos (default: 5000).
}

export function FeaturedDemoCarousel({ demos = FEATURED_DEMOS, delayBetweenVideos = 5000 }: FeaturedDemoCarouselProps) {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [isHovering, setIsHovering] = useState(false);
  const [isWaitingForNext, setIsWaitingForNext] = useState(false);
  const [isTransitioning, setIsTransitioning] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const streamRef = useRef<StreamPlayerApi | undefined>(undefined);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const delayTimerRef = useRef<NodeJS.Timeout | null>(null);
  const svgTimerRef = useRef<NodeJS.Timeout | null>(null);

  const currentDemo = demos[currentIndex];
  const { streamUid, thumbnail, svg, mp3, svgDuration } = currentDemo
    ? getDemoAssetUrls(currentDemo.id)
    : { streamUid: null, thumbnail: null, svg: null, mp3: null, svgDuration: null };

  // Prefer SVG over Stream video for playback.
  const useSvg = !!svg;

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
  useEffect(() => {
    if (useSvg && svgDuration && svgDuration > 0) {
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
  }, [currentIndex, useSvg, svgDuration, mp3]);

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
  };

  // Smooth transition with fade out/in.
  const transitionToIndex = (newIndex: number) => {
    if (newIndex === currentIndex) return;

    // Fade out.
    setIsTransitioning(true);

    // After fade out, switch video and fade back in.
    setTimeout(() => {
      setCurrentIndex(newIndex);
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
            // SVG animated terminal recording.
            <img
              key={`${currentDemo.id}-svg`}
              src={svg}
              alt={currentDemo?.title || 'Demo'}
              className={styles.svgPlayer}
            />
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
