import React, { useEffect, useRef } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiCloseLine, RiBookOpenLine, RiMegaphoneLine } from 'react-icons/ri';
import styles from './styles.module.css';
import { renderInlineMarkdown } from './utils';
import type { Milestone } from './MilestoneList';

interface MilestoneDrawerProps {
  milestone: Milestone | undefined;
  isOpen: boolean;
  onClose: () => void;
}

const statusClassMap: Record<string, string> = {
  shipped: 'drawerMetaStatusShipped',
  'in-progress': 'drawerMetaStatusInprogress',
  planned: 'drawerMetaStatusPlanned',
};

export default function MilestoneDrawer({
  milestone,
  isOpen,
  onClose,
}: MilestoneDrawerProps): JSX.Element | null {
  const drawerRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previousActiveElement = useRef<HTMLElement | null>(null);

  // Focus management: save previous focus and focus close button when drawer opens.
  useEffect(() => {
    if (isOpen) {
      previousActiveElement.current = document.activeElement as HTMLElement;
      // Focus the close button after animation starts.
      setTimeout(() => closeButtonRef.current?.focus(), 100);
    } else if (previousActiveElement.current) {
      // Return focus to triggering element on close.
      previousActiveElement.current.focus();
      previousActiveElement.current = null;
    }
  }, [isOpen]);

  // Close on escape key.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  // Close on click outside.
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (
        isOpen &&
        drawerRef.current &&
        !drawerRef.current.contains(e.target as Node)
      ) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen, onClose]);

  // Prevent body scroll when drawer is open.
  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [isOpen]);

  return (
    <AnimatePresence>
      {isOpen && milestone && (
        <>
          {/* Backdrop */}
          <motion.div
            className={styles.drawerBackdrop}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            onClick={onClose}
          />

          {/* Drawer */}
          <motion.div
            ref={drawerRef}
            className={styles.drawer}
            role="dialog"
            aria-modal="true"
            aria-labelledby="milestone-drawer-title"
            initial={{ x: '100%' }}
            animate={{ x: 0 }}
            exit={{ x: '100%' }}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
          >
            <div className={styles.drawerHeader}>
              <h3 id="milestone-drawer-title" className={styles.drawerTitle}>
                {renderInlineMarkdown(milestone.label)}
              </h3>
              <button
                ref={closeButtonRef}
                className={styles.drawerClose}
                onClick={onClose}
                aria-label="Close drawer"
              >
                <RiCloseLine />
              </button>
            </div>

            <div className={styles.drawerContent}>
              {milestone.description && (
                <p className={styles.drawerDescription}>
                  {milestone.description}
                </p>
              )}

              {milestone.benefits && (
                <div className={styles.drawerBenefits}>
                  <h4 className={styles.drawerBenefitsTitle}>Why It Matters</h4>
                  <p className={styles.drawerBenefitsText}>{milestone.benefits}</p>
                </div>
              )}

              {milestone.screenshot && (
                <div className={styles.drawerScreenshot}>
                  <img
                    src={milestone.screenshot}
                    alt={`Screenshot for ${milestone.label}`}
                    loading="lazy"
                  />
                </div>
              )}

              {milestone.codeExample && (
                <div className={styles.drawerCodeExample}>
                  <code>{milestone.codeExample}</code>
                </div>
              )}

              <div className={styles.drawerLinks}>
                {milestone.changelog && (
                  <Link
                    to={`/changelog/${milestone.changelog}`}
                    className={styles.drawerLinkButton}
                  >
                    <RiMegaphoneLine />
                    <span>View Announcement</span>
                  </Link>
                )}
                {milestone.docs && (
                  <Link
                    to={milestone.docs}
                    className={`${styles.drawerLinkButton} ${styles.drawerLinkButtonDocs}`}
                  >
                    <RiBookOpenLine />
                    <span>Read Documentation</span>
                  </Link>
                )}
              </div>

              <div className={styles.drawerMeta}>
                <span className={styles.drawerMetaItem}>
                  {milestone.quarter.replace('q', 'Q').replace('-', ' ')}
                </span>
                {milestone.version && (
                  <span className={styles.drawerMetaVersion}>
                    {milestone.version}
                  </span>
                )}
                <span
                  className={`${styles.drawerMetaStatus} ${styles[statusClassMap[milestone.status] || 'drawerMetaStatusPlanned']}`}
                >
                  {milestone.status === 'shipped'
                    ? 'Shipped'
                    : milestone.status === 'in-progress'
                      ? 'In Progress'
                      : 'Planned'}
                </span>
              </div>
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}
