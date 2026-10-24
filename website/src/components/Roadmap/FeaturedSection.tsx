import React, { useState } from 'react';
import Link from '@docusaurus/Link';
import * as Icons from 'react-icons/ri';
import { RiArrowLeftSLine, RiArrowRightSLine, RiBookOpenLine, RiMegaphoneLine, RiGitPullRequestLine, RiFileTextLine, RiFlaskLine } from 'react-icons/ri';
import FeaturedDrawer, { FeaturedItem } from './FeaturedDrawer';
import styles from './styles.module.css';

interface FeaturedSectionProps {
  items: FeaturedItem[];
}

const statusConfig = {
  shipped: { label: 'Shipped', className: 'featuredStatusShipped' },
  'in-progress': { label: 'In Progress', className: 'featuredStatusInProgress' },
  planned: { label: 'Planned', className: 'featuredStatusPlanned' },
};

const PAGE_SIZE = 6;

/** Page through curated highlights in their configured order and open feature details. */
export default function FeaturedSection({ items }: FeaturedSectionProps): JSX.Element {
  const [selectedItem, setSelectedItem] = useState<FeaturedItem | undefined>(undefined);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  const [page, setPage] = useState(0);
  // The data is curated newest first. Pagination preserves that editorial order.
  const pageCount = Math.ceil(items.length / PAGE_SIZE);
  const visibleItems = items.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  const handleCardClick = (item: FeaturedItem) => {
    setSelectedItem(item);
    setIsDrawerOpen(true);
  };

  const handleDrawerClose = () => {
    setIsDrawerOpen(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent, item: FeaturedItem) => {
    if (e.target !== e.currentTarget) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      handleCardClick(item);
    }
  };

  return (
    <section className={styles.featuredSection}>
      <h2 className={styles.sectionTitle}>Featured Improvements</h2>
      <p className={styles.sectionDescription}>
        Major capabilities that transform how you work with infrastructure.
      </p>
      {pageCount > 1 && (
        <nav className={styles.featuredPagination} aria-label="Featured improvements pages">
          <button
            type="button"
            className={styles.featuredPageButton}
            disabled={page === 0}
            aria-controls="featured-improvements"
            onClick={() => setPage((current) => Math.max(0, current - 1))}
          >
            <RiArrowLeftSLine aria-hidden="true" /> Previous
          </button>
          <span className={styles.featuredPageStatus} role="status">
            {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, items.length)} of {items.length}
          </span>
          <button
            type="button"
            className={styles.featuredPageButton}
            disabled={page === pageCount - 1}
            aria-controls="featured-improvements"
            onClick={() => setPage((current) => Math.min(pageCount - 1, current + 1))}
          >
            Next <RiArrowRightSLine aria-hidden="true" />
          </button>
        </nav>
      )}
      <div id="featured-improvements" className={styles.featuredGrid}>
        {visibleItems.map((item) => {
          const IconComponent = (Icons as Record<string, React.ComponentType<{ className?: string }>>)[
            item.icon
          ] || Icons.RiQuestionLine;
          const config = statusConfig[item.status];

          return (
            <div
              key={item.id}
              className={`${styles.featuredCard} ${styles.featuredCardClickable}`}
              onClick={() => handleCardClick(item)}
              onKeyDown={(e) => handleKeyDown(e, item)}
              role="button"
              tabIndex={0}
              title="Click for details"
            >
              <div className={styles.featuredHeader}>
                <div className={styles.featuredIconWrapper}>
                  <IconComponent className={styles.featuredIcon} />
                </div>
                <span className={`${styles.featuredStatus} ${styles[config.className]}`}>
                  {config.label}
                </span>
              </div>

              <h3 className={styles.featuredTitle}>{item.title}</h3>
              <p className={styles.featuredTagline}>{item.tagline}</p>
              <p className={styles.featuredDescription}>{item.description}</p>

              <div className={styles.featuredFooter}>
                <span className={styles.featuredQuarter}>
                  {item.quarter.replace('q', 'Q').replace('-', ' ')}
                </span>
                <div className={styles.featuredLinks}>
                  {item.experimental && (
                    <Link
                      to="/experimental"
                      className={`${styles.featuredLink} ${styles.featuredLinkExperimental}`}
                      title="This feature is experimental"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <RiFlaskLine />
                    </Link>
                  )}
                  {item.changelog && (
                    <Link
                      to={`/changelog/${item.changelog}`}
                      className={styles.featuredLink}
                      title="View Announcement"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <RiMegaphoneLine />
                    </Link>
                  )}
                  {item.docs && (
                    <Link
                      to={item.docs}
                      className={styles.featuredLink}
                      title="View Documentation"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <RiBookOpenLine />
                    </Link>
                  )}
                  {item.prd && (
                    <Link
                      to={`https://github.com/cloudposse/atmos/blob/main/docs/prd/${item.prd}.md`}
                      className={styles.featuredLink}
                      title="View PRD"
                      onClick={(e) => e.stopPropagation()}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <RiFileTextLine />
                    </Link>
                  )}
                  {item.pr && (
                    <Link
                      to={`https://github.com/cloudposse/atmos/pull/${item.pr}`}
                      className={styles.featuredLink}
                      title={`View PR #${item.pr}`}
                      onClick={(e) => e.stopPropagation()}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <RiGitPullRequestLine />
                    </Link>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>

      <FeaturedDrawer
        item={selectedItem}
        isOpen={isDrawerOpen}
        onClose={handleDrawerClose}
      />
    </section>
  );
}
