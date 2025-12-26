import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiArrowDownSLine, RiGithubLine, RiGitPullRequestLine } from 'react-icons/ri';
import * as Icons from 'react-icons/ri';
import ProgressBar from './ProgressBar';
import MilestoneList, { Milestone } from './MilestoneList';
import MilestoneDrawer from './MilestoneDrawer';
import Tooltip from './Tooltip';
import styles from './styles.module.css';
import { renderInlineMarkdown } from './utils';

/** PR reference with number and title for hover tooltips. */
export interface PRReference {
  number: number;
  title: string;
}

interface Initiative {
  id: string;
  icon: string;
  title: string;
  tagline: string;
  description: string;
  progress: number;
  status: 'completed' | 'in-progress' | 'planned';
  milestones: Milestone[];
  issues: number[];
  prs: PRReference[];
  changelogSlugs?: string[];
}

interface InitiativeCardProps {
  initiative: Initiative;
  index?: number;
  /** When true, expand all collapsible milestone sections. */
  expandAllMilestones?: boolean;
}

export default function InitiativeCard({
  initiative,
  index = 0,
  expandAllMilestones = false,
}: InitiativeCardProps): JSX.Element {
  const [isExpanded, setIsExpanded] = useState(false);
  const [selectedMilestone, setSelectedMilestone] = useState<Milestone | undefined>(undefined);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  // Expand the initiative card when expandAllMilestones is true.
  useEffect(() => {
    if (expandAllMilestones) {
      setIsExpanded(true);
    }
  }, [expandAllMilestones]);

  // Dynamically get the icon component.
  const IconComponent = (Icons as Record<string, React.ComponentType<{ className?: string }>>)[
    initiative.icon
  ] || Icons.RiQuestionLine;

  const handleMilestoneClick = (milestone: Milestone) => {
    setSelectedMilestone(milestone);
    setIsDrawerOpen(true);
  };

  const handleDrawerClose = () => {
    setIsDrawerOpen(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      setIsExpanded(!isExpanded);
    }
  };

  return (
    <motion.div
      id={initiative.id}
      className={styles.initiativeCardWrapper}
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: '-50px' }}
      transition={{ duration: 0.5, delay: index * 0.1, ease: 'easeOut' }}
    >
      <div
        className={`${styles.initiativeCard} ${isExpanded ? styles.initiativeCardExpanded : ''}`}
      >
        <div
          className={styles.initiativeHeader}
          onClick={() => setIsExpanded(!isExpanded)}
          onKeyDown={handleKeyDown}
          role="button"
          tabIndex={0}
          aria-expanded={isExpanded}
        >
          <div className={styles.initiativeIcon}>
            <IconComponent />
          </div>
          <div className={styles.initiativeTitleGroup}>
            <h3 className={styles.initiativeTitle}>{initiative.title}</h3>
            <p className={styles.initiativeTagline}>{initiative.tagline}</p>
          </div>
          <div className={styles.initiativeHeaderRight}>
            <ProgressBar progress={initiative.progress} size="small" />
            <motion.div
              className={styles.initiativeExpandIcon}
              animate={{ rotate: isExpanded ? 180 : 0 }}
              transition={{ duration: 0.2 }}
            >
              <RiArrowDownSLine />
            </motion.div>
          </div>
        </div>

        <AnimatePresence>
          {isExpanded && (
            <motion.div
              className={styles.initiativeContent}
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.3, ease: 'easeInOut' }}
              onClick={(e) => e.stopPropagation()}
            >
              <p className={styles.initiativeDescription}>{renderInlineMarkdown(initiative.description)}</p>

              <MilestoneList
                milestones={initiative.milestones}
                onMilestoneClick={handleMilestoneClick}
                grouped
                expandAll={expandAllMilestones}
              />

              {(initiative.issues.length > 0 || initiative.prs?.length > 0) && (
                <div className={styles.initiativeLinksContainer}>
                  {initiative.issues.length > 0 && (
                    <div className={styles.initiativeLinks}>
                      <span className={styles.initiativeLinksLabel}>
                        <RiGithubLine /> Issues:
                      </span>
                      <div className={styles.initiativeIssues}>
                        {initiative.issues.map((issue) => (
                          <Link
                            key={issue}
                            to={`https://github.com/cloudposse/atmos/issues/${issue}`}
                            className={styles.initiativeIssueLink}
                            onClick={(e) => e.stopPropagation()}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            #{issue}
                          </Link>
                        ))}
                      </div>
                    </div>
                  )}
                  {initiative.prs?.length > 0 && (
                    <div className={styles.initiativeLinks}>
                      <span className={styles.initiativeLinksLabel}>
                        <RiGitPullRequestLine /> Pull Requests:
                      </span>
                      <div className={styles.initiativeIssues}>
                        {initiative.prs.map((pr) => (
                          <Tooltip key={pr.number} content={pr.title}>
                            <Link
                              to={`https://github.com/cloudposse/atmos/pull/${pr.number}`}
                              className={styles.initiativePrLink}
                              onClick={(e) => e.stopPropagation()}
                              target="_blank"
                              rel="noopener noreferrer"
                            >
                              #{pr.number}
                            </Link>
                          </Tooltip>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </div>

      <MilestoneDrawer
        milestone={selectedMilestone}
        isOpen={isDrawerOpen}
        onClose={handleDrawerClose}
      />
    </motion.div>
  );
}
