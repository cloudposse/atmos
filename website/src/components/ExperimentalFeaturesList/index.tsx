import React from 'react';
import Link from '@docusaurus/Link';
import { getGroupedExperimentalFeatures } from '@site/src/data/experimentalFeatures';
import styles from './styles.module.css';

export default function ExperimentalFeaturesList(): JSX.Element {
  const groups = getGroupedExperimentalFeatures();

  if (groups.length === 0) {
    return (
      <p className={styles.noFeatures}>
        No features are currently marked as experimental.
      </p>
    );
  }

  return (
    <div className={styles.container}>
      {groups.map((group) => (
        <div key={group.id} className={styles.groupContainer}>
          <div className={styles.groupHeader}>{group.title}</div>
          <ul className={styles.featureList}>
            {group.features.map((feature, index) => (
              <li key={index} className={styles.featureItem}>
                <div className={styles.featureHeader}>
                  <strong className={styles.featureTitle}>{feature.label}</strong>
                  <span className={styles.links}>
                    {feature.docs && (
                      <Link to={feature.docs} className={styles.link}>
                        Docs
                      </Link>
                    )}
                    {feature.changelog && (
                      <Link to={`/changelog/${feature.changelog}`} className={styles.link}>
                        Announcement
                      </Link>
                    )}
                    {feature.prd && (
                      <Link to={`https://github.com/cloudposse/atmos/blob/main/docs/prd/${feature.prd}.md`} className={styles.link}>
                        PRD
                      </Link>
                    )}
                  </span>
                </div>
                {feature.description && (
                  <p className={styles.description}>{feature.description}</p>
                )}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}
