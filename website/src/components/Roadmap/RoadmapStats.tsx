import React from 'react';
import { motion } from 'framer-motion';
import { RiCheckLine, RiLoader4Line, RiCalendarTodoLine } from 'react-icons/ri';
import styles from './styles.module.css';

interface Milestone {
  status: 'shipped' | 'in-progress' | 'planned';
}

interface Initiative {
  milestones: Milestone[];
}

interface RoadmapStatsProps {
  initiatives: Initiative[];
}

interface Stats {
  shipped: number;
  inProgress: number;
  planned: number;
  total: number;
  percentComplete: number;
}

function computeStats(initiatives: Initiative[]): Stats {
  let shipped = 0;
  let inProgress = 0;
  let planned = 0;

  for (const initiative of initiatives) {
    for (const milestone of initiative.milestones) {
      if (milestone.status === 'shipped') {
        shipped++;
      } else if (milestone.status === 'in-progress') {
        inProgress++;
      } else {
        planned++;
      }
    }
  }

  const total = shipped + inProgress + planned;
  const percentComplete = total > 0 ? Math.round((shipped / total) * 100) : 0;

  return { shipped, inProgress, planned, total, percentComplete };
}

export default function RoadmapStats({ initiatives }: RoadmapStatsProps): JSX.Element {
  const stats = computeStats(initiatives);

  return (
    <div className={styles.roadmapStats}>
      <div className={styles.statsHeader}>
        <h3 className={styles.statsTitle}>Overall Progress</h3>
        <span className={styles.statsPercent}>{stats.percentComplete}% Complete</span>
      </div>

      <div className={styles.statsProgressBar}>
        <motion.div
          className={styles.statsProgressShipped}
          initial={{ width: 0 }}
          animate={{ width: `${stats.total > 0 ? (stats.shipped / stats.total) * 100 : 0}%` }}
          transition={{ duration: 0.8, ease: 'easeOut' }}
        />
        <motion.div
          className={styles.statsProgressInProgress}
          initial={{ width: 0 }}
          animate={{ width: `${stats.total > 0 ? (stats.inProgress / stats.total) * 100 : 0}%` }}
          transition={{ duration: 0.8, ease: 'easeOut', delay: 0.2 }}
        />
      </div>

      <div className={styles.statsBreakdown}>
        <div className={styles.statItem}>
          <span className={`${styles.statIcon} ${styles.statIconShipped}`}>
            <RiCheckLine />
          </span>
          <span className={styles.statCount}>{stats.shipped}</span>
          <span className={styles.statLabel}>Shipped</span>
        </div>
        <div className={styles.statItem}>
          <span className={`${styles.statIcon} ${styles.statIconInProgress}`}>
            <RiLoader4Line />
          </span>
          <span className={styles.statCount}>{stats.inProgress}</span>
          <span className={styles.statLabel}>In Progress</span>
        </div>
        <div className={styles.statItem}>
          <span className={`${styles.statIcon} ${styles.statIconPlanned}`}>
            <RiCalendarTodoLine />
          </span>
          <span className={styles.statCount}>{stats.planned}</span>
          <span className={styles.statLabel}>Planned</span>
        </div>
      </div>
    </div>
  );
}
