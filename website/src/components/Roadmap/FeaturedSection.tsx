import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import * as Icons from 'react-icons/ri';
import { RiExternalLinkLine, RiBookOpenLine, RiMegaphoneLine, RiGitPullRequestLine, RiFileTextLine } from 'react-icons/ri';
import styles from './styles.module.css';

interface FeaturedItem {
  id: string;
  icon: string;
  title: string;
  tagline: string;
  description: string;
  status: 'shipped' | 'in-progress' | 'planned';
  quarter: string;
  docs?: string;
  changelog?: string;
  pr?: number;
  prd?: string;
}

interface FeaturedSectionProps {
  items: FeaturedItem[];
}

const statusConfig = {
  shipped: { label: 'Shipped', className: 'featuredStatusShipped' },
  'in-progress': { label: 'In Progress', className: 'featuredStatusInProgress' },
  planned: { label: 'Planned', className: 'featuredStatusPlanned' },
};

// Sort order: shipped first, then in-progress, then planned.
const statusOrder: Record<string, number> = {
  shipped: 0,
  'in-progress': 1,
  planned: 2,
};

// Parse quarter string (e.g., "q1-2026") into a sortable number.
const parseQuarter = (quarter: string): number => {
  const match = quarter.match(/q(\d)-(\d{4})/);
  if (!match) return 0;
  const [, q, year] = match;
  return parseInt(year, 10) * 10 + parseInt(q, 10);
};

export default function FeaturedSection({ items }: FeaturedSectionProps): JSX.Element {
  // Sort items by status (shipped first), then by quarter (earlier first).
  const sortedItems = [...items].sort((a, b) => {
    const statusDiff = statusOrder[a.status] - statusOrder[b.status];
    if (statusDiff !== 0) return statusDiff;
    return parseQuarter(a.quarter) - parseQuarter(b.quarter);
  });

  return (
    <section className={styles.featuredSection}>
      <h2 className={styles.sectionTitle}>Featured Improvements</h2>
      <p className={styles.sectionDescription}>
        Major capabilities that transform how you work with infrastructure.
      </p>
      <div className={styles.featuredGrid}>
        {sortedItems.map((item, index) => {
          const IconComponent = (Icons as Record<string, React.ComponentType<{ className?: string }>>)[
            item.icon
          ] || Icons.RiQuestionLine;
          const config = statusConfig[item.status];

          return (
            <motion.div
              key={item.id}
              className={styles.featuredCard}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.4, delay: index * 0.1 }}
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
                  {item.changelog && (
                    <Link
                      to={`/changelog/${item.changelog}`}
                      className={styles.featuredLink}
                      title="View Announcement"
                    >
                      <RiMegaphoneLine />
                    </Link>
                  )}
                  {item.docs && (
                    <Link
                      to={item.docs}
                      className={styles.featuredLink}
                      title="View Documentation"
                    >
                      <RiBookOpenLine />
                    </Link>
                  )}
                  {item.prd && (
                    <Link
                      to={`https://github.com/cloudposse/atmos/blob/main/docs/prd/${item.prd}.md`}
                      className={styles.featuredLink}
                      title="View PRD"
                    >
                      <RiFileTextLine />
                    </Link>
                  )}
                  {item.pr && (
                    <Link
                      to={`https://github.com/cloudposse/atmos/pull/${item.pr}`}
                      className={styles.featuredLink}
                      title={`View PR #${item.pr}`}
                    >
                      <RiGitPullRequestLine />
                    </Link>
                  )}
                </div>
              </div>
            </motion.div>
          );
        })}
      </div>
    </section>
  );
}
