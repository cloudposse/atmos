import React, { Children, isValidElement, ReactNode } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { RiCloseLine } from 'react-icons/ri';
import { useSlideDeck } from './SlideDeckContext';
import './SlideDrawer.css';

export interface SlideDrawerProps {
  children: ReactNode;
  isOpen: boolean;
  onClose: () => void;
}

export function SlideDrawer({ children, isOpen, onClose }: SlideDrawerProps) {
  const { currentSlide, goToSlide } = useSlideDeck();

  // Convert children to array of slides.
  const slides = Children.toArray(children).filter(isValidElement);

  const handleSlideClick = (index: number) => {
    goToSlide(index);
    onClose();
  };

  return (
    <AnimatePresence>
      {isOpen && (
        <>
          {/* Backdrop */}
          <motion.div
            className="slide-drawer__backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            onClick={onClose}
          />

          {/* Drawer panel */}
          <motion.div
            className="slide-drawer"
            initial={{ x: '-100%' }}
            animate={{ x: 0 }}
            exit={{ x: '-100%' }}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
          >
            <div className="slide-drawer__header">
              <h2 className="slide-drawer__title">Slides</h2>
              <button
                className="slide-drawer__close"
                onClick={onClose}
                aria-label="Close drawer"
              >
                <RiCloseLine />
              </button>
            </div>

            <div className="slide-drawer__content">
              {slides.map((slide, index) => {
                const slideNumber = index + 1;
                const isActive = slideNumber === currentSlide;

                return (
                  <button
                    key={index}
                    className={`slide-drawer__thumbnail ${isActive ? 'slide-drawer__thumbnail--active' : ''}`}
                    onClick={() => handleSlideClick(slideNumber)}
                    aria-label={`Go to slide ${slideNumber}`}
                    aria-current={isActive ? 'true' : undefined}
                  >
                    <div className="slide-drawer__thumbnail-number">
                      {slideNumber}
                    </div>
                    <div className="slide-drawer__thumbnail-preview">
                      <div className="slide-drawer__thumbnail-content">
                        {slide}
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}

export default SlideDrawer;
