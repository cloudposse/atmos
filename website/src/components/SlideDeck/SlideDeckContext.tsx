import React, { createContext, useContext, useState, useCallback, useEffect, ReactNode, useMemo, useRef } from 'react';
import type { SlideDeckContextValue, NotesPreferences, NotesPosition, NotesDisplayMode } from './types';

const SlideDeckContext = createContext<SlideDeckContextValue | null>(null);

// localStorage key for notes preferences.
const NOTES_PREFS_KEY = 'slide-deck-notes-preferences';

// Default notes preferences.
const defaultNotesPreferences: NotesPreferences = {
  position: 'right',
  displayMode: 'overlay',
  isPopout: false,
};

// Load preferences from localStorage.
const loadNotesPreferences = (): NotesPreferences => {
  if (typeof window === 'undefined') return defaultNotesPreferences;
  try {
    const stored = localStorage.getItem(NOTES_PREFS_KEY);
    if (stored) {
      return { ...defaultNotesPreferences, ...JSON.parse(stored) };
    }
  } catch (e) {
    console.error('Failed to load notes preferences:', e);
  }
  return defaultNotesPreferences;
};

// Save preferences to localStorage.
const saveNotesPreferences = (prefs: NotesPreferences) => {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(NOTES_PREFS_KEY, JSON.stringify(prefs));
  } catch (e) {
    console.error('Failed to save notes preferences:', e);
  }
};

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
  // Initialize fullscreen to false to avoid hydration mismatch (server always renders false).
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showNotes, setShowNotes] = useState(false);
  const [currentNotes, setCurrentNotes] = useState<React.ReactNode | null>(null);
  const [isMobile, setIsMobile] = useState(false);
  const [notesPreferences, setNotesPreferences] = useState<NotesPreferences>(defaultNotesPreferences);

  // Ref to track current fullscreen state for resize handler (avoids stale closure).
  const isFullscreenRef = useRef(isFullscreen);
  isFullscreenRef.current = isFullscreen;

  // Load notes preferences and set mobile/fullscreen state after mount (client-side only).
  useEffect(() => {
    setNotesPreferences(loadNotesPreferences());
    // Auto-enter fullscreen on mobile after hydration.
    if (isMobileDevice()) {
      setIsMobile(true);
      setIsFullscreen(true);
    }
  }, []);

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
      // Use ref to get current fullscreen state (avoids stale closure).
      const mobile = isMobileDevice();
      setIsMobile(mobile);
      if (mobile && !isFullscreenRef.current) {
        setIsFullscreen(true);
      } else if (!mobile && !document.fullscreenElement && isFullscreenRef.current) {
        setIsFullscreen(false);
      }
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    window.addEventListener('resize', handleResize);
    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
      window.removeEventListener('resize', handleResize);
    };
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

  const toggleNotes = useCallback(() => {
    setShowNotes(prev => !prev);
  }, []);

  const setNotesPosition = useCallback((position: NotesPosition) => {
    setNotesPreferences(prev => {
      const updated = { ...prev, position };
      saveNotesPreferences(updated);
      return updated;
    });
  }, []);

  const setNotesDisplayMode = useCallback((displayMode: NotesDisplayMode) => {
    setNotesPreferences(prev => {
      const updated = { ...prev, displayMode };
      saveNotesPreferences(updated);
      return updated;
    });
  }, []);

  const setNotesPopout = useCallback((isPopout: boolean) => {
    setNotesPreferences(prev => {
      const updated = { ...prev, isPopout };
      saveNotesPreferences(updated);
      return updated;
    });
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
    notesPreferences,
    setNotesPosition,
    setNotesDisplayMode,
    setNotesPopout,
    isMobile,
  }), [currentSlide, totalSlides, goToSlide, nextSlide, prevSlide, isFullscreen, toggleFullscreen, showNotes, toggleNotes, currentNotes, notesPreferences, setNotesPosition, setNotesDisplayMode, setNotesPopout, isMobile]);

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
