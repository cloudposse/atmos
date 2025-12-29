import React, { createContext, useContext, useState, useCallback, useEffect, ReactNode } from 'react';
import type { SlideDeckContextValue } from './types';

const SlideDeckContext = createContext<SlideDeckContextValue | null>(null);

interface SlideDeckProviderProps {
  children: ReactNode;
  totalSlides: number;
  startSlide?: number;
}

export function SlideDeckProvider({
  children,
  totalSlides,
  startSlide = 1
}: SlideDeckProviderProps) {
  const [currentSlide, setCurrentSlide] = useState(startSlide);
  const [isFullscreen, setIsFullscreen] = useState(false);

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

  // Handle fullscreen change events.
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange);
  }, []);

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

  const value: SlideDeckContextValue = {
    currentSlide,
    totalSlides,
    goToSlide,
    nextSlide,
    prevSlide,
    isFullscreen,
    toggleFullscreen,
  };

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
