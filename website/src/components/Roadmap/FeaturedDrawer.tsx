import React, { useEffect, useRef } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiCloseLine, RiBookOpenLine, RiMegaphoneLine, RiGitPullRequestLine, RiFileTextLine } from 'react-icons/ri';
import * as Icons from 'react-icons/ri';
import styles from './styles.module.css';

export interface FeaturedItem {
  id: string;
  icon: string;
  title: string;
  tagline: string;
  description: string;
  benefits?: string;
  status: 'shipped' | 'in-progress' | 'planned';
  quarter: string;
  /** Version this was released in (optional, typically for shipped items). */
  version?: string;
  docs?: string;
  changelog?: string;
  pr?: number;
  prd?: string;
  /** Whether this feature is experimental (still being refined). */
  experimental?: boolean;
}

interface FeaturedDrawerProps {
  item: FeaturedItem | undefined;
  isOpen: boolean;
  onClose: () => void;
}

const statusClassMap: Record<string, string> = {
  shipped: 'drawerMetaStatusShipped',
  'in-progress': 'drawerMetaStatusInprogress',
  planned: 'drawerMetaStatusPlanned',
};

export default function FeaturedDrawer({
  item,
  isOpen,
  onClose,
}: FeaturedDrawerProps): JSX.Element | null {
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

  // Get the icon component.
  const IconComponent = item
    ? (Icons as Record<string, React.ComponentType<{ className?: string }>>)[item.icon] || Icons.RiQuestionLine
    : Icons.RiQuestionLine;

  return (
    <AnimatePresence>
      {isOpen && item && (
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
              <div className={styles.featuredDrawerTitleGroup}>
                <div className={styles.featuredDrawerIcon}>
                  <IconComponent />
                </div>
                <h3 className={styles.drawerTitle}>{item.title}</h3>
              </div>
              <button
                className={styles.drawerClose}
                onClick={onClose}
                aria-label="Close drawer"
              >
                <RiCloseLine />
              </button>
            </div>

            <div className={styles.drawerContent}>
              <p className={styles.featuredDrawerTagline}>{item.tagline}</p>

              {item.description && (
                <p className={styles.drawerDescription}>
                  {item.description}
                </p>
              )}

              {item.benefits && (
                <div className={styles.drawerBenefits}>
                  <h4 className={styles.drawerBenefitsTitle}>Why It Matters</h4>
                  <p className={styles.drawerBenefitsText}>{item.benefits}</p>
                </div>
              )}

              <div className={styles.drawerLinks}>
                {item.changelog && (
                  <Link
                    to={`/changelog/${item.changelog}`}
                    className={styles.drawerLinkButton}
                  >
                    <RiMegaphoneLine />
                    <span>View Announcement</span>
                  </Link>
                )}
                {item.docs && (
                  <Link
                    to={item.docs}
                    className={`${styles.drawerLinkButton} ${styles.drawerLinkButtonDocs}`}
                  >
                    <RiBookOpenLine />
                    <span>Read Documentation</span>
                  </Link>
                )}
                {item.prd && (
                  <Link
                    to={`https://github.com/cloudposse/atmos/blob/main/docs/prd/${item.prd}.md`}
                    className={styles.drawerLinkButton}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <RiFileTextLine />
                    <span>View PRD</span>
                  </Link>
                )}
                {item.pr && (
                  <Link
                    to={`https://github.com/cloudposse/atmos/pull/${item.pr}`}
                    className={styles.drawerLinkButton}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <RiGitPullRequestLine />
                    <span>View PR #{item.pr}</span>
                  </Link>
                )}
              </div>

              <div className={styles.drawerMeta}>
                <span className={styles.drawerMetaItem}>
                  {item.quarter.replace('q', 'Q').replace('-', ' ')}
                </span>
                {item.version && (
                  <Link
                    to={`https://github.com/cloudposse/atmos/releases/tag/${item.version}`}
                    className={styles.drawerMetaVersion}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {item.version}
                  </Link>
                )}
                <span
                  className={`${styles.drawerMetaStatus} ${styles[statusClassMap[item.status] || 'drawerMetaStatusPlanned']}`}
                >
                  {item.status === 'shipped'
                    ? 'Shipped'
                    : item.status === 'in-progress'
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
