import React, { useCallback } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import {
  RiCloseLine,
  RiSpeakLine,
  RiLayoutRightLine,
  RiLayoutBottomLine,
  RiStackLine,
  RiSplitCellsHorizontal,
  RiExternalLinkLine,
} from 'react-icons/ri';
import { useSlideDeck } from './SlideDeckContext';
import { Tooltip } from './Tooltip';
import type { SlideNotesPanelProps } from './types';
import './SlideNotes.css';

/**
 * SlideNotesPanel - A slide-out panel displaying speaker notes.
 *
 * Supports two positions:
 * - 'right': slides in from the right (default)
 * - 'bottom': slides up from the bottom (Google Slides style)
 *
 * Supports two display modes:
 * - 'overlay': floats on top of slides with backdrop
 * - 'shrink': shrinks the slide area (no backdrop)
 */
export function SlideNotesPanel({ isOpen, onClose }: SlideNotesPanelProps) {
  const {
    currentNotes,
    currentSlide,
    notesPreferences,
    setNotesPosition,
    setNotesDisplayMode,
    setNotesPopout,
    isMobile,
  } = useSlideDeck();
  const { position, displayMode } = notesPreferences;

  // Toggle notes position between right and bottom.
  const toggleNotesPosition = useCallback(() => {
    setNotesPosition(position === 'right' ? 'bottom' : 'right');
  }, [position, setNotesPosition]);

  // Toggle notes display mode between overlay and shrink.
  const toggleNotesDisplayMode = useCallback(() => {
    setNotesDisplayMode(displayMode === 'overlay' ? 'shrink' : 'overlay');
  }, [displayMode, setNotesDisplayMode]);

  // Toggle popout mode.
  const toggleNotesPopout = useCallback(() => {
    setNotesPopout(true);
  }, [setNotesPopout]);

  // Animation variants based on position.
  const panelVariants = {
    right: {
      initial: { x: '100%' },
      animate: { x: 0 },
      exit: { x: '100%' },
    },
    bottom: {
      initial: { y: '100%' },
      animate: { y: 0 },
      exit: { y: '100%' },
    },
  };

  const variant = panelVariants[position];
  const showBackdrop = displayMode === 'overlay';
  const panelClassName = `slide-notes slide-notes--${position}`;

  return (
    <AnimatePresence>
      {isOpen && (
        <>
          {/* Backdrop - only shown in overlay mode */}
          {showBackdrop && (
            <motion.div
              className="slide-notes__backdrop"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
              onClick={onClose}
            />
          )}

          {/* Notes panel */}
          <motion.div
            className={panelClassName}
            initial={variant.initial}
            animate={variant.animate}
            exit={variant.exit}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
          >
            <div className="slide-notes__header">
              <div className="slide-notes__header-left">
                <RiSpeakLine className="slide-notes__icon" />
                <h2 className="slide-notes__title">Speaker Notes</h2>
              </div>
              <div className="slide-notes__header-controls">
                {/* Position toggle */}
                <Tooltip content={position === 'right' ? 'Move to Bottom' : 'Move to Right'} position="bottom">
                  <button
                    className="slide-notes__control-btn"
                    onClick={toggleNotesPosition}
                    aria-label={position === 'right' ? 'Move notes to bottom' : 'Move notes to right'}
                  >
                    {position === 'right' ? <RiLayoutBottomLine /> : <RiLayoutRightLine />}
                  </button>
                </Tooltip>

                {/* Display mode toggle */}
                <Tooltip content={displayMode === 'overlay' ? 'Shrink Slides' : 'Overlay on Slides'} position="bottom">
                  <button
                    className="slide-notes__control-btn"
                    onClick={toggleNotesDisplayMode}
                    aria-label={displayMode === 'overlay' ? 'Shrink slides for notes' : 'Overlay notes on slides'}
                  >
                    {displayMode === 'overlay' ? <RiSplitCellsHorizontal /> : <RiStackLine />}
                  </button>
                </Tooltip>

                {/* Popout button - desktop only */}
                {!isMobile && (
                  <Tooltip content="Pop Out Notes" position="bottom">
                    <button
                      className="slide-notes__control-btn"
                      onClick={toggleNotesPopout}
                      aria-label="Pop out notes to separate window"
                    >
                      <RiExternalLinkLine />
                    </button>
                  </Tooltip>
                )}

                {/* Close button */}
                <button
                  className="slide-notes__close"
                  onClick={onClose}
                  aria-label="Close notes panel"
                >
                  <RiCloseLine />
                </button>
              </div>
            </div>

            <div className="slide-notes__content">
              {currentNotes ? (
                <div className="slide-notes__text">
                  {currentNotes}
                </div>
              ) : (
                <div className="slide-notes__empty">
                  <p>No speaker notes for slide {currentSlide}.</p>
                </div>
              )}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}

export default SlideNotesPanel;
