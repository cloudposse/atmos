import React from 'react';
import styles from './BestPracticesBadge.module.css';

interface BestPractices {
  name: string;
  repo_url: string;
  badge_level: string;
  achieve_passing_status?: string;
  updated_at?: string;
}

interface BestPracticesBadgeProps {
  bestPractices: BestPractices | null;
  projectId: number;
}

export default function BestPracticesBadge({ bestPractices, projectId }: BestPracticesBadgeProps) {
  const projectUrl = `https://www.bestpractices.dev/projects/${projectId}`;

  if (!bestPractices) {
    return (
      <div className={styles.panel}>
        <h2>OpenSSF Best Practices badge</h2>
        <p>
          Data temporarily unavailable — verify directly at{' '}
          <a href={projectUrl} target="_blank" rel="noreferrer">
            bestpractices.dev/projects/{projectId}
          </a>
          .
        </p>
      </div>
    );
  }

  const updatedAt = bestPractices.updated_at
    ? new Date(bestPractices.updated_at).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    : null;

  return (
    <div className={styles.panel}>
      <h2>OpenSSF Best Practices badge</h2>
      <div className={styles.badgeRow}>
        <span className={`${styles.badgeLevel} ${styles[bestPractices.badge_level] ?? ''}`}>
          {bestPractices.badge_level}
        </span>
        {updatedAt && <span className={styles.updatedAt}>Achieved {updatedAt}</span>}
      </div>
      <p>
        Verify directly at{' '}
        <a href={projectUrl} target="_blank" rel="noreferrer">
          bestpractices.dev/projects/{projectId}
        </a>
        .
      </p>
    </div>
  );
}
