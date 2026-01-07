import React from 'react';
import Link from '@docusaurus/Link';
import roadmapConfig from '@site/src/data/roadmap';
import styles from './styles.module.css';

interface ExperimentalMilestone {
  label: string;
  status: string;
  quarter: string;
  docs?: string;
  changelog?: string;
  prd?: string;
  description?: string;
}

interface ExperimentalFeature {
  label: string;
  docs?: string;
  changelog?: string;
  prd?: string;
  description?: string;
}

/**
 * Extracts all experimental features from the roadmap data.
 * This ensures the experimental features page stays in sync with the roadmap.
 */
function getExperimentalFeatures(): ExperimentalFeature[] {
  const features: ExperimentalFeature[] = [];

  // Check featured items.
  if (roadmapConfig.featured) {
    roadmapConfig.featured.forEach((item: { experimental?: boolean; title: string; docs?: string; changelog?: string; prd?: string; description?: string }) => {
      if (item.experimental) {
        features.push({
          label: item.title,
          docs: item.docs,
          changelog: item.changelog,
          prd: item.prd,
          description: item.description,
        });
      }
    });
  }

  // Check initiative milestones.
  if (roadmapConfig.initiatives) {
    roadmapConfig.initiatives.forEach((initiative: { milestones?: ExperimentalMilestone[] }) => {
      if (initiative.milestones) {
        initiative.milestones.forEach((milestone: ExperimentalMilestone & { experimental?: boolean }) => {
          if (milestone.experimental) {
            // Avoid duplicates (some features appear in both featured and initiatives).
            const exists = features.some(f => f.label === milestone.label);
            if (!exists) {
              features.push({
                label: milestone.label,
                docs: milestone.docs,
                changelog: milestone.changelog,
                prd: milestone.prd,
                description: milestone.description,
              });
            }
          }
        });
      }
    });
  }

  return features;
}

export default function ExperimentalFeaturesList(): JSX.Element {
  const features = getExperimentalFeatures();

  if (features.length === 0) {
    return (
      <p className={styles.noFeatures}>
        No features are currently marked as experimental.
      </p>
    );
  }

  return (
    <div className={styles.container}>
      <ul className={styles.featureList}>
        {features.map((feature, index) => (
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
  );
}
