import React, { useState } from 'react';
import { RiArrowRightSLine } from 'react-icons/ri';
import { scoreTier } from './scoreTier';
import { CHECK_RISK_LEVEL } from './checkRiskLevel';
import styles from './ScorecardTable.module.css';

interface ScorecardCheck {
  name: string;
  score: number;
  reason: string;
  details?: string[] | null;
  documentation?: {
    url?: string;
    short?: string;
  };
}

interface ScorecardTableProps {
  checks: ScorecardCheck[];
}

export default function ScorecardTable({ checks }: ScorecardTableProps) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  function toggle(name: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }

  return (
    <div className={styles.list}>
      {checks.map((check) => {
        const tier = scoreTier(check.score);
        const risk = CHECK_RISK_LEVEL[check.name];
        const hasDetails = Array.isArray(check.details) && check.details.length > 0;
        const isOpen = expanded.has(check.name);

        return (
          <div
            key={check.name}
            className={`${styles.row} ${hasDetails ? styles.expandableRow : ''}`}
            onClick={hasDetails ? () => toggle(check.name) : undefined}
          >
            <div className={styles.scoreCol}>
              <span className={styles.scoreNumber}>{check.score < 0 ? 'N/A' : check.score}</span>
              <span className={`${styles.scoreUnderline} ${styles[tier]}`} />
            </div>
            <div className={styles.content}>
              <div className={styles.nameLine}>
                {check.documentation?.url ? (
                  <a
                    className={styles.checkName}
                    href={check.documentation.url}
                    target="_blank"
                    rel="noreferrer"
                    onClick={(event) => event.stopPropagation()}
                  >
                    {check.name}
                  </a>
                ) : (
                  <span className={styles.checkName}>{check.name}</span>
                )}
                {risk && (
                  <span className={`${styles.riskBadge} ${styles[risk.toLowerCase()]}`}>
                    {risk}
                  </span>
                )}
              </div>
              <p className={styles.reason}>{check.reason}</p>
              {hasDetails && isOpen && (
                <ul className={styles.detailsList}>
                  {check.details!.map((detail, index) => (
                    <li key={index}>{detail}</li>
                  ))}
                </ul>
              )}
            </div>
            {hasDetails && (
              <RiArrowRightSLine
                className={`${styles.chevron} ${isOpen ? styles.chevronOpen : ''}`}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
