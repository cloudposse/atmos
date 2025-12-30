import React, { useEffect, useCallback, useState, useRef, Children, isValidElement, ReactElement } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { RiArrowLeftSLine, RiArrowRightSLine, RiFullscreenLine, RiFullscreenExitLine, RiMenuLine, RiSpeakLine } from 'react-icons/ri';
import { SlideDeckProvider, useSlideDeck } from './SlideDeckContext';
import { SlideDrawer } from './SlideDrawer';
import { SlideNotesPanel } from './SlideNotesPanel';
import { Tooltip } from './Tooltip';
import type { SlideDeckProps } from './types';
import './SlideDeck.css';

type SlideDeckInnerProps = Omit<SlideDeckProps, 'startSlide'>;

function SlideDeckInner({
  children,
  title,
  showProgress = true,
  showNavigation = true,
  showFullscreen = true,
  showDrawer = true,
  className = '',
}: SlideDeckInnerProps) {
  const {
    currentSlide,
    totalSlides,
    nextSlide,
    prevSlide,
    isFullscreen,
    toggleFullscreen,
    showNotes,
    toggleNotes,
  } = useSlideDeck();

  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [isHovering, setIsHovering] = useState(false);
  const [showControls, setShowControls] = useState(true);
  const hideTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const drawerHoverTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const openDrawer = useCallback(() => setIsDrawerOpen(true), []);
  const closeDrawer = useCallback(() => setIsDrawerOpen(false), []);

  // Show controls and reset hide timer.
  const showControlsTemporarily = useCallback(() => {
    setShowControls(true);
    if (hideTimeoutRef.current) {
      clearTimeout(hideTimeoutRef.current);
    }
    // Hide after 2 seconds of inactivity if not hovering.
    hideTimeoutRef.current = setTimeout(() => {
      if (!isHovering) {
        setShowControls(false);
      }
    }, 2000);
  }, [isHovering]);

  // Handle mouse enter/leave.
  const handleMouseEnter = useCallback(() => {
    setIsHovering(true);
    setShowControls(true);
    if (hideTimeoutRef.current) {
      clearTimeout(hideTimeoutRef.current);
    }
  }, []);

  const handleMouseLeave = useCallback(() => {
    setIsHovering(false);
    // Hide controls after a short delay when mouse leaves.
    hideTimeoutRef.current = setTimeout(() => {
      setShowControls(false);
    }, 500);
  }, []);

  // Handle left edge hover for drawer.
  const handleLeftEdgeEnter = useCallback(() => {
    if (drawerHoverTimeoutRef.current) {
      clearTimeout(drawerHoverTimeoutRef.current);
    }
    drawerHoverTimeoutRef.current = setTimeout(() => {
      setIsDrawerOpen(true);
    }, 200);
  }, []);

  const handleLeftEdgeLeave = useCallback(() => {
    if (drawerHoverTimeoutRef.current) {
      clearTimeout(drawerHoverTimeoutRef.current);
    }
  }, []);

  // Keyboard navigation.
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    // Don't intercept keys when user is typing in form elements.
    const target = e.target as HTMLElement;
    const isEditable =
      target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.isContentEditable;
    if (isEditable) return;

    // Show controls on any key press.
    showControlsTemporarily();

    // Close drawer or notes panel on Escape.
    if (e.key === 'Escape') {
      if (isDrawerOpen) {
        e.preventDefault();
        closeDrawer();
        return;
      }
      if (showNotes) {
        e.preventDefault();
        toggleNotes();
        return;
      }
      if (isFullscreen) {
        e.preventDefault();
        toggleFullscreen();
        return;
      }
    }

    if (e.key === 'ArrowRight' || e.key === ' ') {
      e.preventDefault();
      nextSlide();
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      prevSlide();
    } else if (e.key === 'f' || e.key === 'F') {
      e.preventDefault();
      toggleFullscreen();
    } else if (e.key === 'g' || e.key === 'G') {
      e.preventDefault();
      setIsDrawerOpen(prev => !prev);
    } else if (e.key === 'n' || e.key === 'N') {
      e.preventDefault();
      toggleNotes();
    }
  }, [nextSlide, prevSlide, isFullscreen, toggleFullscreen, isDrawerOpen, closeDrawer, showControlsTemporarily, showNotes, toggleNotes]);

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  // Cleanup timeouts on unmount.
  useEffect(() => {
    return () => {
      if (hideTimeoutRef.current) {
        clearTimeout(hideTimeoutRef.current);
      }
      if (drawerHoverTimeoutRef.current) {
        clearTimeout(drawerHoverTimeoutRef.current);
      }
    };
  }, []);

  // Convert children to array and get current slide.
  const slides = Children.toArray(children).filter(isValidElement) as ReactElement[];
  const currentSlideElement = slides[currentSlide - 1];

  const controlsVisible = showControls || isDrawerOpen || showNotes;

  return (
    <div
      className={`slide-deck ${isFullscreen ? 'slide-deck--fullscreen' : ''} ${controlsVisible ? '' : 'slide-deck--controls-hidden'} ${className}`}
      data-slide-deck
      role="region"
      aria-label={title}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      {/* Slide container with side navigation */}
      <div className="slide-deck__main">
        {/* Left navigation area - triggers drawer on hover */}
        <div
          className="slide-deck__left-area"
          onMouseEnter={showDrawer && !isDrawerOpen ? handleLeftEdgeEnter : undefined}
          onMouseLeave={showDrawer && !isDrawerOpen ? handleLeftEdgeLeave : undefined}
        >
          {showNavigation && (
            <Tooltip content="Previous (←)" position="right">
              <button
                className="slide-deck__side-nav slide-deck__side-nav--prev"
                onClick={prevSlide}
                disabled={currentSlide === 1}
                aria-label="Previous slide"
              >
                <RiArrowLeftSLine />
              </button>
            </Tooltip>
          )}
        </div>

        {/* Slide content area */}
        <div className="slide-deck__container">
          <AnimatePresence mode="wait">
            <motion.div
              key={currentSlide}
              className="slide-deck__slide-wrapper"
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={{ duration: 0.3, ease: 'easeInOut' }}
            >
              {currentSlideElement}
            </motion.div>
          </AnimatePresence>
        </div>

        {/* Right navigation button */}
        {showNavigation && (
          <Tooltip content="Next (→)" position="left">
            <button
              className="slide-deck__side-nav slide-deck__side-nav--next"
              onClick={nextSlide}
              disabled={currentSlide === totalSlides}
              aria-label="Next slide"
            >
              <RiArrowRightSLine />
            </button>
          </Tooltip>
        )}
      </div>

      {/* Bottom toolbar - minimal */}
      <div className="slide-deck__toolbar">
        {showDrawer && (
          <Tooltip content="Slides (G)" position="top">
            <button
              className="slide-deck__tool-button"
              onClick={openDrawer}
              aria-label="Open slide drawer"
            >
              <RiMenuLine />
            </button>
          </Tooltip>
        )}

        <Tooltip content={showNotes ? 'Hide Notes (N)' : 'Speaker Notes (N)'} position="top">
          <button
            className={`slide-deck__tool-button ${showNotes ? 'slide-deck__tool-button--active' : ''}`}
            onClick={toggleNotes}
            aria-label={showNotes ? 'Hide speaker notes' : 'Show speaker notes'}
          >
            <RiSpeakLine />
          </button>
        </Tooltip>

        {showProgress && (
          <div className="slide-deck__progress">
            {currentSlide} / {totalSlides}
          </div>
        )}

        {showFullscreen && (
          <Tooltip content={isFullscreen ? 'Exit Fullscreen (F)' : 'Fullscreen (F)'} position="top">
            <button
              className="slide-deck__tool-button"
              onClick={toggleFullscreen}
              aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
            >
              {isFullscreen ? <RiFullscreenExitLine /> : <RiFullscreenLine />}
            </button>
          </Tooltip>
        )}
      </div>

      {/* Progress bar */}
      <div className="slide-deck__progress-bar">
        <div
          className="slide-deck__progress-bar-fill"
          style={{ width: `${(currentSlide / totalSlides) * 100}%` }}
        />
      </div>

      {/* Slide drawer */}
      {showDrawer && (
        <SlideDrawer isOpen={isDrawerOpen} onClose={closeDrawer}>
          {children}
        </SlideDrawer>
      )}

      {/* Speaker notes panel */}
      <SlideNotesPanel isOpen={showNotes} onClose={toggleNotes} />
    </div>
  );
}

export function SlideDeck({
  children,
  startSlide = 1,
  ...props
}: SlideDeckProps) {
  const slides = Children.toArray(children).filter(isValidElement);
  const totalSlides = slides.length;

  return (
    <SlideDeckProvider totalSlides={totalSlides} startSlide={startSlide}>
      <SlideDeckInner {...props}>{children}</SlideDeckInner>
    </SlideDeckProvider>
  );
}

export default SlideDeck;
