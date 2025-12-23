import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import * as Icons from 'react-icons/ri';
import { RiExternalLinkLine, RiBookOpenLine, RiMegaphoneLine, RiGitPullRequestLine } from 'react-icons/ri';
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
}

interface FeaturedSectionProps {
  items: FeaturedItem[];
}

const statusConfig = {
  shipped: { label: 'Shipped', className: 'featuredStatusShipped' },
  'in-progress': { label: 'In Progress', className: 'featuredStatusInProgress' },
  planned: { label: 'Planned', className: 'featuredStatusPlanned' },
};

export default function FeaturedSection({ items }: FeaturedSectionProps): JSX.Element {
  return (
    <section className={styles.featuredSection}>
      <h2 className={styles.sectionTitle}>Featured Improvements</h2>
      <p className={styles.sectionDescription}>
        Major capabilities that transform how you work with infrastructure.
      </p>
      <div className={styles.featuredGrid}>
        {items.map((item, index) => {
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
