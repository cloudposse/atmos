import React from 'react';
import styles from './BestPracticesBadge.module.css';

interface BestPractices {
  name: string;
  repo_url: string;
  badge_level: string;
  achieve_passing_status?: string;
  updated_at?: string;
  achieved_passing_at?: string;
  achieved_silver_at?: string;
  achieved_gold_at?: string;
}

const ACHIEVED_AT_FIELD: Partial<Record<string, keyof BestPractices>> = {
  passing: 'achieved_passing_at',
  silver: 'achieved_silver_at',
  gold: 'achieved_gold_at',
};

interface BestPracticesBadgeProps {
  bestPractices: BestPractices | null;
  projectId: number;
}

export default function BestPracticesBadge({ bestPractices, projectId }: BestPracticesBadgeProps) {
  const projectUrl = `https://www.bestpractices.dev/projects/${projectId}`;

  if (!bestPractices) {
    return (
      <p className={styles.row}>
        Best Practices badge data temporarily unavailable — verify directly at{' '}
        <a href={projectUrl} target="_blank" rel="noreferrer">
          bestpractices.dev/projects/{projectId}
        </a>
        .
      </p>
    );
  }

  const achievedAtField = ACHIEVED_AT_FIELD[bestPractices.badge_level];
  const achievedAtRaw = achievedAtField ? bestPractices[achievedAtField] : undefined;
  const achievedAt = achievedAtRaw
    ? new Date(achievedAtRaw).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    : null;

  return (
    <p className={styles.row}>
      <span className={`${styles.badgeLevel} ${styles[bestPractices.badge_level] ?? ''}`}>
        {bestPractices.badge_level}
      </span>
      <span>
        Best Practices badge{achievedAt ? ` — achieved ${achievedAt}` : ''}. Verify at{' '}
        <a href={projectUrl} target="_blank" rel="noreferrer">
          bestpractices.dev/projects/{projectId}
        </a>
        .
      </span>
    </p>
  );
}
