import React from 'react';
import { scoreTier } from './scoreTier';
import styles from './ScoreGauge.module.css';

interface ScoreGaugeProps {
  score: number;
}

const RADIUS = 42;
const CENTER = 50;
const STROKE_WIDTH = 8;

export default function ScoreGauge({ score }: ScoreGaugeProps) {
  const tier = scoreTier(score);
  const isUnavailable = score < 0;
  const progress = isUnavailable ? 0 : Math.max(0, Math.min(score, 10)) * 10;

  return (
    <div className={styles.gauge}>
      <svg viewBox="0 0 100 100" className={styles.ring} role="img" aria-label={`Score ${isUnavailable ? 'unavailable' : `${score.toFixed(1)} out of 10`}`}>
        <circle
          cx={CENTER}
          cy={CENTER}
          r={RADIUS}
          fill="none"
          strokeWidth={STROKE_WIDTH}
          className={styles.track}
        />
        {!isUnavailable && (
          <circle
            cx={CENTER}
            cy={CENTER}
            r={RADIUS}
            fill="none"
            strokeWidth={STROKE_WIDTH}
            strokeLinecap="round"
            pathLength={100}
            strokeDasharray={`${progress} 100`}
            className={`${styles.progress} ${styles[tier]}`}
            transform={`rotate(-90 ${CENTER} ${CENTER})`}
          />
        )}
      </svg>
      <div className={styles.value}>
        {isUnavailable ? (
          <span className={styles.na}>N/A</span>
        ) : (
          <span className={styles.number}>{score.toFixed(1)}</span>
        )}
      </div>
    </div>
  );
}
