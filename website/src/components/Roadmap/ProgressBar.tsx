import React from 'react';
import { motion } from 'framer-motion';
import styles from './styles.module.css';

interface ProgressBarProps {
  progress: number;
  showLabel?: boolean;
  size?: 'small' | 'medium' | 'large';
  animated?: boolean;
}

export default function ProgressBar({
  progress,
  showLabel = true,
  size = 'medium',
  animated = true,
}: ProgressBarProps): JSX.Element {
  const clampedProgress = Math.min(100, Math.max(0, progress));

  const getProgressColor = (value: number): string => {
    if (value >= 90) return 'var(--progress-complete)';
    if (value >= 70) return 'var(--progress-high)';
    if (value >= 40) return 'var(--progress-medium)';
    return 'var(--progress-low)';
  };

  return (
    <div className={`${styles.progressContainer} ${styles[`progress--${size}`]}`}>
      <div className={styles.progressTrack}>
        <motion.div
          className={styles.progressFill}
          initial={animated ? { width: 0 } : { width: `${clampedProgress}%` }}
          animate={{ width: `${clampedProgress}%` }}
          transition={{ duration: 0.8, ease: 'easeOut', delay: 0.2 }}
          style={{ backgroundColor: getProgressColor(clampedProgress) }}
        />
      </div>
      {showLabel && (
        <span className={styles.progressLabel}>{clampedProgress}%</span>
      )}
    </div>
  );
}
