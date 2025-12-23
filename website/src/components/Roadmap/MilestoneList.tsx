import React from 'react';
import Link from '@docusaurus/Link';
import { RiCheckLine, RiLoader4Line, RiCalendarTodoLine } from 'react-icons/ri';
import styles from './styles.module.css';
import { renderInlineMarkdown } from './utils';

export interface Milestone {
  label: string;
  status: 'shipped' | 'in-progress' | 'planned';
  quarter: string;
  /** GitHub PR number (optional). */
  pr?: number;
  /** Changelog slug (optional) - links to /changelog/{slug}. */
  changelog?: string;
  /** Documentation path (optional) - links to docs page. */
  docs?: string;
  /** Short description shown in the detail drawer. */
  description?: string;
  /** Screenshot image path (optional) - shown in the detail drawer. */
  screenshot?: string;
  /** Code example (optional) - shown in the detail drawer. */
  codeExample?: string;
}

interface MilestoneListProps {
  milestones: Milestone[];
  showQuarter?: boolean;
  /** Callback when a milestone with a description is clicked. */
  onMilestoneClick?: (milestone: Milestone) => void;
}

const statusConfig = {
  shipped: {
    icon: RiCheckLine,
    label: 'Shipped',
    className: 'statusShipped',
  },
  'in-progress': {
    icon: RiLoader4Line,
    label: 'In Progress',
    className: 'statusInProgress',
  },
  planned: {
    icon: RiCalendarTodoLine,
    label: 'Planned',
    className: 'statusPlanned',
  },
};

export default function MilestoneList({
  milestones,
  showQuarter = true,
  onMilestoneClick,
}: MilestoneListProps): JSX.Element {
  // Get the primary link for the milestone label (announcement first, then docs).
  const getMilestoneLabelLink = (milestone: Milestone): string | null => {
    if (milestone.changelog) return `/changelog/${milestone.changelog}`;
    if (milestone.docs) return milestone.docs;
    return null;
  };

  // Check if milestone has rich content worth showing in drawer.
  const hasRichContent = (milestone: Milestone): boolean => {
    return Boolean(milestone.description || milestone.screenshot || milestone.codeExample);
  };

  // Render badges on the right side (Announcement, Docs, PRs).
  const renderMilestoneLinks = (milestone: Milestone) => {
    if (!milestone.changelog && !milestone.docs && !milestone.pr) return null;

    return (
      <span className={styles.milestoneLinks}>
        {milestone.changelog && (
          <Link
            to={`/changelog/${milestone.changelog}`}
            className={styles.milestoneLinkBadge}
            onClick={(e) => e.stopPropagation()}
            title="View announcement"
          >
            Announcement
          </Link>
        )}
        {milestone.docs && (
          <Link
            to={milestone.docs}
            className={`${styles.milestoneLinkBadge} ${styles.milestoneLinkDocs}`}
            onClick={(e) => e.stopPropagation()}
            title="View documentation"
          >
            Docs
          </Link>
        )}
        {milestone.pr && (
          <Link
            to={`https://github.com/cloudposse/atmos/pull/${milestone.pr}`}
            className={styles.milestoneLinkBadge}
            onClick={(e) => e.stopPropagation()}
            target="_blank"
            rel="noopener noreferrer"
            title="View pull request"
          >
            PR #{milestone.pr}
          </Link>
        )}
      </span>
    );
  };

  // Sort milestones: shipped first, then in-progress, then planned.
  const sortedMilestones = [...milestones].sort((a, b) => {
    const order = { shipped: 0, 'in-progress': 1, planned: 2 };
    return order[a.status] - order[b.status];
  });

  return (
    <div className={styles.milestoneList}>
      <ul className={styles.milestoneItems}>
        {sortedMilestones.map((milestone, index) => {
          const config = statusConfig[milestone.status];
          const Icon = config.icon;
          const labelLink = getMilestoneLabelLink(milestone);
          const isClickable = hasRichContent(milestone) && onMilestoneClick;

          const handleClick = () => {
            if (isClickable) {
              onMilestoneClick(milestone);
            }
          };

          const handleKeyDown = (e: React.KeyboardEvent) => {
            if (isClickable && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault();
              onMilestoneClick(milestone);
            }
          };

          return (
            <li
              key={index}
              className={`${styles.milestoneItem} ${isClickable ? styles.milestoneClickable : ''}`}
              onClick={handleClick}
              onKeyDown={handleKeyDown}
              role={isClickable ? 'button' : undefined}
              tabIndex={isClickable ? 0 : undefined}
              title={milestone.description ? 'Click for details' : undefined}
            >
              <span className={`${styles.statusBadge} ${styles[config.className]}`}>
                <Icon className={styles.statusIcon} />
              </span>
              {labelLink && !isClickable ? (
                <Link
                  to={labelLink}
                  className={styles.milestoneLabelLink}
                  onClick={(e) => e.stopPropagation()}
                >
                  {renderInlineMarkdown(milestone.label)}
                </Link>
              ) : (
                <span className={styles.milestoneLabel}>
                  {renderInlineMarkdown(milestone.label)}
                </span>
              )}
              <div className={styles.milestoneRight}>
                {renderMilestoneLinks(milestone)}
                {showQuarter && (
                  <span className={styles.milestoneQuarter}>
                    {milestone.quarter.replace('q', 'Q').replace('-', ' ')}
                  </span>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
