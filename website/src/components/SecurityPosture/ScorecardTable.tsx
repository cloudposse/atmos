import React from 'react';
import styles from './ScorecardTable.module.css';

interface ScorecardCheck {
  name: string;
  score: number;
  reason: string;
  documentation?: {
    url?: string;
    short?: string;
  };
}

interface ScorecardTableProps {
  checks: ScorecardCheck[];
}

function scoreTier(score: number): 'good' | 'warn' | 'bad' {
  if (score >= 8) {
    return 'good';
  }
  if (score >= 4) {
    return 'warn';
  }
  return 'bad';
}

export default function ScorecardTable({ checks }: ScorecardTableProps) {
  return (
    <div className={styles.tableWrapper}>
      <table className={styles.table}>
        <thead>
          <tr>
            <th>Check</th>
            <th>Score</th>
            <th>Reason</th>
          </tr>
        </thead>
        <tbody>
          {checks.map((check) => (
            <tr key={check.name}>
              <td>
                {check.documentation?.url ? (
                  <a href={check.documentation.url} target="_blank" rel="noreferrer">
                    {check.name}
                  </a>
                ) : (
                  check.name
                )}
              </td>
              <td>
                <span className={`${styles.scorePill} ${styles[scoreTier(check.score)]}`}>
                  {check.score}/10
                </span>
              </td>
              <td className={styles.reason}>{check.reason}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
