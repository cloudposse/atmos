import React, { useEffect, useRef } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiCloseLine, RiBookOpenLine, RiMegaphoneLine } from 'react-icons/ri';
import styles from './styles.module.css';
import { renderInlineMarkdown } from './utils';
import type { Milestone } from './MilestoneList';

interface MilestoneDrawerProps {
  milestone: Milestone | null;
  isOpen: boolean;
  onClose: () => void;
}

export default function MilestoneDrawer({
  milestone,
  isOpen,
  onClose,
}: MilestoneDrawerProps): JSX.Element | null {
  const drawerRef = useRef<HTMLDivElement>(null);

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
            initial={{ x: '100%' }}
            animate={{ x: 0 }}
            exit={{ x: '100%' }}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
          >
            <div className={styles.drawerHeader}>
              <h3 className={styles.drawerTitle}>
                {renderInlineMarkdown(milestone.label)}
              </h3>
              <button
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
                <span
                  className={`${styles.drawerMetaStatus} ${styles[`drawerMetaStatus${milestone.status.charAt(0).toUpperCase() + milestone.status.slice(1).replace('-', '')}`]}`}
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
