import React, { useState, useRef, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiCheckLine, RiTimeLine, RiCalendarLine, RiArrowLeftSLine, RiArrowRightSLine, RiCloseLine } from 'react-icons/ri';
import styles from './styles.module.css';
import { renderInlineMarkdown } from './utils';

interface Quarter {
  id: string;
  label: string;
  status: 'completed' | 'current' | 'planned';
}

interface Milestone {
  label: string;
  status: 'shipped' | 'in-progress' | 'planned';
  quarter: string;
  pr?: number;
  changelog?: string;
}

interface Initiative {
  id: string;
  title: string;
  icon: string;
  milestones: Milestone[];
}

interface QuarterTimelineProps {
  quarters: Quarter[];
  initiatives?: Initiative[];
}

const statusConfig = {
  completed: {
    icon: RiCheckLine,
    className: 'quarterCompleted',
  },
  current: {
    icon: RiTimeLine,
    className: 'quarterCurrent',
  },
  planned: {
    icon: RiCalendarLine,
    className: 'quarterPlanned',
  },
};

export default function QuarterTimeline({
  quarters,
  initiatives = [],
}: QuarterTimelineProps): JSX.Element {
  const [selectedQuarter, setSelectedQuarter] = useState<string | null>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  // Check scroll position to show/hide arrows.
  const checkScroll = () => {
    const container = scrollContainerRef.current;
    if (container) {
      setCanScrollLeft(container.scrollLeft > 0);
      setCanScrollRight(
        container.scrollLeft < container.scrollWidth - container.clientWidth - 1
      );
    }
  };

  useEffect(() => {
    checkScroll();
    window.addEventListener('resize', checkScroll);
    return () => window.removeEventListener('resize', checkScroll);
  }, []);

  const scroll = (direction: 'left' | 'right') => {
    const container = scrollContainerRef.current;
    if (container) {
      const scrollAmount = 200;
      container.scrollBy({
        left: direction === 'left' ? -scrollAmount : scrollAmount,
        behavior: 'smooth',
      });
    }
  };

  // Get milestones for a specific quarter across all initiatives.
  const getMilestonesForQuarter = (quarterId: string) => {
    const milestones: { initiative: string; initiativeId: string; milestone: Milestone }[] = [];
    initiatives.forEach((initiative) => {
      initiative.milestones
        .filter((m) => m.quarter === quarterId)
        .forEach((milestone) => {
          milestones.push({
            initiative: initiative.title,
            initiativeId: initiative.id,
            milestone,
          });
        });
    });
    return milestones;
  };

  const selectedQuarterData = selectedQuarter
    ? quarters.find((q) => q.id === selectedQuarter)
    : null;
  const quarterMilestones = selectedQuarter
    ? getMilestonesForQuarter(selectedQuarter)
    : [];

  // Group milestones by initiative, then by status within each initiative.
  const milestonesByInitiative = quarterMilestones.reduce((acc, item) => {
    if (!acc[item.initiativeId]) {
      acc[item.initiativeId] = {
        name: item.initiative,
        milestones: [],
      };
    }
    acc[item.initiativeId].milestones.push(item.milestone);
    return acc;
  }, {} as Record<string, { name: string; milestones: Milestone[] }>);

  // Sort milestones within each initiative by status.
  const statusOrder = { shipped: 0, 'in-progress': 1, planned: 2 };
  Object.values(milestonesByInitiative).forEach((group) => {
    group.milestones.sort((a, b) => statusOrder[a.status] - statusOrder[b.status]);
  });

  return (
    <div className={styles.quarterTimeline}>
      <div className={styles.quarterTimelineWrapper}>
        {canScrollLeft && (
          <button
            className={`${styles.scrollButton} ${styles.scrollButtonLeft}`}
            onClick={() => scroll('left')}
            aria-label="Scroll left"
          >
            <RiArrowLeftSLine />
          </button>
        )}

        <div
          className={styles.quarterTimelineScroll}
          ref={scrollContainerRef}
          onScroll={checkScroll}
        >
          <div className={styles.quarterTimelineTrack}>
            {quarters.map((quarter, index) => {
              const config = statusConfig[quarter.status];
              const Icon = config.icon;
              const isLast = index === quarters.length - 1;
              const isSelected = selectedQuarter === quarter.id;
              const milestoneCount = getMilestonesForQuarter(quarter.id).length;

              return (
                <React.Fragment key={quarter.id}>
                  <motion.button
                    className={`${styles.quarterNode} ${styles[config.className]} ${isSelected ? styles.quarterSelected : ''}`}
                    initial={{ opacity: 0, scale: 0.8 }}
                    whileInView={{ opacity: 1, scale: 1 }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.4, delay: index * 0.1 }}
                    onClick={() => setSelectedQuarter(isSelected ? null : quarter.id)}
                    aria-expanded={isSelected}
                    aria-label={`${quarter.label}: ${milestoneCount} milestones`}
                  >
                    <div className={styles.quarterIcon}>
                      <Icon />
                    </div>
                    <span className={styles.quarterLabel}>{quarter.label}</span>
                    {milestoneCount > 0 && (
                      <span className={styles.quarterMilestoneCount}>{milestoneCount}</span>
                    )}
                  </motion.button>
                  {!isLast && (
                    <div
                      className={`${styles.quarterConnector} ${
                        quarter.status === 'completed' ? styles.quarterConnectorCompleted : ''
                      }`}
                    />
                  )}
                </React.Fragment>
              );
            })}
          </div>
        </div>

        {canScrollRight && (
          <button
            className={`${styles.scrollButton} ${styles.scrollButtonRight}`}
            onClick={() => scroll('right')}
            aria-label="Scroll right"
          >
            <RiArrowRightSLine />
          </button>
        )}
      </div>

      <AnimatePresence>
        {selectedQuarter && selectedQuarterData && (
          <motion.div
            className={styles.quarterDetail}
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.3 }}
          >
            <div className={styles.quarterDetailHeader}>
              <h3 className={styles.quarterDetailTitle}>
                {selectedQuarterData.label}
                <span className={styles.quarterDetailCount}>
                  {quarterMilestones.length} milestone{quarterMilestones.length !== 1 ? 's' : ''}
                </span>
              </h3>
              <button
                className={styles.quarterDetailClose}
                onClick={() => setSelectedQuarter(null)}
                aria-label="Close"
              >
                <RiCloseLine />
              </button>
            </div>

            <div className={styles.quarterDetailContent}>
              {quarterMilestones.length === 0 ? (
                <p className={styles.quarterDetailEmpty}>No milestones scheduled for this quarter.</p>
              ) : (
                <div className={styles.quarterDetailGrid}>
                  {Object.entries(milestonesByInitiative).map(([initiativeId, group]) => (
                    <div key={initiativeId} className={styles.quarterDetailInitiativeGroup}>
                      <h4 className={styles.quarterDetailInitiativeTitle}>{group.name}</h4>
                      <ul className={styles.quarterDetailMilestoneList}>
                        {group.milestones.map((milestone, idx) => {
                          const statusDotClass = {
                            shipped: styles.quarterDetailStatusDotShipped,
                            'in-progress': styles.quarterDetailStatusDotInProgress,
                            planned: styles.quarterDetailStatusDotPlanned,
                          }[milestone.status];
                          return (
                          <li key={idx} className={styles.quarterDetailMilestoneItem}>
                            <span className={`${styles.quarterDetailStatusDot} ${statusDotClass}`} />
                            {milestone.changelog ? (
                              <Link
                                to={`/changelog/${milestone.changelog}`}
                                className={styles.quarterDetailMilestoneLink}
                                onClick={(e) => e.stopPropagation()}
                              >
                                {renderInlineMarkdown(milestone.label)}
                              </Link>
                            ) : (
                              <span className={styles.quarterDetailMilestoneText}>{renderInlineMarkdown(milestone.label)}</span>
                            )}
                          </li>
                          );
                        })}
                      </ul>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
