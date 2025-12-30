import React from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { RiCloseLine, RiSpeakLine } from 'react-icons/ri';
import { useSlideDeck } from './SlideDeckContext';
import type { SlideNotesPanelProps } from './types';
import './SlideNotes.css';

/**
 * SlideNotesPanel - A slide-out panel displaying speaker notes.
 *
 * Slides in from the right side of the screen when the user presses 'N'.
 * Displays the notes content registered by SlideNotes components.
 */
export function SlideNotesPanel({ isOpen, onClose }: SlideNotesPanelProps) {
  const { currentNotes, currentSlide } = useSlideDeck();

  return (
    <AnimatePresence>
      {isOpen && (
        <>
          {/* Backdrop */}
          <motion.div
            className="slide-notes__backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            onClick={onClose}
          />

          {/* Notes panel */}
          <motion.div
            className="slide-notes"
            initial={{ x: '100%' }}
            animate={{ x: 0 }}
            exit={{ x: '100%' }}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
          >
            <div className="slide-notes__header">
              <div className="slide-notes__header-left">
                <RiSpeakLine className="slide-notes__icon" />
                <h2 className="slide-notes__title">Speaker Notes</h2>
              </div>
              <button
                className="slide-notes__close"
                onClick={onClose}
                aria-label="Close notes panel"
              >
                <RiCloseLine />
              </button>
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
