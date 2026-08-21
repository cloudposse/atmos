import React, { useEffect, useCallback, useRef } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { RiCloseLine, RiArrowRightLine } from 'react-icons/ri';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import Link from '@docusaurus/Link';
import type { CapabilityInfo } from '@site/src/data/tools';
import './index.css';

interface CapabilityDrawerProps {
  capability: CapabilityInfo | null;
  isOpen: boolean;
  onClose: () => void;
}

const CapabilityDrawer: React.FC<CapabilityDrawerProps> = ({ capability, isOpen, onClose }) => {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previousActiveElement = useRef<HTMLElement | null>(null);

  const handleEscape = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose]
  );

  useEffect(() => {
    if (isOpen) {
      // Store the previously focused element.
      previousActiveElement.current = document.activeElement as HTMLElement;
      document.addEventListener('keydown', handleEscape);
      document.body.style.overflow = 'hidden';
      // Focus the close button when drawer opens.
      setTimeout(() => closeButtonRef.current?.focus(), 0);
    }
    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.body.style.overflow = '';
      // Restore focus when drawer closes.
      if (!isOpen && previousActiveElement.current) {
        previousActiveElement.current.focus();
      }
    };
  }, [isOpen, handleEscape]);

  return (
    <AnimatePresence>
      {isOpen && capability && (
        <>
          <motion.div
            className="capability-drawer-backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            onClick={onClose}
            aria-hidden="true"
          />
          <motion.aside
            className="capability-drawer"
            initial={{ x: '100%' }}
            animate={{ x: 0 }}
            exit={{ x: '100%' }}
            transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            role="dialog"
            aria-modal="true"
            aria-labelledby="capability-drawer-title"
          >
            <div className="capability-drawer__header">
              <button ref={closeButtonRef} className="capability-drawer__close" onClick={onClose} aria-label="Close drawer">
                <RiCloseLine />
              </button>
            </div>

            <div className="capability-drawer__content">
              <h2 id="capability-drawer-title" className="capability-drawer__title">
                {capability.title}
              </h2>

              <section className="capability-drawer__section">
                <h3>What is it?</h3>
                <div className="capability-drawer__text">
                  <Markdown remarkPlugins={[remarkGfm]}>{capability.description}</Markdown>
                </div>
              </section>

              <section className="capability-drawer__section">
                <h3>Why it matters</h3>
                <div className="capability-drawer__text capability-drawer__text--highlight">
                  <Markdown remarkPlugins={[remarkGfm]}>{capability.whyItMatters}</Markdown>
                </div>
              </section>

              <section className="capability-drawer__section">
                <h3>How Atmos helps</h3>
                <div className="capability-drawer__text capability-drawer__text--atmos">
                  <Markdown remarkPlugins={[remarkGfm]}>{capability.atmosSupport}</Markdown>
                </div>
              </section>

              <Link to={capability.docsLink} className="capability-drawer__cta">
                Learn More <RiArrowRightLine />
              </Link>
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
};

export default CapabilityDrawer;
