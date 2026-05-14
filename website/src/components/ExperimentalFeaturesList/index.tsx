import React from 'react';
import Link from '@docusaurus/Link';
import roadmapConfig from '@site/src/data/roadmap';
import styles from './styles.module.css';

interface ExperimentalFeature {
  label: string;
  docs?: string;
  changelog?: string;
  prd?: string;
  description?: string;
}

interface FeatureGroup {
  id: string;
  title: string;
  features: ExperimentalFeature[];
}

/**
 * Extracts all experimental features from the roadmap data, grouped by initiative.
 * This ensures the experimental features page stays in sync with the roadmap.
 */
function getGroupedExperimentalFeatures(): FeatureGroup[] {
  const groups: FeatureGroup[] = [];
  const seenLabels = new Set<string>();

  // First, collect featured experimental items into a "Featured" group.
  if (roadmapConfig.featured) {
    const featuredFeatures: ExperimentalFeature[] = [];
    roadmapConfig.featured.forEach((item: { experimental?: boolean; title: string; docs?: string; changelog?: string; prd?: string; description?: string }) => {
      if (item.experimental) {
        featuredFeatures.push({
          label: item.title,
          docs: item.docs,
          changelog: item.changelog,
          prd: item.prd,
          description: item.description,
        });
        seenLabels.add(item.title);
      }
    });
    if (featuredFeatures.length > 0) {
      groups.push({
        id: 'featured',
        title: 'Featured',
        features: featuredFeatures,
      });
    }
  }

  // Then, group initiative milestones by their parent initiative.
  if (roadmapConfig.initiatives) {
    roadmapConfig.initiatives.forEach((initiative: { id: string; title: string; milestones?: Array<{ label: string; experimental?: boolean; docs?: string; changelog?: string; prd?: string; description?: string }> }) => {
      if (initiative.milestones) {
        const initiativeFeatures: ExperimentalFeature[] = [];
        initiative.milestones.forEach((milestone) => {
          if (milestone.experimental && !seenLabels.has(milestone.label)) {
            initiativeFeatures.push({
              label: milestone.label,
              docs: milestone.docs,
              changelog: milestone.changelog,
              prd: milestone.prd,
              description: milestone.description,
            });
            seenLabels.add(milestone.label);
          }
        });
        if (initiativeFeatures.length > 0) {
          groups.push({
            id: initiative.id,
            title: initiative.title,
            features: initiativeFeatures,
          });
        }
      }
    });
  }

  return groups;
}

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
