import React, { createContext, useContext, useState, useCallback, useEffect, ReactNode, useMemo } from 'react';
import type { SlideDeckContextValue } from './types';

const SlideDeckContext = createContext<SlideDeckContextValue | null>(null);

interface SlideDeckProviderProps {
  children: ReactNode;
  totalSlides: number;
  startSlide?: number;
}

// Check if device is mobile/tablet (touch device or small screen).
const isMobileDevice = () => {
  if (typeof window === 'undefined') return false;
  // Check for touch capability or small screen.
  const hasTouch = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
  const isSmallScreen = window.innerWidth <= 1024;
  return hasTouch && isSmallScreen;
};

export function SlideDeckProvider({
  children,
  totalSlides,
  startSlide = 1
}: SlideDeckProviderProps) {
  const [currentSlide, setCurrentSlide] = useState(startSlide);
  // Start in fullscreen mode on mobile devices.
  const [isFullscreen, setIsFullscreen] = useState(() => isMobileDevice());
  const [showNotes, setShowNotes] = useState(false);
  const [currentNotes, setCurrentNotes] = useState<React.ReactNode | null>(null);

  // Sync with URL hash on mount.
  useEffect(() => {
    const hash = window.location.hash;
    const match = hash.match(/^#slide-(\d+)$/);
    if (match) {
      const slideNum = parseInt(match[1], 10);
      if (slideNum >= 1 && slideNum <= totalSlides) {
        setCurrentSlide(slideNum);
      }
    }
  }, [totalSlides]);

  // Update URL hash when slide changes.
  useEffect(() => {
    const newHash = `#slide-${currentSlide}`;
    if (window.location.hash !== newHash) {
      window.history.replaceState(null, '', newHash);
    }
  }, [currentSlide]);

  // Handle browser back/forward.
  useEffect(() => {
    const handleHashChange = () => {
      const hash = window.location.hash;
      const match = hash.match(/^#slide-(\d+)$/);
      if (match) {
        const slideNum = parseInt(match[1], 10);
        if (slideNum >= 1 && slideNum <= totalSlides) {
          setCurrentSlide(slideNum);
        }
      }
    };

    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, [totalSlides]);

  // Handle fullscreen change events and mobile detection.
  useEffect(() => {
    const handleFullscreenChange = () => {
      // If native fullscreen changed, sync state.
      // But keep fullscreen on if we're on mobile.
      if (document.fullscreenElement) {
        setIsFullscreen(true);
      } else if (!isMobileDevice()) {
        setIsFullscreen(false);
      }
    };

    const handleResize = () => {
      // Auto-enter fullscreen mode on mobile, exit on desktop (unless native fullscreen).
      const mobile = isMobileDevice();
      if (mobile && !isFullscreen) {
        setIsFullscreen(true);
      } else if (!mobile && !document.fullscreenElement && isFullscreen) {
        setIsFullscreen(false);
      }
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    window.addEventListener('resize', handleResize);
    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
      window.removeEventListener('resize', handleResize);
    };
  }, [isFullscreen]);

  const goToSlide = useCallback((index: number) => {
    if (index >= 1 && index <= totalSlides) {
      setCurrentSlide(index);
    }
  }, [totalSlides]);

  const nextSlide = useCallback(() => {
    setCurrentSlide(prev => Math.min(prev + 1, totalSlides));
  }, [totalSlides]);

  const prevSlide = useCallback(() => {
    setCurrentSlide(prev => Math.max(prev - 1, 1));
  }, []);

  const toggleFullscreen = useCallback(async () => {
    try {
      if (!document.fullscreenElement) {
        const deckElement = document.querySelector('[data-slide-deck]');
        if (deckElement) {
          await deckElement.requestFullscreen();
        }
      } else {
        await document.exitFullscreen();
      }
    } catch (err) {
      console.error('Fullscreen error:', err);
    }
  }, []);

  const toggleNotes = useCallback(() => {
    setShowNotes(prev => !prev);
  }, []);

  const value: SlideDeckContextValue = useMemo(() => ({
    currentSlide,
    totalSlides,
    goToSlide,
    nextSlide,
    prevSlide,
    isFullscreen,
    toggleFullscreen,
    showNotes,
    toggleNotes,
    currentNotes,
    setCurrentNotes,
  }), [currentSlide, totalSlides, goToSlide, nextSlide, prevSlide, isFullscreen, toggleFullscreen, showNotes, toggleNotes, currentNotes]);

  return (
    <SlideDeckContext.Provider value={value}>
      {children}
    </SlideDeckContext.Provider>
  );
}

export function useSlideDeck(): SlideDeckContextValue {
  const context = useContext(SlideDeckContext);
  if (!context) {
    throw new Error('useSlideDeck must be used within a SlideDeckProvider');
  }
  return context;
}

export { SlideDeckContext };
