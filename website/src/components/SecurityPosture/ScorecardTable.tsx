import React, { useMemo, useState } from 'react';
import Admonition from '@theme/Admonition';
import { RiArrowRightSLine } from 'react-icons/ri';
import { scoreTier } from './scoreTier';
import { CHECK_RISK_LEVEL, sortChecksByRisk } from './checkRiskLevel';
import { CHECK_ANNOTATIONS } from './checkAnnotations';
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
  const sortedChecks = useMemo(() => sortChecksByRisk(checks), [checks]);

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
      {sortedChecks.map((check) => {
        const tier = scoreTier(check.score);
        const risk = CHECK_RISK_LEVEL[check.name];
        const annotation = CHECK_ANNOTATIONS[check.name];
        const hasDetails = Array.isArray(check.details) && check.details.length > 0;
        const isExpandable = hasDetails || !!annotation;
        const isOpen = expanded.has(check.name);

        return (
          <div
            key={check.name}
            className={`${styles.row} ${isExpandable ? styles.expandableRow : ''}`}
            onClick={isExpandable ? () => toggle(check.name) : undefined}
          >
            <div className={styles.scoreCol}>
              <span className={styles.scoreNumber}>
                {check.score < 0 ? 'N/A' : check.score}
                {annotation && (
                  <sup className={styles.annotationMark} title="See Cloud Posse's note on this score">
                    *
                  </sup>
                )}
              </span>
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
                {annotation && (
                  <span className={`${styles.riskBadge} ${styles.annotated}`}>
                    Score inaccurate
                  </span>
                )}
              </div>
              <p className={styles.reason}>{check.reason}</p>
              {isOpen && (
                <>
                  {annotation && (
                    <Admonition type="info" title="Cloud Posse's response" className={styles.annotationNote}>
                      {annotation.note}
                    </Admonition>
                  )}
                  {hasDetails && (
                    <ul className={styles.detailsList}>
                      {check.details!.map((detail, index) => (
                        <li key={index}>{detail}</li>
                      ))}
                    </ul>
                  )}
                </>
              )}
            </div>
            {isExpandable && (
              <button
                type="button"
                className={styles.chevronButton}
                aria-expanded={isOpen}
                aria-label={`${isOpen ? 'Collapse' : 'Expand'} details for ${check.name}`}
                onClick={(event) => {
                  event.stopPropagation();
                  toggle(check.name);
                }}
              >
                <RiArrowRightSLine
                  className={`${styles.chevron} ${isOpen ? styles.chevronOpen : ''}`}
                />
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}
